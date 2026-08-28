package server

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

// fsEntry is one subdirectory in a listDirectory response.
type fsEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// listDirectoryResponse is the GET /api/fs/list response body: the directory
// listed, its parent (empty at the filesystem root), and its immediate
// subdirectories. Only directories are returned — this powers the
// "real workspace" directory picker, nothing more; file contents are never
// exposed.
type listDirectoryResponse struct {
	Path    string    `json:"path"`
	Parent  string    `json:"parent"`
	Entries []fsEntry `json:"entries"`
}

// listDirectoryHandler lists the subdirectories of an absolute path on the
// host (defaulting to the user's home directory), so the browser can offer a
// folder picker for a real workspace. Read-only, directories only.
func listDirectoryHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			path = home
		}

		path = filepath.Clean(path)
		if !filepath.IsAbs(path) {
			writeError(w, http.StatusBadRequest, errors.New("path must be absolute"))
			return
		}

		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			writeError(w, http.StatusNotFound, errors.New("not a directory"))
			return
		}

		dirents, err := os.ReadDir(path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		entries := make([]fsEntry, 0, len(dirents))
		for _, de := range dirents {
			if !de.IsDir() {
				continue
			}
			entries = append(entries, fsEntry{Name: de.Name(), Path: filepath.Join(path, de.Name()), IsDir: true})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

		parent := filepath.Dir(path)
		if parent == path {
			parent = ""
		}

		writeJSON(w, http.StatusOK, listDirectoryResponse{Path: path, Parent: parent, Entries: entries})
	}
}
