package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bradr/singlenote/internal/indexer"
	// Using empty watcher as it's harder to mock and not strictly needed for toggle/append if we don't test SSE here
	"github.com/bradr/singlenote/internal/watcher"
)

func TestHandleAppend(t *testing.T) {
	tmpDir := t.TempDir()
	fileName := "notes.md"
	fullPath := filepath.Join(tmpDir, fileName)

	// Create initial file
	err := os.WriteFile(fullPath, []byte("# Notes\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	idx, _ := indexer.New(filepath.Join(tmpDir, "test.db"))
	w := &watcher.Watcher{} // minimal

	server, err := New(tmpDir, fileName, idx, w)
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("POST", "/append", nil)
	if err != nil {
		t.Fatal(err)
	}
	
	// Add form data
	req.PostForm = map[string][]string{
		"text": {"Hello World"},
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleAppend)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Read file to verify append
	b, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(b), "Hello World") {
		t.Errorf("Appended text not found in file, got: %s", string(b))
	}
}

func TestHandleToggleTask(t *testing.T) {
	tmpDir := t.TempDir()
	fileName := "notes.md"
	fullPath := filepath.Join(tmpDir, fileName)

	content := "# Notes\n- [ ] Unfinished task\n- [x] Finished task\n"
	err := os.WriteFile(fullPath, []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	idx, _ := indexer.New(filepath.Join(tmpDir, "test.db"))
	w := &watcher.Watcher{}

	server, err := New(tmpDir, fileName, idx, w)
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("POST", "/tasks/toggle", nil)
	if err != nil {
		t.Fatal(err)
	}
	
	req.PostForm = map[string][]string{
		"text": {"- [ ] Unfinished task"},
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleToggleTask)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Verify file was updated
	b, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(b), "- [ ] Unfinished task") {
		t.Errorf("Task was not toggled, file contains: %s", string(b))
	}
	if !strings.Contains(string(b), "- [x] Unfinished task") {
		t.Errorf("Toggled task not found as completed, file contains: %s", string(b))
	}
}
