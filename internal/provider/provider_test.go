package provider

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProviderEndpointAndHeaders(t *testing.T) {
	pOpenAI := &ProviderConfig{
		Name:   ProviderOpenAI,
		APIKey: "sk-openai",
		Model:  "gpt-4o",
	}
	if pOpenAI.GetEndpoint() != "https://api.openai.com/v1" {
		t.Errorf("unexpected OpenAI endpoint: %s", pOpenAI.GetEndpoint())
	}
	headers := pOpenAI.GetHeaders()
	if len(headers) != 0 {
		t.Errorf("expected 0 headers, got %v", headers)
	}

	pAnthropic := &ProviderConfig{
		Name:   ProviderAnthropic,
		APIKey: "sk-ant",
		Model:  "claude-3-opus",
	}
	if pAnthropic.GetEndpoint() != "https://api.anthropic.com/v1" {
		t.Errorf("unexpected Anthropic endpoint: %s", pAnthropic.GetEndpoint())
	}
	headers = pAnthropic.GetHeaders()
	if headers["anthropic-version"] != "2023-06-01" {
		t.Errorf("missing anthropic-version header: %v", headers)
	}
	if headers["x-api-key"] != "sk-ant" {
		t.Errorf("missing x-api-key header: %v", headers)
	}

	pGemini := &ProviderConfig{
		Name:   ProviderGemini,
		APIKey: "gemini-key",
		Model:  "gemini-1.5-pro",
	}
	if pGemini.GetEndpoint() != "https://generativelanguage.googleapis.com/v1beta/openai" {
		t.Errorf("unexpected Gemini endpoint: %s", pGemini.GetEndpoint())
	}
}

func TestFetchModels(t *testing.T) {
	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"data": [
				{"id": "gpt-4o"},
				{"id": "gpt-4-turbo"}
			]
		}`))
	}))
	defer openAIServer.Close()

	pOpenAI := &ProviderConfig{
		Name:     ProviderOpenAI,
		Endpoint: openAIServer.URL,
		APIKey:   "key",
	}
	models, err := pOpenAI.FetchModels()
	if err != nil {
		t.Fatalf("FetchModels failed for OpenAI: %v", err)
	}
	if len(models) != 2 || models[0] != "gpt-4o" || models[1] != "gpt-4-turbo" {
		t.Errorf("unexpected OpenAI models: %v", models)
	}

	geminiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"models": [
				{"name": "models/gemini-2.0-flash"},
				{"name": "models/gemini-1.5-pro"}
			]
		}`))
	}))
	defer geminiServer.Close()

	pGemini := &ProviderConfig{
		Name:     ProviderGemini,
		Endpoint: geminiServer.URL,
		APIKey:   "key",
	}
	models, err = pGemini.FetchModels()
	if err != nil {
		t.Fatalf("FetchModels failed for Gemini: %v", err)
	}
	if len(models) != 2 || models[0] != "gemini-2.0-flash" || models[1] != "gemini-1.5-pro" {
		t.Errorf("unexpected Gemini models: %v", models)
	}
}

func TestProviderManagerLoadSave(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "awas-provider-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "providers.json")

	mgr := NewManager(configPath)
	if mgr.ActiveProfile != "default" {
		t.Errorf("expected default profile, got %s", mgr.ActiveProfile)
	}

	p := &ProviderConfig{
		Name:   ProviderOpenRouter,
		APIKey: "openrouter-key",
		Model:  "meta-llama/llama-3",
	}
	mgr.Profiles["openrouter"] = p
	mgr.ActiveProfile = "openrouter"

	if err := mgr.Save(); err != nil {
		t.Fatalf("failed to save profiles: %v", err)
	}

	mgr2 := NewManager(configPath)
	if mgr2.ActiveProfile != "openrouter" {
		t.Errorf("expected openrouter profile, got %s", mgr2.ActiveProfile)
	}
	p2, ok := mgr2.Profiles["openrouter"]
	if !ok || p2.APIKey != "openrouter-key" || p2.Model != "meta-llama/llama-3" {
		t.Errorf("loaded profile mismatch: %v", p2)
	}
}
