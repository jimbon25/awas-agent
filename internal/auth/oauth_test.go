package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCredentialsLoadSave(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "awas-auth-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "oauth.json")

	creds := NewCredentials(configPath)
	creds.AccessToken = "test-token-123"
	creds.Provider = "github"

	if err := creds.Save(); err != nil {
		t.Fatalf("failed to save credentials: %v", err)
	}

	creds2 := NewCredentials(configPath)
	if creds2.AccessToken != "test-token-123" || creds2.Provider != "github" {
		t.Errorf("loaded credentials mismatch: %v", creds2)
	}
}

func TestDeviceFlowAuth(t *testing.T) {
	pollCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/device/code" {
			resp := map[string]any{
				"device_code":      "dev-code-999",
				"user_code":        "ABCD-1234",
				"verification_uri": "https://github.com/login/device",
				"expires_in":       60,
				"interval":         1, 
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if r.URL.Path == "/device/token" {
			pollCount++
			if pollCount == 1 {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]any{
					"error": "authorization_pending",
				})
			} else {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]any{
					"access_token": "mock-oauth-token",
					"token_type":   "bearer",
				})
			}
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	authClient := NewAuthClient("mock-client-id", server.URL + "/device/code", server.URL + "/device/token")

	flow, err := authClient.StartDeviceFlow()
	if err != nil {
		t.Fatalf("StartDeviceFlow failed: %v", err)
	}

	if flow.UserCode != "ABCD-1234" || flow.VerificationURI != "https://github.com/login/device" {
		t.Errorf("unexpected flow details: %v", flow)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	token, err := authClient.PollForToken(ctx, flow)
	if err != nil {
		t.Fatalf("PollForToken failed: %v", err)
	}

	if token != "mock-oauth-token" {
		t.Errorf("expected 'mock-oauth-token', got '%s'", token)
	}
}

func TestLoopbackServerFlow(t *testing.T) {
	server, err := StartLoopbackServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("StartLoopbackServer failed: %v", err)
	}
	defer server.Close()

	go func() {
		time.Sleep(50 * time.Millisecond)
		resp, err := http.Get(server.GetRedirectURI() + "?token=loopback-token-xyz")
		if err == nil {
			resp.Body.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	token, err := server.WaitForToken(ctx)
	if err != nil {
		t.Fatalf("WaitForToken failed: %v", err)
	}

	if token != "loopback-token-xyz" {
		t.Errorf("expected 'loopback-token-xyz', got '%s'", token)
	}
}
