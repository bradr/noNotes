package indexer

import (
	"database/sql"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/bradr/noNotes/internal/git"
)

type Indexer struct {
	db *sql.DB
}

type Task struct {
	LineNum   int
	Timestamp int64
	Text      string
	Context   string
	Done      bool
}

func New(dbPath string) (*Indexer, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	idx := &Indexer{db: db}
	if err := idx.initSchema(); err != nil {
		return nil, err
	}

	return idx, nil
}

func (idx *Indexer) Close() error {
	return idx.db.Close()
}

func (idx *Indexer) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS timeline (
		line_num INTEGER PRIMARY KEY,
		timestamp INTEGER,
		created_at INTEGER,
		text TEXT
	);

	CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		line_num INTEGER,
		timestamp INTEGER,
		created_at INTEGER,
		text TEXT,
		context TEXT,
		done BOOLEAN
	);
	`
	_, err := idx.db.Exec(schema)
	if err != nil {
		return err
	}

	// Migrations for existing databases
	_, _ = idx.db.Exec("ALTER TABLE timeline ADD COLUMN created_at INTEGER")
	_, _ = idx.db.Exec("ALTER TABLE tasks ADD COLUMN created_at INTEGER")
	_, _ = idx.db.Exec("ALTER TABLE tasks ADD COLUMN done BOOLEAN")

	return nil
}

func canonicalize(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "- [ ] ")
	s = strings.TrimPrefix(s, "- [x] ")
	return s
}

func (idx *Indexer) UpdateTimeline(blameLines []git.BlameLine) error {
	// 1. Fetch current created_at mappings to preserve them
	oldCreated := make(map[string]int64)
	rows, err := idx.db.Query("SELECT text, created_at FROM timeline")
	if err == nil {
		for rows.Next() {
			var txt string
			var ca int64
			if err := rows.Scan(&txt, &ca); err == nil {
				oldCreated[canonicalize(txt)] = ca
			}
		}
		rows.Close()
	}

	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("DELETE FROM timeline")
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO timeline (line_num, timestamp, created_at, text) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, b := range blameLines {
		ca := b.Timestamp
		if old, ok := oldCreated[canonicalize(b.Text)]; ok {
			ca = old
		}
		// Enforce: Created date cannot be after modified date
		if ca > b.Timestamp {
			ca = b.Timestamp
		}
		_, err = stmt.Exec(b.LineNum, b.Timestamp, ca, b.Text)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Regex to find uncompleted tasks
var taskRegex = regexp.MustCompile(`^- \[([ x])\] (.*)$`)

func (idx *Indexer) UpdateTasks(blameLines []git.BlameLine) error {
	// 1. Fetch current created_at mappings for tasks
	oldCreated := make(map[string]int64)
	rows, err := idx.db.Query("SELECT text, created_at FROM tasks")
	if err == nil {
		for rows.Next() {
			var txt string
			var ca int64
			if err := rows.Scan(&txt, &ca); err == nil {
				oldCreated[txt] = ca // Task text is already stripped of [ ]
			}
		}
		rows.Close()
	}

	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("DELETE FROM tasks")
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO tasks (line_num, timestamp, created_at, text, context, done) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, b := range blameLines {
		matches := taskRegex.FindStringSubmatch(strings.TrimSpace(b.Text))
		if len(matches) > 2 {
			isDone := matches[1] == "x"
			taskText := matches[2]
			// Find context: nearest heading above
			context := ""
			for j := i - 1; j >= 0; j-- {
				lineText := strings.TrimSpace(blameLines[j].Text)
				if strings.HasPrefix(lineText, "#") {
					context = strings.TrimLeft(lineText, "# ")
					break
				}
			}

			ca := b.Timestamp
			if old, ok := oldCreated[taskText]; ok {
				ca = old
			}
			// Enforce: Created date cannot be after modified date
			if ca > b.Timestamp {
				ca = b.Timestamp
			}

			_, err = stmt.Exec(b.LineNum, b.Timestamp, ca, taskText, context, isDone)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (idx *Indexer) GetTasks() ([]Task, error) {
	rows, err := idx.db.Query("SELECT line_num, timestamp, text, context, done FROM tasks WHERE done = 0 ORDER BY line_num ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.LineNum, &t.Timestamp, &t.Text, &t.Context, &t.Done); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

type TimelineRow struct {
	LineNum   int    `json:"line_num"`
	Timestamp int64  `json:"timestamp"`
	CreatedAt int64  `json:"created_at"`
	Text      string `json:"text"`
}

func (idx *Indexer) GetTimeline() ([]TimelineRow, error) {
	rows, err := idx.db.Query("SELECT line_num, timestamp, created_at, text FROM timeline ORDER BY line_num ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TimelineRow
	for rows.Next() {
		var t TimelineRow
		if err := rows.Scan(&t.LineNum, &t.Timestamp, &t.CreatedAt, &t.Text); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, nil
}

func (idx *Indexer) GetLine(lineNum int) (TimelineRow, error) {
	var t TimelineRow
	err := idx.db.QueryRow("SELECT line_num, timestamp, created_at, text FROM timeline WHERE line_num = ?", lineNum).
		Scan(&t.LineNum, &t.Timestamp, &t.CreatedAt, &t.Text)
	return t, err
}

func (idx *Indexer) UpdateLineDate(lineNum int, createdAt int64, modifiedAt int64) error {
	_, err := idx.db.Exec("UPDATE timeline SET created_at = ?, timestamp = ? WHERE line_num = ?", createdAt, modifiedAt, lineNum)
	return err
}

// UpdateTimelineFromText rebuilds the timeline table from raw file content
// without git blame. Timestamps are preserved for lines whose text matches an
// existing entry; new lines get the current time. gitCommit() will later
// overwrite with precise blame timestamps.
func (idx *Indexer) UpdateTimelineFromText(content string) error {
	type entry struct{ ts, ca int64 }
	old := make(map[string]entry)
	rows, err := idx.db.Query("SELECT text, timestamp, created_at FROM timeline")
	if err == nil {
		for rows.Next() {
			var txt string
			var e entry
			if err := rows.Scan(&txt, &e.ts, &e.ca); err == nil {
				old[txt] = e
			}
		}
		rows.Close()
	}

	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err = tx.Exec("DELETE FROM timeline"); err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO timeline (line_num, timestamp, created_at, text) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for i, line := range strings.Split(content, "\n") {
		ts, ca := now, now
		if e, ok := old[line]; ok {
			ts, ca = e.ts, e.ca
		}
		if _, err = stmt.Exec(i+1, ts, ca, line); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// UpdateTasksFromText parses tasks directly from file content without needing
// git blame. Returns true if the task list changed (added, removed, or toggled).
func (idx *Indexer) UpdateTasksFromText(content string) (bool, error) {
	type existing struct {
		lineNum int
		done    bool
		ca      int64
	}
	old := make(map[string]existing)
	rows, err := idx.db.Query("SELECT text, line_num, done, created_at FROM tasks")
	if err == nil {
		for rows.Next() {
			var txt string
			var e existing
			if err := rows.Scan(&txt, &e.lineNum, &e.done, &e.ca); err == nil {
				old[txt] = e
			}
		}
		rows.Close()
	}

	type newTask struct {
		lineNum int
		text    string
		context string
		done    bool
	}
	now := time.Now().Unix()
	var incoming []newTask
	ctx := ""
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			ctx = strings.TrimLeft(trimmed, "# ")
			continue
		}
		matches := taskRegex.FindStringSubmatch(trimmed)
		if len(matches) < 3 {
			continue
		}
		incoming = append(incoming, newTask{
			lineNum: i + 1,
			text:    matches[2],
			context: ctx,
			done:    matches[1] == "x",
		})
	}

	// Detect changes: different count, or any task added/removed/toggled/moved.
	changed := len(incoming) != len(old)
	if !changed {
		for _, t := range incoming {
			e, exists := old[t.text]
			if !exists || e.done != t.done || e.lineNum != t.lineNum {
				changed = true
				break
			}
		}
	}

	if !changed {
		return false, nil
	}

	tx, err := idx.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	if _, err = tx.Exec("DELETE FROM tasks"); err != nil {
		return false, err
	}

	stmt, err := tx.Prepare("INSERT INTO tasks (line_num, timestamp, created_at, text, context, done) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		return false, err
	}
	defer stmt.Close()

	for _, t := range incoming {
		ca := now
		if e, ok := old[t.text]; ok {
			ca = e.ca
		}
		if _, err = stmt.Exec(t.lineNum, now, ca, t.text, t.context, t.done); err != nil {
			return false, err
		}
	}

	return true, tx.Commit()
}

