package watcher

import (
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/bradr/singlenote/internal/git"
	"github.com/bradr/singlenote/internal/indexer"
	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	repoPath      string
	filePath      string
	indexer       *indexer.Indexer
	debounceTimer *time.Timer
	debounceWait  time.Duration
	mu            sync.Mutex
	
	clients       map[chan struct{}]bool
	muClients     sync.Mutex
}

func New(repoPath, fileName string, idx *indexer.Indexer, debounce time.Duration) *Watcher {
	return &Watcher{
		repoPath:     repoPath,
		filePath:     filepath.Join(repoPath, fileName),
		indexer:      idx,
		debounceWait: debounce,
		clients:      make(map[chan struct{}]bool),
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
				// We only care about edits/creates to our specific file
				if event.Name == w.filePath && (event.Has(fsnotify.Write) || event.Has(fsnotify.Create)) {
					w.triggerDebounce()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			}
		}
	}()

	err = watcher.Add(w.repoPath) // Watch the directory to catch file creation
	if err != nil {
		return err
	}
	
	// Run initial index on start
	w.commitAndIndex()

	return nil
}

func (w *Watcher) triggerDebounce() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
	}

	w.debounceTimer = time.AfterFunc(w.debounceWait, func() {
		w.commitAndIndex()
	})
}

func (w *Watcher) commitAndIndex() {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	// 1. Git add and commit
	err := git.Add(w.repoPath, "notes.md") // HARDCODED for now based on PRD
	if err != nil {
		log.Printf("Git add failed: %v", err)
	}
	
	err = git.Commit(w.repoPath, "Auto-commit from SingleNote")
	if err != nil {
		// Can happen if nothing changed, that's fine
		log.Printf("Git commit: %v (might mean no changes)", err)
	}

	// 2. Git Blame over the file
	lines, err := git.Blame(w.repoPath, "notes.md")
	if err != nil {
		log.Printf("Git blame failed: %v", err)
		return
	}

	// 3. Update SQLite Indexes
	err = w.indexer.UpdateTimeline(lines)
	if err != nil {
		log.Printf("UpdateTimeline failed: %v", err)
	}

	err = w.indexer.UpdateTasks(lines)
	if err != nil {
		log.Printf("UpdateTasks failed: %v", err)
	}
	
	// 4. Notify all frontends
	w.Broadcast()

	log.Println("Successfully indexed file changes.")
}

func (w *Watcher) RegisterClient() chan struct{} {
	w.muClients.Lock()
	defer w.muClients.Unlock()
	ch := make(chan struct{}, 1)
	w.clients[ch] = true
	return ch
}

func (w *Watcher) UnregisterClient(ch chan struct{}) {
	w.muClients.Lock()
	defer w.muClients.Unlock()
	delete(w.clients, ch)
	close(ch)
}

func (w *Watcher) Broadcast() {
	w.muClients.Lock()
	defer w.muClients.Unlock()
	for ch := range w.clients {
		select {
		case ch <- struct{}{}:
		default:
		}
	}

	log.Println("Successfully indexed file changes.")
}
