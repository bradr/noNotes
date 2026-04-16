package git

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type BlameLine struct {
	Timestamp int64
	Text      string
	LineNum   int
}

func Add(repoPath, file string) error {
	cmd := exec.Command("git", "add", file)
	cmd.Dir = repoPath
	return cmd.Run()
}

func Commit(repoPath, msg string) error {
	cmd := exec.Command("git", "commit", "-m", msg)
	cmd.Dir = repoPath
	return cmd.Run()
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
