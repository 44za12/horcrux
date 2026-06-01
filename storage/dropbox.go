package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// dropboxAppKey is a public identifier for PKCE OAuth — not a secret.
// In PKCE flows the app key only identifies the application; proof of
// possession is provided by the code_challenge / code_verifier exchange.
// Shipping this in the binary is expected for desktop OAuth apps.
const dropboxAppKey = "aubp4ukbc7e77pb"

type DropboxProvider struct {
	Token      *oauth2.Token
	httpClient *http.Client
	mu         sync.Mutex
}

func NewDropboxProvider(token *oauth2.Token) *DropboxProvider {
	return &DropboxProvider{Token: token}
}

func (d *DropboxProvider) Name() string { return "dropbox" }

func (d *DropboxProvider) Authenticate(ctx context.Context) error {
	if d.Token != nil && d.Token.Valid() {
		d.httpClient = &http.Client{
			Transport: &dropboxTransport{token: d.Token.AccessToken, provider: d},
		}
		return nil
	}

	if d.Token != nil && d.Token.RefreshToken != "" {
		if err := d.refreshToken(); err == nil {
			d.httpClient = &http.Client{
				Transport: &dropboxTransport{token: d.Token.AccessToken, provider: d},
			}
			return nil
		}
	}

	return fmt.Errorf("Dropbox not authenticated. Run 'horcrux providers auth dropbox' first")
}

func (d *DropboxProvider) refreshToken() error {
	if d.Token == nil || d.Token.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", d.Token.RefreshToken)
	data.Set("client_id", dropboxAppKey)

	resp, err := http.Post("https://api.dropboxapi.com/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("refreshing token: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding refresh response: %w", err)
	}

	d.Token.AccessToken = result.AccessToken
	if result.ExpiresIn > 0 {
		d.Token.Expiry = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	}
	return nil
}

