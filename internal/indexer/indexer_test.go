package indexer

import (
	"os"
	"testing"

	"github.com/bradr/noNotes/internal/git"
)

func TestUpdateTasks(t *testing.T) {
	dbPath := "test_tasks.db"
	defer os.Remove(dbPath)

	idx, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create indexer: %v", err)
	}
	defer idx.Close()

	lines := []git.BlameLine{
		{LineNum: 1, Text: "# Project A"},
		{LineNum: 2, Text: "- [ ] Buy milk"},
		{LineNum: 3, Text: "- [x] Finished task"},
		{LineNum: 4, Text: "  - [ ] Indented task"},
	}

	err = idx.UpdateTasks(lines)
	if err != nil {
		t.Fatalf("UpdateTasks failed: %v", err)
	}

	tasks, err := idx.GetTasks()
	if err != nil {
		t.Fatalf("GetTasks failed: %v", err)
	}

	// 2 uncompleted tasks expected: "Buy milk" and "Indented task"
	if len(tasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(tasks))
	}

	if tasks[0].Text != "Buy milk" || tasks[0].Context != "Project A" {
		t.Errorf("Unexpected task 0: %+v", tasks[0])
	}
}

func TestUpdateTasksFromText(t *testing.T) {
	dbPath := "test_tasks_text.db"
	defer os.Remove(dbPath)

	idx, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create indexer: %v", err)
	}
	defer idx.Close()

	content := "# Goals\n- [ ] Task 1\n- [x] Task 2\n# Backlog\n- [ ] Task 3"
	changed, err := idx.UpdateTasksFromText(content)
	if err != nil {
		t.Fatalf("UpdateTasksFromText failed: %v", err)
	}
	if !changed {
		t.Error("Expected changed to be true")
	}

	tasks, err := idx.GetTasks()
	if err != nil {
		t.Fatalf("GetTasks failed: %v", err)
	}

	if len(tasks) != 2 {
		t.Errorf("Expected 2 uncompleted tasks, got %d", len(tasks))
	}

	if tasks[1].Text != "Task 3" || tasks[1].Context != "Backlog" {
		t.Errorf("Task 3 mismatch: %+v", tasks[1])
	}
}

