# noNotes

noNotes is a minimalist, append-only note-taking application designed for a distraction-free writing experience. It features local SQLite-backed search, automatic file-system synchronization, and a Git-based history for every note.

## Features

- **Minimalist Editor**: Focused on writing with seamless formatting.
- **Append-Only Workflow**: Encourages continuous thought capture.
- **Visual Timeline**: Uses a calendar to navigate your notes history.
- **Git Integration**: Every change is tracked via an internal Git repository. The app automatically initializes a new repository if one isn't found.
- **Auto-Initialization**: On first run, a default `notes.md` is created and the Git repository/SQLite database are automatically configured.
- **Real-Time Sync**: Automatically reflects changes made externally to your `notes.md` file.


## Tech Stack

- **Backend**: [Go](https://go.dev/)
- **Database**: [SQLite](https://www.sqlite.org/) (for indexing tasks and search)
- **History Control**: [Git](https://git-scm.com/)
- **Frontend**: HTML, Vanilla JavaScript, CSS
- **Testing**: [Playwright](https://playwright.dev/) for E2E, Go `testing` for unit tests.

---

## 🚀 Getting Started

### Running with Docker

1. **Build the image**:
   ```bash
   docker build -t nonotes .
   ```

2. **Run with persistence**:
   To ensure your notes and metadata survive container restarts, mount a local directory for your notes and a file for the SQLite database:
   ```bash
   docker run -p 8080:8080 \
     -v $(pwd)/notes:/app/notes \
     -v $(pwd)/singlenote.db:/app/singlenote.db \
     nonotes
   ```

### Running Locally

#### Prerequisites

- [Go 1.24+](https://go.dev/dl/)
- [Git](https://git-scm.com/downloads) installed and reachable in your `PATH`.
- (Optional) [Node.js](https://nodejs.org/) and `npm` for running E2E tests.

#### Running Locally

1. **Start the server**:
   ```bash
   make run
   ```
   The application will start on [http://localhost:8080](http://localhost:8080).

2. **Develop**:
   Changes are automatically indexed from the `notes/` directory.
---

## 🧪 Testing

The project is split into Go unit tests and Playwright E2E tests.

### Go Backend Tests
```bash
make test
```

### E2E Playwright Tests
Ensure you have installed the dependencies in the `e2e` directory first:
```bash
cd e2e && npm install
npx playwright install
cd ..
make test-e2e
```

---

## 📁 Project Structure

- `cmd/singlenote/`: Application entry point.
- `internal/`: Core business logic.
  - `api/`: HTTP server and routes.
  - `git/`: Git interaction logic for history and blame.
  - `indexer/`: SQLite indexing for tasks and full-text search.
  - `watcher/`: File system watcher for syncing `notes.md`.
- `web/`: Frontend assets (HTML templates and static files).
- `e2e/`: Playwright E2E tests and reproduction scripts.
- `scripts/`: Development utilities (e.g., history generation).
- `notes/`: (Ignored) Your local notes storage and Git repo.

---

## License
MIT
