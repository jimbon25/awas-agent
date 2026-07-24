package index

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func SaveIndex(workDir string, idx *Index) error {
	awasDir := filepath.Join(workDir, ".awas")
	if err := os.MkdirAll(awasDir, 0755); err != nil {
		return err
	}

	indexPath := filepath.Join(awasDir, "index.json")
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(indexPath, data, 0644)
}

func LoadIndex(workDir string) (*Index, error) {
	indexPath := filepath.Join(workDir, ".awas", "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}

	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}

	return &idx, nil
}
