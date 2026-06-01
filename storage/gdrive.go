package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// GDrive credentials are user-provided, not shipped in the binary.
// Users must create a Google Cloud project and OAuth 2.0 Client ID
// for "Desktop application" at https://console.cloud.google.com/apis/credentials.
// The client secret is stored encrypted in providers.hrcrx, never in source.

type GDriveProvider struct {
	Token        *oauth2.Token
	ClientID     string
	ClientSecret string
	httpClient   *http.Client
	mu           sync.Mutex
}

func NewGDriveProvider(token *oauth2.Token, clientID, clientSecret string) *GDriveProvider {
	return &GDriveProvider{Token: token, ClientID: clientID, ClientSecret: clientSecret}
}

func (g *GDriveProvider) Name() string { return "gdrive" }

func (g *GDriveProvider) Authenticate(ctx context.Context) error {
	if g.ClientID == "" {
		return fmt.Errorf("Google Drive client ID not configured. Run 'horcrux providers auth gdrive' first")
	}
	if g.Token != nil && g.Token.Valid() {
		g.httpClient = g.gdriveOAuthConfig().Client(ctx, g.Token)
		return nil
	}

	if g.Token != nil && g.Token.RefreshToken != "" {
		ts := g.gdriveOAuthConfig().TokenSource(ctx, g.Token)
		newToken, err := ts.Token()
		if err == nil {
			g.Token = newToken
			g.httpClient = g.gdriveOAuthConfig().Client(ctx, newToken)
			return nil
		}
	}

	return fmt.Errorf("Google Drive not authenticated. Run 'horcrux providers auth gdrive' first")
}

func (g *GDriveProvider) gdriveOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     g.ClientID,
		ClientSecret: g.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
		Scopes:      []string{"https://www.googleapis.com/auth/drive.file"},
		RedirectURL: "http://localhost:9876/callback",
	}
}

func (g *GDriveProvider) refreshToken(ctx context.Context) error {
	if g.Token == nil || g.Token.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	data := url.Values{}
	data.Set("client_id", g.ClientID)
	data.Set("client_secret", g.ClientSecret)
	data.Set("refresh_token", g.Token.RefreshToken)
	data.Set("grant_type", "refresh_token")

	resp, err := http.Post("https://oauth2.googleapis.com/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("refreshing token: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding token response: %w", err)
	}

	g.Token.AccessToken = result.AccessToken
	g.Token.TokenType = result.TokenType
	g.Token.Expiry = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	g.httpClient = g.gdriveOAuthConfig().Client(ctx, g.Token)
	return nil
}

