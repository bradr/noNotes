#!/usr/bin/env python3
import os
import subprocess
from datetime import datetime, timedelta
import random

repo_dir = "notes"
file_path = os.path.join(repo_dir, "notes.md")

os.makedirs(repo_dir, exist_ok=True)

subprocess.run(["git", "init"], cwd=repo_dir, check=True)

now = datetime.now()
start_date = now - timedelta(days=365)

entries = [
    ("Idea: build a note taking app that is append-only", False),
    ("Need to buy milk", True),
    ("Met with John #meeting", False),
    ("The quick brown fox jumps over the lazy dog", False),
    ("Follow up with email to Sarah", True),
    ("Read up on SQLite vector indexing", False),
    ("Design the Scrubber UI in Figma #design", False),
    ("Fix the memory leak in Go service", True),
    ("Great weather today! Walked 5 miles.", False),
    ("Draft the PRD for SingleNote #project-greenfield", False),
    ("Explore CodeMirror 6 custom extensions", False),
    ("Order new keyboard", True),
]

# Generate ~100 commits
current_date = start_date
with open(file_path, 'w') as f:
    f.write("# SingleNote History\n\n")

subprocess.run(["git", "add", "notes.md"], cwd=repo_dir, check=True)
subprocess.run(["git", "commit", "-m", "Initial commit", "--date", current_date.isoformat()], cwd=repo_dir, check=True)

for i in range(100):
    # Determine how many days to jump forward (0 to 7)
    days_jump = random.randint(0, 7)
    current_date += timedelta(days=days_jump, hours=random.randint(1, 10))
    if current_date > now:
         break
         
    entry, is_task = random.choice(entries)
    if is_task:
         val = random.choice([" ", " ", "x"]) # mostly open
         line = f"- [{val}] {entry}\n\n"
    else:
         line = f"{entry}\n\n"
         
    with open(file_path, 'a') as f:
         if random.random() > 0.8:
              # Add a timestamp header occasionally
              f.write(f"## {current_date.strftime('%B %d, %Y')}\n")
         f.write(line)
         
    subprocess.run(["git", "add", "notes.md"], cwd=repo_dir, check=True)
    
    # We must set both GIT_AUTHOR_DATE and GIT_COMMITTER_DATE
    env = os.environ.copy()
    env["GIT_AUTHOR_DATE"] = current_date.isoformat()
    env["GIT_COMMITTER_DATE"] = current_date.isoformat()
    
    subprocess.run(["git", "commit", "-m", f"Append entry {i}", "--date", current_date.isoformat()], cwd=repo_dir, env=env, check=True)

print("Fake history generation complete!")
