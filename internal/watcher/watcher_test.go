package watcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/bradr/noNotes/internal/indexer"
)

func TestWatcherDebounce(t *testing.T) {
	tmpDir := t.TempDir()
	fileName := "notes.md"

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	cmd.Run()

	// Create initial file
	err := os.WriteFile(filepath.Join(tmpDir, fileName), []byte("# Notes\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	idx, err := indexer.New(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to initialize indexer: %v", err)
	}

	// Create watcher with very short debounce
	w := New(tmpDir, fileName, idx, 50*time.Millisecond)

	updateChan := w.RegisterClient()
	defer w.UnregisterClient(updateChan)

	w.triggerDebounce()
	
	// Wait for debounce + git commands to complete
	time.Sleep(1000 * time.Millisecond)

	// Simulate event
	select {
	case <-updateChan:
		// Success
	default:
		t.Error("Expected an update on UpdateChan after debounce, but got none")
	}
}
