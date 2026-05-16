// Package index provides a SQLite-backed project file index with fsnotify watching.
package index

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	_ "github.com/mattn/go-sqlite3"
)

const schema = `
CREATE TABLE IF NOT EXISTS files (
	path        TEXT PRIMARY KEY,
	content     TEXT,
	modified_at INTEGER
);
`

// Index watches a project directory and maintains a SQLite record of its files.
type Index struct {
	projectPath string
	db          *sql.DB
	watcher     *fsnotify.Watcher
	mu          sync.RWMutex
	stopCh      chan struct{}
}

// New creates an Index for projectPath, opening (or creating) the SQLite
// database at <projectPath>/.forge-index.db.
func New(projectPath string) (*Index, error) {
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, fmt.Errorf("index: cannot resolve path %q: %w", projectPath, err)
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("index: project path does not exist: %w", err)
	}

	dbPath := filepath.Join(abs, ".forge-index.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("index: open db: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("index: create schema: %w", err)
	}

	return &Index{
		projectPath: abs,
		db:          db,
		stopCh:      make(chan struct{}),
	}, nil
}

// Start indexes all existing files and starts the fsnotify watcher.
func (i *Index) Start() error {
	if err := i.scanAll(); err != nil {
		return fmt.Errorf("index: initial scan: %w", err)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("index: create watcher: %w", err)
	}
	i.watcher = w

	// Watch the project root and all subdirectories.
	if err := filepath.WalkDir(i.projectPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			// Skip hidden dirs (.git, .build, etc.)
			if d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return w.Add(path)
		}
		return nil
	}); err != nil {
		w.Close()
		return fmt.Errorf("index: watch dirs: %w", err)
	}

	go i.watchLoop()
	return nil
}

// Stop shuts down the watcher and closes the database.
func (i *Index) Stop() {
	close(i.stopCh)
	if i.watcher != nil {
		i.watcher.Close()
	}
	i.db.Close()
}

// ListFiles returns all relative paths tracked in the index.
func (i *Index) ListFiles() ([]string, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	rows, err := i.db.Query("SELECT path FROM files ORDER BY path")
	if err != nil {
		return nil, fmt.Errorf("index: list files: %w", err)
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// ReadFile returns the content for the given relative path.
func (i *Index) ReadFile(relPath string) (string, error) {
	abs, err := i.safeJoin(relPath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(i.projectPath, abs)
	if err != nil {
		return "", err
	}

	i.mu.RLock()
	defer i.mu.RUnlock()

	var content string
	err = i.db.QueryRow("SELECT content FROM files WHERE path = ?", rel).Scan(&content)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("index: file not found: %s", relPath)
	}
	if err != nil {
		return "", fmt.Errorf("index: read file: %w", err)
	}
	return content, nil
}

// ApplyDiff parses a unified diff and applies it to the file in the index and
// on disk. Returns an error if the diff cannot be applied cleanly.
func (i *Index) ApplyDiff(relPath, unifiedDiff string) error {
	abs, err := i.safeJoin(relPath)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(i.projectPath, abs)
	if err != nil {
		return err
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	// Read current content (may not be in DB yet if newly created).
	var current string
	dbErr := i.db.QueryRow("SELECT content FROM files WHERE path = ?", rel).Scan(&current)
	if dbErr != nil && dbErr != sql.ErrNoRows {
		return fmt.Errorf("index: apply diff read: %w", dbErr)
	}
	if dbErr == sql.ErrNoRows {
		raw, err := os.ReadFile(abs)
		if err != nil {
			current = "" // new file
		} else {
			current = string(raw)
		}
	}

	updated, err := applyUnifiedDiff(current, unifiedDiff)
	if err != nil {
		return fmt.Errorf("index: apply diff: %w", err)
	}

	// Write to disk.
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("index: mkdir for diff: %w", err)
	}
	if err := os.WriteFile(abs, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("index: write after diff: %w", err)
	}

	// Update SQLite.
	now := time.Now().UnixMilli()
	_, err = i.db.Exec(
		"INSERT INTO files(path,content,modified_at) VALUES(?,?,?) ON CONFLICT(path) DO UPDATE SET content=excluded.content, modified_at=excluded.modified_at",
		rel, updated, now,
	)
	return err
}

// ---------- internal helpers ----------

func (i *Index) safeJoin(rel string) (string, error) {
	// Clean the path to remove .. traversal attempts.
	clean := filepath.Clean(rel)
	abs := filepath.Join(i.projectPath, clean)
	// Verify the result is still within the project root.
	if !strings.HasPrefix(abs, i.projectPath+string(filepath.Separator)) && abs != i.projectPath {
		return "", fmt.Errorf("index: path traversal detected: %q", rel)
	}
	return abs, nil
}

func (i *Index) scanAll() error {
	return filepath.WalkDir(i.projectPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		return i.indexFile(path)
	})
}

func (i *Index) indexFile(absPath string) error {
	rel, err := filepath.Rel(i.projectPath, absPath)
	if err != nil {
		return err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil // file may have disappeared
	}

	// Skip large files (> 1 MiB) and binary-ish files.
	if info.Size() > 1<<20 {
		return nil
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}

	// Skip files that look binary (contain null bytes in first 512 bytes).
	sample := content
	if len(sample) > 512 {
		sample = sample[:512]
	}
	for _, b := range sample {
		if b == 0 {
			return nil
		}
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	_, err = i.db.Exec(
		"INSERT INTO files(path,content,modified_at) VALUES(?,?,?) ON CONFLICT(path) DO UPDATE SET content=excluded.content, modified_at=excluded.modified_at",
		rel, string(content), info.ModTime().UnixMilli(),
	)
	return err
}

func (i *Index) watchLoop() {
	for {
		select {
		case <-i.stopCh:
			return
		case event, ok := <-i.watcher.Events:
			if !ok {
				return
			}
			switch {
			case event.Has(fsnotify.Create) || event.Has(fsnotify.Write):
				_ = i.indexFile(event.Name)
				// If a new directory was created, watch it.
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = i.watcher.Add(event.Name)
				}
			case event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename):
				rel, err := filepath.Rel(i.projectPath, event.Name)
				if err == nil {
					i.mu.Lock()
					_, _ = i.db.Exec("DELETE FROM files WHERE path = ?", rel)
					i.mu.Unlock()
				}
			}
		case _, ok := <-i.watcher.Errors:
			if !ok {
				return
			}
			// Log watcher errors but keep running.
		}
	}
}

