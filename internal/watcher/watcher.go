package watcher

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bradr/noNotes/internal/git"
	"github.com/bradr/noNotes/internal/indexer"
	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	repoPath       string
	filePath       string
	indexer        *indexer.Indexer
	debounceTimer  *time.Timer
	debounceWait   time.Duration
	commitTimer    *time.Timer
	commitInterval time.Duration
	mu             sync.Mutex

	clients   map[chan bool]bool
	muClients sync.Mutex
}

func New(repoPath, fileName string, idx *indexer.Indexer, debounce, commitInterval time.Duration) *Watcher {
	return &Watcher{
		repoPath:       repoPath,
		filePath:       filepath.Join(repoPath, fileName),
		indexer:        idx,
		debounceWait:   debounce,
		commitInterval: commitInterval,
		clients:        make(map[chan bool]bool),
	}
}

func (w *Watcher) Start() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Name == w.filePath && (event.Has(fsnotify.Write) || event.Has(fsnotify.Create)) {
					w.triggerDebounce()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("watcher error:", err)
			}
		}
	}()

	if err = watcher.Add(w.repoPath); err != nil {
		return err
	}

	// Full index on startup so timeline timestamps are populated from the start.
	w.commitAndIndex()

	return nil
}

// triggerDebounce resets both timers on every file change:
// - short debounce fires reIndex (tasks + broadcast, no git)
// - commit timer fires gitCommit (git add/commit/blame, timeline timestamps)
func (w *Watcher) triggerDebounce() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
	}
	w.debounceTimer = time.AfterFunc(w.debounceWait, w.reIndex)

	if w.commitTimer != nil {
		w.commitTimer.Stop()
	}
	w.commitTimer = time.AfterFunc(w.commitInterval, w.gitCommit)
}

// reIndex reads the file directly and updates the tasks table without touching
// git. Fast enough to run on every save.
func (w *Watcher) reIndex() {
	content, err := os.ReadFile(w.filePath)
	if err != nil {
		log.Printf("reIndex: read failed: %v", err)
		return
	}
	text := string(content)
	if err := w.indexer.UpdateTimelineFromText(text); err != nil {
		log.Printf("reIndex: UpdateTimelineFromText failed: %v", err)
	}
	tasksChanged, err := w.indexer.UpdateTasksFromText(text)
	if err != nil {
		log.Printf("reIndex: UpdateTasksFromText failed: %v", err)
	}
	w.Broadcast(tasksChanged)
	log.Println("Fast re-index complete.")
}

// gitCommit does a git add+commit, then re-runs blame to refresh the per-line
// timestamps used for hover dates and the timeline.
func (w *Watcher) gitCommit() {
	w.mu.Lock()
	defer w.mu.Unlock()

	fileName := filepath.Base(w.filePath)

	if err := git.Add(w.repoPath, fileName); err != nil {
		log.Printf("Git add failed: %v", err)
	}
	if err := git.Commit(w.repoPath, "Auto-commit from noNotes"); err != nil {
		log.Printf("Git commit: %v (no changes?)", err)
	}

	lines, err := git.Blame(w.repoPath, fileName)
	if err != nil {
		log.Printf("Git blame failed: %v", err)
		return
	}
	if err := w.indexer.UpdateTimeline(lines); err != nil {
		log.Printf("UpdateTimeline failed: %v", err)
	}
	if err := w.indexer.UpdateTasks(lines); err != nil {
		log.Printf("UpdateTasks failed: %v", err)
	}

	w.Broadcast(true)
	log.Println("Git commit and timeline index complete.")
}

// commitAndIndex is the full pipeline used at startup only.
func (w *Watcher) commitAndIndex() {
	w.mu.Lock()
	defer w.mu.Unlock()

	fileName := filepath.Base(w.filePath)

	if err := git.Add(w.repoPath, fileName); err != nil {
		log.Printf("Git add failed: %v", err)
	}
	if err := git.Commit(w.repoPath, "Auto-commit from noNotes"); err != nil {
		log.Printf("Git commit: %v (no changes?)", err)
	}

	lines, err := git.Blame(w.repoPath, fileName)
	if err != nil {
		log.Printf("Git blame failed: %v", err)
		return
	}
	if err := w.indexer.UpdateTimeline(lines); err != nil {
		log.Printf("UpdateTimeline failed: %v", err)
	}
	if err := w.indexer.UpdateTasks(lines); err != nil {
		log.Printf("UpdateTasks failed: %v", err)
	}

	w.Broadcast(true)
	log.Println("Startup index complete.")
}

// ToggleLine flips [ ] ↔ [x] on a specific line under the mutex.
func (w *Watcher) ToggleLine(filePath string, lineNum int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	b, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	if lineNum < 1 || lineNum > len(lines) {
		return fmt.Errorf("line %d out of bounds", lineNum)
	}
	line := lines[lineNum-1]
	switch {
	case strings.Contains(line, "[ ]"):
		lines[lineNum-1] = strings.Replace(line, "[ ]", "[x]", 1)
	case strings.Contains(line, "[x]"):
		lines[lineNum-1] = strings.Replace(line, "[x]", "[ ]", 1)
	default:
		return fmt.Errorf("no checkbox on line %d", lineNum)
	}
	return os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0644)
}

// WriteFile writes content to the notes file under the mutex.
func (w *Watcher) WriteFile(filePath string, content []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return os.WriteFile(filePath, content, 0644)
}

// AppendFile appends text to the notes file under the mutex.
func (w *Watcher) AppendFile(filePath string, text string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}

func (w *Watcher) RegisterClient() chan bool {
	w.muClients.Lock()
	defer w.muClients.Unlock()
	ch := make(chan bool, 1)
	w.clients[ch] = true
	return ch
}

func (w *Watcher) UnregisterClient(ch chan bool) {
	w.muClients.Lock()
	defer w.muClients.Unlock()
	delete(w.clients, ch)
	close(ch)
}

// Broadcast notifies all SSE clients. tasksChanged signals whether the task
// list needs to reload (false = only timeline/editor content changed).
func (w *Watcher) Broadcast(tasksChanged bool) {
	w.muClients.Lock()
	defer w.muClients.Unlock()
	for ch := range w.clients {
		select {
		case ch <- tasksChanged:
		default:
		}
	}
}
