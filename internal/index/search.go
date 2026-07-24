package index

import "strings"

func SearchSymbols(idx *Index, query string, kind string) []Symbol {
	if idx == nil {
		return nil
	}

	var results []Symbol
	query = strings.ToLower(strings.TrimSpace(query))
	kind = strings.ToLower(strings.TrimSpace(kind))

	for _, sym := range idx.Symbols {
		if kind != "" && strings.ToLower(sym.Kind) != kind {
			continue
		}

		if query != "" {
			nameLower := strings.ToLower(sym.Name)
			if !strings.Contains(nameLower, query) {
				continue
			}
		}

		results = append(results, sym)
	}

	return results
}
