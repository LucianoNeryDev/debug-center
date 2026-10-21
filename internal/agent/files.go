package agent

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

func filePath(w http.ResponseWriter, r *http.Request) (string, bool) {
	p := r.URL.Query().Get("path")
	if p == "" || !filepath.IsAbs(p) {
		writeErr(w, http.StatusBadRequest, "path must be absolute")
		return "", false
	}
	return filepath.Clean(p), true
}

// handleFileGet downloads a file, or lists a directory as JSON.
func (s *Server) handleFileGet(w http.ResponseWriter, r *http.Request) {
	p, ok := filePath(w, r)
	if !ok {
		return
	}
	s.record(r, "file.get", p)
	fi, err := os.Stat(p)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if fi.IsDir() {
		ents, err := os.ReadDir(p)
		if err != nil {
			writeErr(w, http.StatusForbidden, err.Error())
			return
		}
		list := make([]proto.FileEntry, 0, len(ents))
		for _, e := range ents {
			info, err := e.Info()
			if err != nil {
				continue
			}
			list = append(list, proto.FileEntry{
				Name: e.Name(), Size: info.Size(), Mode: info.Mode().String(),
				ModTime: info.ModTime().UTC(), IsDir: e.IsDir(),
			})
		}
		w.Header().Set("X-Dbc-Kind", "dir")
		writeJSON(w, list)
		return
	}
	f, err := os.Open(p)
	if err != nil {
		writeErr(w, http.StatusForbidden, err.Error())
		return
	}
	defer f.Close()
	w.Header().Set("X-Dbc-Kind", "file")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	_, _ = io.Copy(w, f)
}

// handleFilePut uploads the request body to path, replacing it atomically.
func (s *Server) handleFilePut(w http.ResponseWriter, r *http.Request) {
	p, ok := filePath(w, r)
	if !ok {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".dbc-upload-*")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, err := io.Copy(tmp, r.Body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), p)
	}
	if err != nil {
		os.Remove(tmp.Name())
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.record(r, "file.put", p+" ("+strconv.FormatInt(n, 10)+" bytes)")
	writeJSON(w, map[string]int64{"bytes": n})
}
