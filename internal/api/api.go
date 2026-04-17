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

	"github.com/bradr/noNotes/internal/git"
	"github.com/bradr/noNotes/internal/indexer"
	"github.com/bradr/noNotes/internal/watcher"
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
		"highlight": func(text, query string) template.HTML {
			if query == "" {
				return template.HTML(template.HTMLEscapeString(text))
			}
			// Case-insensitive replacement
			lowerText := strings.ToLower(text)
			lowerQuery := strings.ToLower(query)
			
			var result strings.Builder
			lastIdx := 0
			for {
				idx := strings.Index(lowerText[lastIdx:], lowerQuery)
				if idx == -1 {
					result.WriteString(template.HTMLEscapeString(text[lastIdx:]))
					break
				}
				idx += lastIdx
				result.WriteString(template.HTMLEscapeString(text[lastIdx:idx]))
				result.WriteString("<mark class='search-highlight'>")
				result.WriteString(template.HTMLEscapeString(text[idx : idx+len(query)]))
				result.WriteString("</mark>")
				lastIdx = idx + len(query)
			}
			return template.HTML(result.String())
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
	mux.HandleFunc("/line/get", s.handleGetLine)
	mux.HandleFunc("/line/update-date", s.handleUpdateLineDate)
	mux.HandleFunc("/history", s.handleHistory)
	mux.HandleFunc("/diff", s.handleDiff)
	mux.HandleFunc("/revert", s.handleRevert)

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
	query := strings.ToLower(r.URL.Query().Get("q"))
	tasks, err := s.indexer.GetTasks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if query != "" {
		filtered := make([]indexer.Task, 0)
		for _, t := range tasks {
			if strings.Contains(strings.ToLower(t.Text), query) {
				filtered = append(filtered, t)
			}
		}
		tasks = filtered
	}
	s.render(w, "tasks.html", map[string]interface{}{
		"Tasks": tasks,
		"Query": query,
	})
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

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	entries, err := git.History(s.repoPath, filepath.Base(s.filePath))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash")
	if hash == "" {
		http.Error(w, "Hash required", http.StatusBadRequest)
		return
	}
	diff, err := git.Diff(s.repoPath, filepath.Base(s.filePath), hash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(diff))
}

func (s *Server) handleRevert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hash := r.FormValue("hash")
	if hash == "" {
		http.Error(w, "Hash required", http.StatusBadRequest)
		return
	}
	err := git.Undo(s.repoPath, hash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
