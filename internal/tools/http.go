package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func HTTPRequest(method, url, headersJSON, body string) string {
	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(method)

	if url == "" {
		return "[Error] URL is required"
	}

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return fmt.Sprintf("[Error] failed to create request: %v", err)
	}

	if headersJSON != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(headersJSON), &headers); err != nil {
			return fmt.Sprintf("[Error] failed to parse headers JSON: %v", err)
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	}

	if body != "" && req.Header.Get("Content-Type") == "" {
		if strings.HasPrefix(body, "{") || strings.HasPrefix(body, "[") {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("[Error] request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("[Error] failed to read response: %v", err)
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Status: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode)))
	result.WriteString(fmt.Sprintf("Content-Type: %s\n", resp.Header.Get("Content-Type")))

	bodyStr := string(respBody)
	if len(bodyStr) > 5000 {
		bodyStr = bodyStr[:5000] + "... (truncated)"
	}

	var prettyJSON bytes.Buffer
	if json.Indent(&prettyJSON, []byte(bodyStr), "", "  ") == nil {
		result.WriteString(fmt.Sprintf("\nBody:\n%s", prettyJSON.String()))
	} else {
		result.WriteString(fmt.Sprintf("\nBody:\n%s", bodyStr))
	}

	return result.String()
}
