package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitAddCommitBlame(t *testing.T) {
	tmpDir := t.TempDir()
	fileName := "test.md"

	// 1. Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	
	// Configure git so commit doesn't fail
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	cmd.Run()

	// 2. Create file
	filePath := filepath.Join(tmpDir, fileName)
	err := os.WriteFile(filePath, []byte("Line 1\nLine 2\n"), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// 3. Test Add
	err = Add(tmpDir, fileName)
	if err != nil {
		t.Fatalf("git add failed: %v", err)
	}

	// 4. Test Commit
	err = Commit(tmpDir, "Initial commit")
	if err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// 5. Test Blame
	lines, err := Blame(tmpDir, fileName)
	if err != nil {
		t.Fatalf("git blame failed: %v", err)
	}

	if len(lines) != 2 {
		t.Errorf("Expected 2 lines from blame, got %d", len(lines))
	}

	if len(lines) > 0 && lines[0].Text != "Line 1" {
		t.Errorf("Expected 'Line 1', got '%s'", lines[0].Text)
	}
}
