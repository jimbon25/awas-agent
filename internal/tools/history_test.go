package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUndoRedoPipeline(t *testing.T) {
	SetCurrentSessionID("test-session-123")
	defer SetCurrentSessionID("")

	workspaceA, err := os.MkdirTemp("", "awas-history-test-a")
	if err != nil {
		t.Fatalf("failed to create workspace A: %v", err)
	}
	defer os.RemoveAll(workspaceA)

	workspaceB, err := os.MkdirTemp("", "awas-history-test-b")
	if err != nil {
		t.Fatalf("failed to create workspace B: %v", err)
	}
	defer os.RemoveAll(workspaceB)

	_ = ClearHistory()

	filePath := "test.txt"
	absPathA := filepath.Join(workspaceA, filePath)

	err = RecordChange(workspaceA, filePath, "write_file", "created", nil, []byte("version 1"))
	if err != nil {
		t.Fatalf("RecordChange failed: %v", err)
	}
	_ = os.WriteFile(absPathA, []byte("version 1"), 0644)

	err = RecordChange(workspaceA, filePath, "edit_file", "modified", []byte("version 1"), []byte("version 2"))
	if err != nil {
		t.Fatalf("RecordChange failed: %v", err)
	}
	_ = os.WriteFile(absPathA, []byte("version 2"), 0644)

	absPathB := filepath.Join(workspaceB, "other.txt")
	err = RecordChange(workspaceB, "other.txt", "write_file", "created", nil, []byte("project b content"))
	if err != nil {
		t.Fatalf("RecordChange failed: %v", err)
	}
	_ = os.WriteFile(absPathB, []byte("project b content"), 0644)

	msg, err := Undo(workspaceA, 1)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	t.Log(msg)

	contentA, err := os.ReadFile(absPathA)
	if err != nil {
		t.Fatalf("failed to read file A: %v", err)
	}
	if string(contentA) != "version 1" {
		t.Errorf("expected content 'version 1', got '%s'", string(contentA))
	}

	contentB, err := os.ReadFile(absPathB)
	if err != nil {
		t.Fatalf("failed to read file B: %v", err)
	}
	if string(contentB) != "project b content" {
		t.Errorf("expected project B content to be untouched, got '%s'", string(contentB))
	}

	msg, err = Undo(workspaceA, 1)
	if err != nil {
		t.Fatalf("second Undo failed: %v", err)
	}
	t.Log(msg)

	if _, err := os.Stat(absPathA); !os.IsNotExist(err) {
		t.Error("expected test.txt in Workspace A to be deleted")
	}

	msg, err = Redo(workspaceA, 1)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	t.Log(msg)

	contentA, err = os.ReadFile(absPathA)
	if err != nil {
		t.Fatalf("failed to read file A: %v", err)
	}
	if string(contentA) != "version 1" {
		t.Errorf("expected content 'version 1' after redo, got '%s'", string(contentA))
	}

	msg, err = Redo(workspaceA, 1)
	if err != nil {
		t.Fatalf("second Redo failed: %v", err)
	}
	t.Log(msg)

	contentA, err = os.ReadFile(absPathA)
	if err != nil {
		t.Fatalf("failed to read file A: %v", err)
	}
	if string(contentA) != "version 2" {
		t.Errorf("expected content 'version 2' after second redo, got '%s'", string(contentA))
	}
}
