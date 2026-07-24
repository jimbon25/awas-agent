package tools

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type WebSearchConfig struct {
	SearXNGURL string 
}

var searchConfig WebSearchConfig

func SetSearchConfig(cfg WebSearchConfig) {
	searchConfig = cfg
}

func WebSearch(query string, maxResults int) string {
	if query == "" {
		return "[Error] query is required"
	}
	if maxResults <= 0 {
		maxResults = 5
	}

	if searchConfig.SearXNGURL != "" {
		results, err := searchSearXNG(query, maxResults)
		if err == nil && len(results) > 0 {
			return formatResults("SearXNG", query, results)
		}
	}

	results, err := searchDuckDuckGo(query, maxResults)
	if err != nil {
		return fmt.Sprintf("[Error] search failed: %v", err)
	}
	return formatResults("DuckDuckGo", query, results)
}

type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

func searchSearXNG(query string, maxResults int) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("%s/search?q=%s&format=json&categories=general",
		strings.TrimRight(searchConfig.SearXNGURL, "/"),
		url.QueryEscape(query),
	)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(searchURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	results := []SearchResult{}

	resultsMatch := regexp.MustCompile(`"results"\s*:\s*\[`).FindStringIndex(string(body))
	if resultsMatch == nil {
		return nil, fmt.Errorf("no results field found")
	}

	bodyStr := string(body)
	titleRe := regexp.MustCompile(`"title"\s*:\s*"([^"]*)"`)
	urlRe := regexp.MustCompile(`"url"\s*:\s*"([^"]*)"`)
	contentRe := regexp.MustCompile(`"content"\s*:\s*"([^"]*)"`)

	titles := titleRe.FindAllStringSubmatch(bodyStr, -1)
	urls := urlRe.FindAllStringSubmatch(bodyStr, -1)
	contents := contentRe.FindAllStringSubmatch(bodyStr, -1)

	for i := 0; i < len(titles) && i < maxResults; i++ {
		r := SearchResult{Title: titles[i][1]}
		if i < len(urls) {
			r.URL = urls[i][1]
		}
		if i < len(contents) {
			r.Snippet = contents[i][1]
		}
		results = append(results, r)
	}

	return results, nil
}

func searchDuckDuckGo(query string, maxResults int) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("https://lite.duckduckgo.com/lite/?q=%s", url.QueryEscape(query))

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AWAS/1.0)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)

	results := []SearchResult{}

	linkRe := regexp.MustCompile(`<a[^>]*class="result-link"[^>]*href="([^"]*)"[^>]*>([^<]*)</a>`)
	snippetRe := regexp.MustCompile(`<td[^>]*class="result-snippet"[^>]*>(.*?)</td>`)

	links := linkRe.FindAllStringSubmatch(html, -1)
	snippets := snippetRe.FindAllStringSubmatch(html, -1)

	for i := 0; i < len(links) && i < maxResults; i++ {
		r := SearchResult{
			URL:   links[i][1],
			Title: strings.TrimSpace(links[i][2]),
		}
		if i < len(snippets) {
			tagRe := regexp.MustCompile(`<[^>]*>`)
			r.Snippet = strings.TrimSpace(tagRe.ReplaceAllString(snippets[i][1], ""))
		}
		results = append(results, r)
	}

	return results, nil
}

func formatResults(source, query string, results []SearchResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🔍 Web Search (%s): %s\n", source, query))
	b.WriteString(strings.Repeat("─", 50) + "\n")

	for i, r := range results {
		b.WriteString(fmt.Sprintf("\n%d. %s\n", i+1, r.Title))
		b.WriteString(fmt.Sprintf("   %s\n", r.URL))
		if r.Snippet != "" {
			snippet := r.Snippet
			if len(snippet) > 150 {
				snippet = snippet[:150] + "..."
			}
			b.WriteString(fmt.Sprintf("   %s\n", snippet))
		}
	}

	b.WriteString(fmt.Sprintf("\n%d results found.", len(results)))
	return b.String()
}
