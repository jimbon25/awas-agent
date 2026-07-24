package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type openAIModelsResp struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

type geminiModelsResp struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func (p *ProviderConfig) FetchModels() ([]string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	endpoint := p.GetEndpoint()
	if endpoint == "" {
		return nil, fmt.Errorf("provider endpoint is empty")
	}

	reqURL := endpoint
	var reqHeaders map[string]string

	switch p.Name {
	case ProviderGemini:
		if strings.Contains(endpoint, "generativelanguage.googleapis.com") || p.Endpoint == "" {
			reqURL = "https://generativelanguage.googleapis.com/v1beta/models"
		} else {
			reqURL = strings.TrimSuffix(endpoint, "/") + "/v1beta/models"
		}
		reqHeaders = map[string]string{
			"x-goog-api-key": p.APIKey,
		}

	default:
		reqURL = strings.TrimSuffix(endpoint, "/") + "/models"
		reqHeaders = p.GetHeaders()
		if p.APIKey != "" && reqHeaders["Authorization"] == "" {
			reqHeaders["Authorization"] = "Bearer " + p.APIKey
		}
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create models request failed: %w", err)
	}

	for k, v := range reqHeaders {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status fetching models: %d", resp.StatusCode)
	}

	var models []string

	if p.Name == ProviderGemini {
		var geminiResp geminiModelsResp
		if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
			return nil, fmt.Errorf("decode Gemini models failed: %w", err)
		}
		for _, m := range geminiResp.Models {
			name := m.Name
			name = strings.TrimPrefix(name, "models/")
			models = append(models, name)
		}
	} else {
		var openAIResp openAIModelsResp
		if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
			return nil, fmt.Errorf("decode OpenAI models failed: %w", err)
		}
		for _, m := range openAIResp.Data {
			models = append(models, m.ID)
		}
	}

	return models, nil
}
