package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bradr/noNotes/internal/api"
	"github.com/bradr/noNotes/internal/git"
	"github.com/bradr/noNotes/internal/indexer"
	"github.com/bradr/noNotes/internal/watcher"
)

func main() {
	repoPath := "notes"
	fileName := "notes.md"
	dbPath := "singlenote.db"

	if err := git.ConfigureSafeDirectory(repoPath); err != nil {
		log.Printf("Warning: failed to configure git safe directory: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repoPath, ".git")); os.IsNotExist(err) {
		log.Println("Initializing new git repository in", repoPath)
		if err := os.MkdirAll(repoPath, 0755); err != nil {
			log.Fatal(err)
		}
		if err := git.Init(repoPath); err != nil {
			log.Fatalf("Failed to initialize git: %v", err)
		}
	}

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
	defer idx.Close()

	w := watcher.New(repoPath, fileName, idx, 2*time.Second)
	if err := w.Start(); err != nil {
		log.Fatalf("Failed to start watcher: %v", err)
	}

	server, err := api.New(repoPath, fileName, idx, w)
	if err != nil {
		log.Fatalf("Failed to initialize API server: %v", err)
	}

	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: server.SetupRoutes(),
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Println("Starting noNotes server on http://localhost:8080")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-quit
	log.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}
}
