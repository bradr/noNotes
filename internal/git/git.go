package git

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func ConfigureSafeDirectory(repoPath string) error {
	// git config --global --add safe.directory "*"
	// This is safe to run multiple times
	cmd := exec.Command("git", "config", "--global", "--add", "safe.directory", "*")
	// Use an absolute path if possible, but '*' is easiest for multi-volume setups
	return cmd.Run()
}

func Init(repoPath string) error {
	cmd := exec.Command("git", "init")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		return err
	}

	// Configure git so commit doesn't fail
	for _, args := range [][]string{
		{"config", "user.email", "you@example.com"},
		{"config", "user.name", "noNotes"},
	} {
		c := exec.Command("git", args...)
		c.Dir = repoPath
		if err := c.Run(); err != nil {
			// for global config, Dir might not strictly matter as much but let's keep it consistent
			return err
		}
	}
	return nil
}

type BlameLine struct {
	Timestamp int64
	Text      string
	LineNum   int
}

func Add(repoPath, file string) error {
	cmd := exec.Command("git", "add", file)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := string(bytes.TrimSpace(out))
		if strings.Contains(outStr, "index.lock") {
			os.Remove(filepath.Join(repoPath, ".git", "index.lock"))
			cmd = exec.Command("git", "add", file)
			cmd.Dir = repoPath
			out, err = cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("retry %w: %s", err, string(bytes.TrimSpace(out)))
			}
			return nil
		}
		return fmt.Errorf("%w: %s", err, outStr)
	}
	return nil
}

func Commit(repoPath, msg string) error {
	cmd := exec.Command("git", "commit", "-m", msg)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := string(bytes.TrimSpace(out))
		if strings.Contains(outStr, "index.lock") {
			os.Remove(filepath.Join(repoPath, ".git", "index.lock"))
			cmd = exec.Command("git", "commit", "-m", msg)
			cmd.Dir = repoPath
			out, err = cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("retry %w: %s", err, string(bytes.TrimSpace(out)))
			}
			return nil
		}
		return fmt.Errorf("%w: %s", err, outStr)
	}
	return nil
}

// Blame runs `git blame -p file` returning the birth timestamp of each line.
// We only really care about committer-time for indexing.
func Blame(repoPath, file string) ([]BlameLine, error) {
	cmd := exec.Command("git", "blame", "-p", file)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git blame failed: %w", err)
	}

	var results []BlameLine
	scanner := bufio.NewScanner(bytes.NewReader(out))

	var currentLineNum int
	var currentSHA string
	commitTimestamps := make(map[string]int64)

	for scanner.Scan() {
		line := scanner.Text()

		if len(line) == 0 {
			continue
		}

		if line[0] == '\t' {
			// This is the actual line content
			results = append(results, BlameLine{
				Timestamp: commitTimestamps[currentSHA],
				Text:      line[1:], // strip tab
				LineNum:   currentLineNum,
			})
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 0 {
			continue
		}

		key := parts[0]

		if len(key) == 40 {
			currentSHA = key
			currentLineNum++
		} else if key == "committer-time" {
			t, _ := strconv.ParseInt(parts[1], 10, 64)
			commitTimestamps[currentSHA] = t
		}
	}

	return results, nil
}

type LogEntry struct {
	Hash      string   `json:"hash"`
	Timestamp int64    `json:"timestamp"`
	Subject   string   `json:"subject"`
	Additions int      `json:"additions"`
	Deletions int      `json:"deletions"`
	Preview   []string `json:"preview"`
}

func History(repoPath, file string) ([]LogEntry, error) {
	// Use -p to get the patch so we can extract previews
	// We use a unique separator to reliably identify commit lines
	const separator = "===COMMIT==="
	cmd := exec.Command("git", "log", "--pretty=format:"+separator+"%H|%at|%s", "-p", file)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	var entries []LogEntry
	scanner := bufio.NewScanner(bytes.NewReader(out))
	var currentEntry *LogEntry

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, separator) {
			line = strings.TrimPrefix(line, separator)
			parts := strings.SplitN(line, "|", 3)
			if len(parts) == 3 {
				ts, _ := strconv.ParseInt(parts[1], 10, 64)
				newEntry := LogEntry{
					Hash:      parts[0],
					Timestamp: ts,
					Subject:   parts[2],
					Preview:   []string{},
				}
				entries = append(entries, newEntry)
				currentEntry = &entries[len(entries)-1]
			}
			continue
		}

		if currentEntry == nil {
			continue
		}

		// Parse diff lines
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			currentEntry.Additions++
			if len(currentEntry.Preview) < 4 {
				currentEntry.Preview = append(currentEntry.Preview, line)
			}
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			currentEntry.Deletions++
			if len(currentEntry.Preview) < 4 {
				currentEntry.Preview = append(currentEntry.Preview, line)
			}
		}
	}

	return entries, nil
}

func Diff(repoPath, file, hash string) (string, error) {
	// git show <hash> -- <file> shows the patch for that file in that commit
	cmd := exec.Command("git", "show", "--patch", hash, "--", file)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git show failed: %w", err)
	}
	return string(out), nil
}

func Undo(repoPath, hash string) error {
	// Revert the specific changes from this commit without making a new commit yet
	cmd := exec.Command("git", "revert", "-n", hash)
	cmd.Dir = repoPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// If revert fails (usually due to conflicts), we abort the revert to leave the repo clean
		exec.Command("git", "revert", "--abort").Run()
		return fmt.Errorf("This change overlaps with more recent edits and cannot be automatically undone")
	}
	return nil
}
