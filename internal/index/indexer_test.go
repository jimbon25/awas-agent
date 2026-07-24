package index

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIndexPipeline(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "awas-index-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}

	goFileContent := `package src

// MyStruct represents a test struct.
type MyStruct struct {
	Value string
}

// MyFunction is a test function.
func MyFunction() string {
	return "hello"
}
`
	goFilePath := filepath.Join(srcDir, "main.go")
	if err := os.WriteFile(goFilePath, []byte(goFileContent), 0644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}

	gitignoreContent := `
# Ignore build outputs
ignored_file.go
ignored_dir/
`
	if err := os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte(gitignoreContent), 0644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	ignoredGoFilePath := filepath.Join(tempDir, "ignored_file.go")
	if err := os.WriteFile(ignoredGoFilePath, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to write ignored file: %v", err)
	}

	idx, err := BuildIndex(tempDir)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	if len(idx.Files) != 1 {
		t.Errorf("expected 1 file info, got %d", len(idx.Files))
	} else {
		if idx.Files[0].Path != "src/main.go" {
			t.Errorf("expected file path 'src/main.go', got '%s'", idx.Files[0].Path)
		}
	}

	if len(idx.Symbols) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(idx.Symbols))
	} else {
		structSym := idx.Symbols[0]
		if structSym.Name != "MyStruct" || structSym.Kind != "struct" {
			t.Errorf("unexpected struct symbol: %+v", structSym)
		}
		funcSym := idx.Symbols[1]
		if funcSym.Name != "MyFunction" || funcSym.Kind != "function" {
			t.Errorf("unexpected func symbol: %+v", funcSym)
		}
	}

	results := SearchSymbols(idx, "MyFunc", "")
	if len(results) != 1 {
		t.Errorf("expected 1 search result, got %d", len(results))
	} else if results[0].Name != "MyFunction" {
		t.Errorf("expected search result 'MyFunction', got '%s'", results[0].Name)
	}

	err = SaveIndex(tempDir, idx)
	if err != nil {
		t.Fatalf("SaveIndex failed: %v", err)
	}

	loadedIdx, err := LoadIndex(tempDir)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	if loadedIdx.Root != idx.Root {
		t.Errorf("expected loaded root '%s', got '%s'", idx.Root, loadedIdx.Root)
	}
	if len(loadedIdx.Symbols) != len(idx.Symbols) {
		t.Errorf("expected loaded symbols count %d, got %d", len(idx.Symbols), len(loadedIdx.Symbols))
	}
}
