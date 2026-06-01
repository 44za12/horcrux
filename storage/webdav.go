package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type WebDAVProvider struct {
	BaseURL  string
	Username string
	Password string
	client   *http.Client
}

func NewWebDAVProvider(baseURL, username, password string) *WebDAVProvider {
	return &WebDAVProvider{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Username: username,
		Password: password,
		client:   &http.Client{},
	}
}

func (w *WebDAVProvider) Name() string { return "webdav" }

func (w *WebDAVProvider) Authenticate(_ context.Context) error {
	parsed, err := url.Parse(w.BaseURL)
	if err != nil {
		return fmt.Errorf("invalid WebDAV URL: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" {
		return fmt.Errorf("WebDAV endpoint must use https")
	}
	req, err := http.NewRequest("PROPFIND", w.BaseURL+"/horcrux/", nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.SetBasicAuth(w.Username, w.Password)
	req.Header.Set("Depth", "0")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to WebDAV: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 || resp.StatusCode == 405 {
		req2, _ := http.NewRequest("MKCOL", w.BaseURL+"/horcrux/", nil)
		req2.SetBasicAuth(w.Username, w.Password)
		resp2, err := w.client.Do(req2)
		if err != nil {
			return fmt.Errorf("creating directory: %w", err)
		}
		defer resp2.Body.Close()
		if resp2.StatusCode != 201 && resp2.StatusCode != 405 && resp2.StatusCode != 301 {
			return fmt.Errorf("creating directory failed: %d", resp2.StatusCode)
		}
	} else if resp.StatusCode >= 400 {
		return fmt.Errorf("authentication failed: HTTP %d", resp.StatusCode)
	}

	return nil
}

func (w *WebDAVProvider) Upload(_ context.Context, key string, data []byte) error {
	url := w.BaseURL + "/horcrux/" + key
	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.SetBasicAuth(w.Username, w.Password)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("upload to WebDAV: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 && resp.StatusCode != 204 && resp.StatusCode != 200 {
		return fmt.Errorf("WebDAV upload failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (w *WebDAVProvider) Download(_ context.Context, key string) ([]byte, error) {
	url := w.BaseURL + "/horcrux/" + key
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(w.Username, w.Password)

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download from WebDAV: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, os.ErrNotExist
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("WebDAV download failed: HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (w *WebDAVProvider) Delete(_ context.Context, key string) error {
	url := w.BaseURL + "/horcrux/" + key
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(w.Username, w.Password)

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("delete from WebDAV: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 && resp.StatusCode != 200 && resp.StatusCode != 404 {
		return fmt.Errorf("WebDAV delete failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (w *WebDAVProvider) Exists(_ context.Context, key string) (bool, error) {
	url := w.BaseURL + "/horcrux/" + key
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return false, err
	}
	req.SetBasicAuth(w.Username, w.Password)

	resp, err := w.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200, nil
}

func (w *WebDAVProvider) List(_ context.Context, prefix string) ([]string, error) {
	req, err := http.NewRequest("PROPFIND", w.BaseURL+"/horcrux/", nil)
	if err != nil {
		return nil, fmt.Errorf("creating PROPFIND request: %w", err)
	}
	req.SetBasicAuth(w.Username, w.Password)
	req.Header.Set("Depth", "1")

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing WebDAV directory: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode != 207 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("WebDAV PROPFIND failed (%d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var keys []string
	xmlStr := string(body)
	basePath := "/horcrux/"
	for {
		start := strings.Index(xmlStr, "<d:href>")
		if start == -1 {
			start = strings.Index(xmlStr, "<D:href>")
		}
		if start == -1 {
			start = strings.Index(xmlStr, "<href>")
		}
		if start == -1 {
			break
		}
		end := strings.Index(xmlStr[start:], ">")
		if end == -1 {
			break
		}
		closeTag := strings.Index(xmlStr[start+end+1:], "<")
		if closeTag == -1 {
			break
		}
		href := xmlStr[start+end+1 : start+end+1+closeTag]
		if decoded, err := url.QueryUnescape(href); err == nil {
			href = decoded
		}
		href = strings.TrimPrefix(href, basePath)
		href = strings.TrimSuffix(href, "/")
		if href != "" && strings.HasPrefix(href, prefix) {
			keys = append(keys, href)
		}
		xmlStr = xmlStr[start+end+1+closeTag:]
	}
	return keys, nil
}
