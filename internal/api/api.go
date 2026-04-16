package api

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bradr/singlenote/internal/indexer"
	"github.com/bradr/singlenote/internal/watcher"
)

type Server struct {
	repoPath string
	filePath string
	indexer  *indexer.Indexer
	watcher  *watcher.Watcher
}

func New(repoPath, fileName string, idx *indexer.Indexer, w *watcher.Watcher) (*Server, error) {
	return &Server{
		repoPath: repoPath,
		filePath: filepath.Join(repoPath, fileName),
		indexer:  idx,
		watcher:  w,
	}, nil
}

func (s *Server) render(w http.ResponseWriter, name string, data interface{}) {
	tmpl := template.New("").Funcs(template.FuncMap{
		"formatDate": func(ts int64) string {
			if ts == 0 {
				return ""
			}
			return time.Unix(ts, 0).Format("Jan 02")
		},
	})
	tmpl, err := tmpl.ParseGlob("web/templates/*.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	err = tmpl.ExecuteTemplate(w, name, data)
	if err != nil {
		http.Error(w, "Execute error: "+err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/append", s.handleAppend)
	mux.HandleFunc("/update", s.handleUpdate)
	mux.HandleFunc("/tasks", s.handleGetTasks)
	mux.HandleFunc("/tasks/toggle", s.handleToggleTask)
	mux.HandleFunc("/events", s.handleSSE)
	mux.HandleFunc("/timeline", s.handleTimeline)
	mux.HandleFunc("/activity", s.handleActivity)
	mux.HandleFunc("/line/get", s.handleGetLine)
	mux.HandleFunc("/line/update-date", s.handleUpdateLineDate)

	fs := http.FileServer(http.Dir("web/static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	return mux
}

func (s *Server) handleGetLine(w http.ResponseWriter, r *http.Request) {
	lineStr := r.URL.Query().Get("line")
	lineNum, _ := strconv.Atoi(lineStr)
	
	row, err := s.indexer.GetLine(lineNum)
	if err != nil {
		http.Error(w, "Line not found", http.StatusNotFound)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(row)
}

func (s *Server) handleUpdateLineDate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	lineNum, _ := strconv.Atoi(r.FormValue("line"))
	createdAt, _ := strconv.ParseInt(r.FormValue("created_at"), 10, 64)
	modifiedAt, _ := strconv.ParseInt(r.FormValue("modified_at"), 10, 64)
	
	err := s.indexer.UpdateLineDate(lineNum, createdAt, modifiedAt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.WriteHeader(http.StatusOK)
}

// Serves the main SPA / Editor
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.render(w, "index.html", nil)
}

func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	// Let's fetch all lines from timeline table
	rows, err := s.indexer.GetTimeline()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rows)
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	dates, err := s.indexer.GetActivityDates()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dates)
}

func (s *Server) handleAppend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	text := r.FormValue("text")
	if text == "" {
		http.Error(w, "Text is required", http.StatusBadRequest)
		return
	}

	f, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	if _, err := f.WriteString(fmt.Sprintf("\n%s\n", text)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	text := r.FormValue("text")
	if text == "" {
		// Just clear the file if it's empty, or ignore
	}

	err := os.WriteFile(s.filePath, []byte(text), 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGetTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.indexer.GetTasks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "tasks.html", tasks)
}

func (s *Server) handleToggleTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	lineStr := r.FormValue("line")
	if lineStr == "" {
		http.Error(w, "Line number required", http.StatusBadRequest)
		return
	}

	targetLine, err := strconv.Atoi(lineStr)
	if err != nil {
		http.Error(w, "Invalid line number", http.StatusBadRequest)
		return
	}

	b, err := os.ReadFile(s.filePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	lines := strings.Split(string(b), "\n")
	if targetLine < 1 || targetLine > len(lines) {
		http.Error(w, "Line number out of bounds", http.StatusNotFound)
		return
	}

	lineIdx := targetLine - 1
	line := lines[lineIdx]

	if strings.Contains(line, "[ ]") {
		lines[lineIdx] = strings.Replace(line, "[ ]", "[x]", 1)
	} else if strings.Contains(line, "[x]") {
		lines[lineIdx] = strings.Replace(line, "[x]", "[ ]", 1)
	} else {
		http.Error(w, "No checkbox found on target line", http.StatusNotFound)
		return
	}

	// Write back
	err = os.WriteFile(s.filePath, []byte(strings.Join(lines, "\n")), 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	notifyCh := s.watcher.RegisterClient()
	defer s.watcher.UnregisterClient(notifyCh)

	for {
		select {
		case <-notifyCh:
			fmt.Fprintf(w, "event: update\ndata: {}\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
