package indexer

import (
	"database/sql"
	"regexp"
	"strings"

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

