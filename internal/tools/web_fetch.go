package tools

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

func WebFetch(urlStr string) string {
	if urlStr == "" {
		return "[Error] url is required"
	}

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return fmt.Sprintf("[Error] failed to create request: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AWAS/1.0)")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("[Error] request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("[Error] failed to read response: %v", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Sprintf("[Error] server returned status %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	var content string

	if strings.Contains(contentType, "html") {
		content = extractReadableText(string(body))
	} else {
		content = string(body)
	}

	const maxLen = 20000
	if len(content) > maxLen {
		content = content[:maxLen] + "\n... (content truncated)"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("URL: %s\n", urlStr))
	b.WriteString(fmt.Sprintf("Content-Type: %s\n", resp.Header.Get("Content-Type")))
	b.WriteString(strings.Repeat("─", 40) + "\n")
	b.WriteString(content)
	return b.String()
}

// extractReadableText converts raw HTML into clean visible text by removing
// script/style/head boilerplate and collapsing whitespace.
func extractReadableText(html string) string {
	// Drop <head>...</head> (contains scripts, styles, meta).
	html = regexp.MustCompile(`(?is)<head\b[^>]*>.*?</head>`).ReplaceAllString(html, " ")
	// Drop non-visible blocks. RE2 has no backreferences, so list each tag.
	for _, tag := range []string{"script", "style", "noscript", "svg", "template"} {
		re := regexp.MustCompile(`(?is)<` + tag + `\b[^>]*>.*?</` + tag + `>`)
		html = re.ReplaceAllString(html, " ")
	}
	// Drop all remaining tags.
	html = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(html, " ")
	// Decode a few common HTML entities.
	html = strings.NewReplacer(
		"&nbsp;", " ",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&#39;", "'",
		"&apos;", "'",
	).Replace(html)
	// Collapse whitespace (tabs/newlines/multiple spaces) into single spaces.
	html = regexp.MustCompile(`[ \t\r\n]+`).ReplaceAllString(html, " ")
	return strings.TrimSpace(html)
}
