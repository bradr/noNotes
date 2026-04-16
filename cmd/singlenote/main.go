package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/bradr/singlenote/internal/api"
	"github.com/bradr/singlenote/internal/git"
	"github.com/bradr/singlenote/internal/indexer"
	"github.com/bradr/singlenote/internal/watcher"
)

func main() {
	// Let's ensure the repo paths exist
	repoPath := "notes"
	fileName := "notes.md"
	dbPath := "singlenote.db"

	// 1. Configure safe directory for Docker/Volume scenarios
	if err := git.ConfigureSafeDirectory(repoPath); err != nil {
		log.Printf("Warning: failed to configure git safe directory: %v", err)
	}

	// 2. Create notes dir if it doesn't exist
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); os.IsNotExist(err) {
		log.Println("Initializing new git repository in", repoPath)
		if err := os.MkdirAll(repoPath, 0755); err != nil {
			log.Fatal(err)
		}
		if err := git.Init(repoPath); err != nil {
			log.Fatalf("Failed to initialize git: %v", err)
		}
	}


	// Create notes.md if it doesn't exist
	fullFilePath := filepath.Join(repoPath, fileName)
	if _, err := os.Stat(fullFilePath); os.IsNotExist(err) {
		if err := os.WriteFile(fullFilePath, []byte("# SingleNote Init\n"), 0644); err != nil {
			log.Fatal(err)
		}
	}

	idx, err := indexer.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize indexer: %v", err)
	}

	w := watcher.New(repoPath, fileName, idx, 2*time.Second)
	if err := w.Start(); err != nil {
		log.Fatalf("Failed to start watcher: %v", err)
	}

	server, err := api.New(repoPath, fileName, idx, w)
	if err != nil {
		log.Fatalf("Failed to initialize API server: %v", err)
	}

	handler := server.SetupRoutes()

	log.Println("Starting noNotes server on http://localhost:8080")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal(err)
	}
}
