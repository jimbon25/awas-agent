package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type AuthClient struct {
	clientID      string
	deviceCodeURL string
	tokenURL      string
	httpClient    *http.Client
}

type DeviceFlow struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	Interval        int    `json:"interval"`
	ExpiresIn       int    `json:"expires_in"`
}

func NewAuthClient(clientID, deviceCodeURL, tokenURL string) *AuthClient {
	return &AuthClient{
		clientID:      clientID,
		deviceCodeURL: deviceCodeURL,
		tokenURL:      tokenURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (a *AuthClient) StartDeviceFlow() (*DeviceFlow, error) {
	data := url.Values{}
	data.Set("client_id", a.clientID)
	data.Set("scope", "read:user")

	req, err := http.NewRequest("POST", a.deviceCodeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("start device flow request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status starting device flow (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var flow DeviceFlow
	if err := json.NewDecoder(resp.Body).Decode(&flow); err != nil {
		return nil, fmt.Errorf("decode device flow response failed: %w", err)
	}

	if flow.Interval <= 0 {
		flow.Interval = 5
	}
	return &flow, nil
}

func (a *AuthClient) PollForToken(ctx context.Context, flow *DeviceFlow) (string, error) {
	currentInterval := time.Duration(flow.Interval) * time.Second

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
			token, pending, nextInterval, err := a.requestToken(flow.DeviceCode)
			if err != nil {
				return "", err
			}
			if !pending {
				return token, nil
			}
			if nextInterval > 0 {
				currentInterval = nextInterval
			}

			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(currentInterval):
				
			}
		}
	}
}

func (a *AuthClient) requestToken(deviceCode string) (string, bool, time.Duration, error) {
	data := url.Values{}
	data.Set("client_id", a.clientID)
	data.Set("device_code", deviceCode)
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	req, err := http.NewRequest("POST", a.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", false, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", false, 0, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, 0, err
	}

	if home, err := os.UserHomeDir(); err == nil {
		logFile := filepath.Join(home, ".awas", "oauth_debug.log")
		f, _ := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if f != nil {
			f.WriteString(fmt.Sprintf("[%s] Status: %d, Body: %s\n", time.Now().Format(time.RFC3339), resp.StatusCode, string(bodyBytes)))
			f.Close()
		}
	}

	var errResp struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		Interval         int    `json:"interval"`
	}
	if err := json.Unmarshal(bodyBytes, &errResp); err == nil && errResp.Error != "" {
		if errResp.Error == "authorization_pending" {
			return "", true, 0, nil
		}
		if errResp.Error == "slow_down" {
			interval := time.Duration(errResp.Interval) * time.Second
			if interval <= 0 {
				interval = 10 * time.Second
			}
			return "", true, interval, nil
		}
		return "", false, 0, fmt.Errorf("oauth error: %s (%s)", errResp.Error, errResp.ErrorDescription)
	}

	if resp.StatusCode != http.StatusOK {
		return "", false, 0, fmt.Errorf("unexpected token request status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return "", false, 0, err
	}

	if tokenResp.AccessToken == "" {
		return "", false, 0, fmt.Errorf("empty access token returned")
	}

	return tokenResp.AccessToken, false, 0, nil
}

type LoopbackServer struct {
	server      *http.Server
	tokenChan   chan string
	redirectURI string
}

func StartLoopbackServer(addr string) (*LoopbackServer, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)
	tokenChan := make(chan string, 1)

	mux := http.NewServeMux()
	server := &http.Server{
		Handler: mux,
	}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			token = r.URL.Query().Get("code")
		}

		if token != "" {
			select {
			case tokenChan <- token:
			default:
			}
		}

		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`
			<!DOCTYPE html>
			<html>
			<head>
				<title>AWAS Authentication</title>
				<style>
					body { font-family: sans-serif; text-align: center; margin-top: 50px; background-color: #1a1a1a; color: #ffffff; }
					h1 { color: #00F0FF; }
				</style>
			</head>
			<body>
				<h1>Authentication Successful!</h1>
				<p>AWAS CLI has been authorized. You can close this browser window now.</p>
			</body>
			</html>
		`))
	})

	go func() {
		server.Serve(listener)
	}()

	return &LoopbackServer{
		server:      server,
		tokenChan:   tokenChan,
		redirectURI: redirectURI,
	}, nil
}

func (s *LoopbackServer) GetRedirectURI() string {
	return s.redirectURI
}

func (s *LoopbackServer) WaitForToken(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case token := <-s.tokenChan:
		return token, nil
	}
}

func (s *LoopbackServer) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}
