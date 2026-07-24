package provider

type ProviderType string

const (
	ProviderOpenAI     ProviderType = "openai"
	ProviderAnthropic  ProviderType = "anthropic"
	ProviderGemini     ProviderType = "gemini"
	ProviderOpenRouter ProviderType = "openrouter"
	ProviderCustom     ProviderType = "custom"
)

type ProviderConfig struct {
	Name     ProviderType      `json:"name"`
	Endpoint string            `json:"endpoint,omitempty"`
	APIKey   string            `json:"api_key"`
	Model    string            `json:"model"`
	Headers  map[string]string `json:"headers,omitempty"`
}

func (p *ProviderConfig) GetEndpoint() string {
	if p.Endpoint != "" {
		return p.Endpoint
	}
	switch p.Name {
	case ProviderOpenAI:
		return "https://api.openai.com/v1"
	case ProviderAnthropic:
		return "https://api.anthropic.com/v1"
	case ProviderGemini:
		return "https://generativelanguage.googleapis.com/v1beta/openai"
	case ProviderOpenRouter:
		return "https://openrouter.ai/api/v1"
	}
	return ""
}

func (p *ProviderConfig) GetAPIKey() string {
	return p.APIKey
}

func (p *ProviderConfig) GetModel() string {
	return p.Model
}

func (p *ProviderConfig) GetHeaders() map[string]string {
	headers := make(map[string]string)
	for k, v := range p.Headers {
		headers[k] = v
	}
	if p.Name == ProviderAnthropic {
		headers["anthropic-version"] = "2023-06-01"
		if p.APIKey != "" {
			headers["x-api-key"] = p.APIKey
		}
	}
	return headers
}