func (g *GDriveProvider) doRequest(req *http.Request) (*http.Response, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.httpClient == nil {
		return nil, fmt.Errorf("not authenticated")
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 401 {
		resp.Body.Close()
		if err := g.refreshToken(context.Background()); err != nil {
			return nil, fmt.Errorf("token expired and refresh failed: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+g.Token.AccessToken)
		return g.httpClient.Do(req)
	}

	return resp, nil
}

func (g *GDriveProvider) Upload(ctx context.Context, key string, data []byte) error {
	if g.httpClient == nil {
		return fmt.Errorf("not authenticated")
	}

	fileID, err := g.findFile(ctx, key)
	if err != nil {
		return fmt.Errorf("searching for existing file: %w", err)
	}

	if fileID != "" {
		return g.updateFile(ctx, fileID, data)
	}
	return g.createFile(ctx, key, data)
}

func (g *GDriveProvider) Download(ctx context.Context, key string) ([]byte, error) {
	if g.httpClient == nil {
		return nil, fmt.Errorf("not authenticated")
	}

	fileID, err := g.findFile(ctx, key)
	if err != nil {
		return nil, err
	}
	if fileID == "" {
		return nil, fmt.Errorf("file '%s' not found on Google Drive", key)
	}

	u := fmt.Sprintf("https://www.googleapis.com/drive/v3/files/%s?alt=media", fileID)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := g.doRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (g *GDriveProvider) Delete(ctx context.Context, key string) error {
	if g.httpClient == nil {
		return fmt.Errorf("not authenticated")
	}

	fileID, err := g.findFile(ctx, key)
	if err != nil {
		return err
	}
	if fileID == "" {
		return nil
	}

	u := fmt.Sprintf("https://www.googleapis.com/drive/v3/files/%s", fileID)
	req, _ := http.NewRequestWithContext(ctx, "DELETE", u, nil)
	resp, err := g.doRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		return fmt.Errorf("delete failed with status %d", resp.StatusCode)
	}
	return nil
}

func (g *GDriveProvider) Exists(ctx context.Context, key string) (bool, error) {
	if g.httpClient == nil {
		return false, fmt.Errorf("not authenticated")
	}

	fileID, err := g.findFile(ctx, key)
	if err != nil {
		return false, err
	}
	return fileID != "", nil
}

func (g *GDriveProvider) findFile(ctx context.Context, name string) (string, error) {
	u := fmt.Sprintf("https://www.googleapis.com/drive/v3/files?q=name='%s'+and+trashed=false&fields=files(id)", url.PathEscape(name))
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := g.doRequest(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Files []struct {
			ID string `json:"id"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Files) > 0 {
		return result.Files[0].ID, nil
	}
	return "", nil
}

func (g *GDriveProvider) List(ctx context.Context, prefix string) ([]string, error) {
	if g.httpClient == nil {
		return nil, fmt.Errorf("not authenticated")
	}
	q := fmt.Sprintf("name contains '%s' and trashed=false", prefix)
	u := fmt.Sprintf("https://www.googleapis.com/drive/v3/files?q=%s&fields=files(name)", url.PathEscape(q))
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := g.doRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Files []struct {
			Name string `json:"name"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	var keys []string
	for _, f := range result.Files {
		keys = append(keys, f.Name)
	}
	return keys, nil
}

func (g *GDriveProvider) createFile(ctx context.Context, name string, data []byte) error {
	metadata := map[string]string{"name": name}
	metaJSON, _ := json.Marshal(metadata)

	var body bytes.Buffer
	body.WriteString(fmt.Sprintf("--boundary\r\nContent-Type: application/json; charset=UTF-8\r\n\r\n%s\r\n--boundary\r\nContent-Type: application/octet-stream\r\n\r\n", string(metaJSON)))
	body.Write(data)
	body.WriteString("\r\n--boundary--\r\n")

	req, _ := http.NewRequestWithContext(ctx, "POST",
		"https://www.googleapis.com/upload/drive/v3/files?uploadType=multipart",
		bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", "multipart/related; boundary=boundary")

	resp, err := g.doRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (g *GDriveProvider) updateFile(ctx context.Context, fileID string, data []byte) error {
	req, _ := http.NewRequestWithContext(ctx, "PATCH",
		fmt.Sprintf("https://www.googleapis.com/upload/drive/v3/files/%s?uploadType=media", fileID),
		bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := g.doRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func RunGDriveAuth(clientID, clientSecret string) (*oauth2.Token, error) {
	if clientID == "" {
		return nil, fmt.Errorf("Google Drive client ID is required. Set up OAuth credentials at https://console.cloud.google.com/apis/credentials and pass --client-id")
	}
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
		Scopes:      []string{"https://www.googleapis.com/auth/drive.file"},
		RedirectURL: "http://localhost:9876/callback",
	}
	state, err := generatePKCEVerifier()
	if err != nil {
		return nil, fmt.Errorf("generating OAuth state: %w", err)
	}
	verifier, err := generatePKCEVerifier()
	if err != nil {
		return nil, fmt.Errorf("generating PKCE verifier: %w", err)
	}
	authURL := config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier))

	fmt.Println("\nOpening browser for Google Drive authorization...")
	fmt.Printf("If browser doesn't open, visit:\n\n%s\n\n", authURL)

	openBrowser(authURL)

	code, err := receiveAuthCode(state)
	if err != nil {
		return nil, err
	}

	token, err := config.Exchange(context.Background(), code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("exchanging auth code: %w", err)
	}
	return token, nil
}

func openBrowser(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	cmd.Start()
}
