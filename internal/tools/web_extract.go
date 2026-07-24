package tools

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
)

func WebExtract(urlStr, cssSelector string) string {
	if urlStr == "" {
		return "[Error] url is required"
	}
	if cssSelector == "" {
		return "[Error] selector is required"
	}

	selectors, err := parseSelectorChain(cssSelector)
	if err != nil {
		return fmt.Sprintf("[Error] %v", err)
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

	if resp.StatusCode >= 400 {
		return fmt.Sprintf("[Error] server returned status %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return fmt.Sprintf("[Error] failed to parse HTML: %v", err)
	}

	matches := findMatchingElements(doc, selectors)

	items := make([]string, 0, len(matches))
	for _, n := range matches {
		text := strings.TrimSpace(textContent(n))
		if text != "" {
			items = append(items, text)
		}
	}

	if len(items) == 0 {
		return fmt.Sprintf("[Error] no elements matched selector %q on %s", cssSelector, urlStr)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Extracted %d item(s) for selector %q from %s:\n", len(items), cssSelector, urlStr))
	b.WriteString(strings.Repeat("─", 40) + "\n")
	for i, item := range items {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
	}
	return b.String()
}

type selectorPart struct {
	tag   string 
	class string 
	id    string 
}

func parseSelectorChain(sel string) ([]selectorPart, error) {
	parts := strings.Fields(sel) 
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty selector")
	}
	result := make([]selectorPart, 0, len(parts))
	for _, p := range parts {
		sp := selectorPart{}
		tokens := strings.FieldsFunc(p, func(r rune) bool { return r == '.' || r == '#' })
		markers := splitMarkers(p)
		if len(tokens) == 0 {
			return nil, fmt.Errorf("invalid selector part %q", p)
		}
		sp.tag = tokens[0]
		for _, m := range markers {
			switch m.kind {
			case '#':
				sp.id = m.value
			case '.':
				if sp.class == "" {
					sp.class = m.value
				} else {
					sp.class = sp.class + " " + m.value
				}
			}
		}
		result = append(result, sp)
	}
	return result, nil
}

type marker struct {
	kind  byte
	value string
}

func splitMarkers(p string) []marker {
	var markers []marker
	var cur strings.Builder
	var curKind byte
	flush := func() {
		if curKind != 0 {
			markers = append(markers, marker{kind: curKind, value: cur.String()})
			cur.Reset()
		}
	}
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c == '.' || c == '#' {
			flush()
			curKind = c
		} else {
			cur.WriteByte(c)
		}
	}
	flush()
	return markers
}

func findMatchingElements(root *html.Node, chain []selectorPart) []*html.Node {
	var results []*html.Node
	var walk func(n *html.Node, depth int)
	walk = func(n *html.Node, depth int) {
		if depth >= len(chain) {
			return
		}
		part := chain[depth]
		if matchesPart(n, part) {
			if depth == len(chain)-1 {
				results = append(results, n)
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c, depth+1)
			}
		} else {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c, depth)
			}
		}
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		walk(c, 0)
	}
	return results
}

func matchesPart(n *html.Node, part selectorPart) bool {
	if n.Type != html.ElementNode {
		return false
	}
	if part.tag != "" && part.tag != n.Data {
		return false
	}
	if part.id != "" {
		if attr(n, "id") != part.id {
			return false
		}
	}
	if part.class != "" {
		nodeClasses := strings.Fields(attr(n, "class"))
		for _, want := range strings.Fields(part.class) {
			found := false
			for _, have := range nodeClasses {
				if have == want {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(c *html.Node) {
		if c.Type == html.ElementNode && (c.Data == "script" || c.Data == "style") {
			return
		}
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
		for child := c.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return b.String()
}