// applyUnifiedDiff applies a unified diff string to original and returns the
// resulting text. This is a simple line-based implementation that handles the
// common @@-hunk format. It does NOT require the `patch` binary.
func applyUnifiedDiff(original, diff string) (string, error) {
	origLines := strings.Split(original, "\n")

	diffLines := strings.Split(diff, "\n")
	outLines := make([]string, len(origLines))
	copy(outLines, origLines)

	// We track offset: how many lines we have added/removed so far.
	var offset int

	i := 0
	for i < len(diffLines) {
		line := diffLines[i]
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index ") {
			i++
			continue
		}
		if strings.HasPrefix(line, "@@") {
			// Parse: @@ -oldStart,oldCount +newStart,newCount @@
			var oldStart, oldCount, newStart, newCount int
			_, err := fmt.Sscanf(line, "@@ -%d,%d +%d,%d @@", &oldStart, &oldCount, &newStart, &newCount)
			if err != nil {
				// Try single-line form: @@ -l +l @@
				_, err = fmt.Sscanf(line, "@@ -%d +%d @@", &oldStart, &newStart)
				if err != nil {
					i++
					continue
				}
				oldCount = 1
				newCount = 1
			}
			i++

			// Collect hunk lines.
			var hunkOld []string // lines going away (context + removed)
			var hunkNew []string // lines coming in (context + added)
			for i < len(diffLines) {
				hl := diffLines[i]
				if strings.HasPrefix(hl, "@@") || strings.HasPrefix(hl, "---") || strings.HasPrefix(hl, "+++") || strings.HasPrefix(hl, "diff ") {
					break
				}
				switch {
				case strings.HasPrefix(hl, "-"):
					hunkOld = append(hunkOld, hl[1:])
				case strings.HasPrefix(hl, "+"):
					hunkNew = append(hunkNew, hl[1:])
				default: // context line
					ctx := strings.TrimPrefix(hl, " ")
					hunkOld = append(hunkOld, ctx)
					hunkNew = append(hunkNew, ctx)
				}
				i++
			}

			// Apply hunk: replace hunkOld slice in outLines starting at
			// (oldStart-1+offset) with hunkNew.
			start := oldStart - 1 + offset
			if start < 0 {
				start = 0
			}
			if start > len(outLines) {
				start = len(outLines)
			}
			end := start + len(hunkOld)
			if end > len(outLines) {
				end = len(outLines)
			}
			replaced := make([]string, 0, len(outLines)-len(hunkOld)+len(hunkNew))
			replaced = append(replaced, outLines[:start]...)
			replaced = append(replaced, hunkNew...)
			replaced = append(replaced, outLines[end:]...)
			offset += len(hunkNew) - len(hunkOld)
			outLines = replaced
			continue
		}
		i++
	}

	return strings.Join(outLines, "\n"), nil
}
