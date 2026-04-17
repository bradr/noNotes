package indexer

import (
	"os"
	"testing"

	"github.com/bradr/noNotes/internal/git"
)

func TestUpdateTasks(t *testing.T) {
	// Create an in-memory db or temporary file
	dbPath := "test_tasks.db"
	defer os.Remove(dbPath)

	idx, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create indexer: %v", err)
	}

	lines := []git.BlameLine{
		{LineNum: 1, Text: "Just a normal line"},
		{LineNum: 2, Text: "- [ ] Buy milk"},
		{LineNum: 3, Text: "- [x] Finished task"},
		{LineNum: 4, Text: "  - [ ] Indented task (currently not supported by regex but let's test)"},
	}

	err = idx.UpdateTasks(lines)
	if err != nil {
		t.Fatalf("UpdateTasks failed: %v", err)
	}

	tasks, err := idx.GetTasks()
	if err != nil {
		t.Fatalf("GetTasks failed: %v", err)
	}

	if len(tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(tasks))
	}

	if len(tasks) > 0 && tasks[0].Text != "- [ ] Buy milk" {
		t.Errorf("Expected '- [ ] Buy milk', got '%s'", tasks[0].Text)
	}
}