func (d *DropboxProvider) Upload(ctx context.Context, key string, data []byte) error {
	if d.httpClient == nil {
		return fmt.Errorf("not authenticated")
	}

	path := "/" + key
	args, _ := json.Marshal(map[string]interface{}{"path": path, "mode": "overwrite", "autorename": false, "mute": true})

	req, _ := http.NewRequestWithContext(ctx, "POST",
		"https://content.dropboxapi.com/2/files/upload",
		bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Dropbox-API-Arg", string(args))

	resp, err := d.doRequest(req)
	if err != nil {
		return fmt.Errorf("upload to Dropbox: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Dropbox upload failed (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func (d *DropboxProvider) Download(ctx context.Context, key string) ([]byte, error) {
	if d.httpClient == nil {
		return nil, fmt.Errorf("not authenticated")
	}

	path := "/" + key
	args, _ := json.Marshal(map[string]string{"path": path})

	req, _ := http.NewRequestWithContext(ctx, "POST",
		"https://content.dropboxapi.com/2/files/download",
		nil)
	req.Header.Set("Dropbox-API-Arg", string(args))

	resp, err := d.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("download from Dropbox: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Dropbox download failed (%d): %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

func (d *DropboxProvider) Delete(ctx context.Context, key string) error {
	if d.httpClient == nil {
		return fmt.Errorf("not authenticated")
	}

	path := "/" + key
	args, _ := json.Marshal(map[string]string{"path": path})

	req, _ := http.NewRequestWithContext(ctx, "POST",
		"https://api.dropboxapi.com/2/files/delete_v2",
		strings.NewReader(string(args)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.doRequest(req)
	if err != nil {
		return fmt.Errorf("delete from Dropbox: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 409 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Dropbox delete failed (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func (d *DropboxProvider) List(ctx context.Context, prefix string) ([]string, error) {
	if d.httpClient == nil {
		return nil, fmt.Errorf("not authenticated")
	}

	args, _ := json.Marshal(map[string]interface{}{
		"path":                                "",
		"recursive":                           false,
		"include_deleted":                     false,
		"include_has_explicit_shared_members": false,
	})

	req, _ := http.NewRequestWithContext(ctx, "POST",
		"https://api.dropboxapi.com/2/files/list_folder",
		strings.NewReader(string(args)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.doRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Dropbox list failed (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Entries []struct {
			Name string `json:"name"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	var keys []string
	for _, e := range result.Entries {
		if strings.HasPrefix(e.Name, prefix) {
			keys = append(keys, e.Name)
		}
	}
	return keys, nil
}

func (d *DropboxProvider) Exists(ctx context.Context, key string) (bool, error) {
	if d.httpClient == nil {
		return false, fmt.Errorf("not authenticated")
	}

	path := "/" + key
	args, _ := json.Marshal(map[string]string{"path": path})

	req, _ := http.NewRequestWithContext(ctx, "POST",
		"https://api.dropboxapi.com/2/files/get_metadata",
		strings.NewReader(string(args)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.doRequest(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return true, nil
	}
	if resp.StatusCode == 409 {
		return false, nil
	}
	return false, fmt.Errorf("Dropbox metadata check failed (%d)", resp.StatusCode)
}

func (d *DropboxProvider) doRequest(req *http.Request) (*http.Response, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.httpClient == nil {
		return nil, fmt.Errorf("not authenticated")
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 401 {
		resp.Body.Close()
		if err := d.refreshToken(); err != nil {
			return nil, fmt.Errorf("token expired and refresh failed: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+d.Token.AccessToken)
		return d.httpClient.Do(req)
	}

	return resp, nil
}

type dropboxTransport struct {
	token    string
	provider *DropboxProvider
}

func (t *dropboxTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(req)
}

func generatePKCEVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func computePKCEChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func RunDropboxAuth() (*oauth2.Token, error) {
	verifier, err := generatePKCEVerifier()
	if err != nil {
		return nil, fmt.Errorf("generating PKCE verifier: %w", err)
	}
	challenge := computePKCEChallenge(verifier)

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", dropboxAppKey)
	params.Set("redirect_uri", "http://localhost:9876/callback")
	params.Set("token_access_type", "offline")
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")
	params.Set("scope", "files.content.read files.content.write")
	state, err := generatePKCEVerifier()
	if err != nil {
		return nil, fmt.Errorf("generating OAuth state: %w", err)
	}
	params.Set("state", state)

	authURL := "https://www.dropbox.com/oauth2/authorize?" + params.Encode()

	fmt.Println("\nOpening browser for Dropbox authorization...")
	fmt.Printf("If browser doesn't open, visit:\n\n%s\n\n", authURL)

	openBrowser(authURL)

	code, err := receiveAuthCode(state)
	if err != nil {
		return nil, err
	}

	tokenData := url.Values{}
	tokenData.Set("grant_type", "authorization_code")
	tokenData.Set("code", code)
	tokenData.Set("redirect_uri", "http://localhost:9876/callback")
	tokenData.Set("client_id", dropboxAppKey)
	tokenData.Set("code_verifier", verifier)

	resp, err := http.Post("https://api.dropboxapi.com/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(tokenData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("exchanging auth code: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}

	token := &oauth2.Token{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		RefreshToken: tokenResp.RefreshToken,
	}
	if tokenResp.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return token, nil
}

func receiveAuthCode(expectedState string) (string, error) {
	type callbackResult struct {
		code string
		err  error
	}

	resultCh := make(chan callbackResult, 1)
	mux := http.NewServeMux()
	server := &http.Server{Addr: "127.0.0.1:9876"}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		errParam := r.URL.Query().Get("error")

		if errParam != "" {
			w.WriteHeader(400)
			fmt.Fprintln(w, "Authorization failed: "+errParam)
			resultCh <- callbackResult{err: fmt.Errorf("authorization denied: %s", errParam)}
			return
		}

		if state != expectedState {
			w.WriteHeader(400)
			fmt.Fprintln(w, "Invalid state parameter")
			resultCh <- callbackResult{err: fmt.Errorf("invalid state parameter")}
			return
		}

		if code == "" {
			w.WriteHeader(400)
			fmt.Fprintln(w, "Missing authorization code")
			resultCh <- callbackResult{err: fmt.Errorf("missing authorization code")}
			return
		}

		fmt.Fprintln(w, "Authorization successful! You can close this tab now.")
		resultCh <- callbackResult{code: code}
	})

	server.Handler = mux
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			resultCh <- callbackResult{err: err}
		}
	}()

	select {
	case result := <-resultCh:
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		if result.err != nil {
			return "", result.err
		}
		return result.code, nil
	case <-time.After(5 * time.Minute):
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		return "", fmt.Errorf("authentication timed out after 5 minutes")
	}
}
