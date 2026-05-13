package app

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// memStore is a thread-safe in-memory file store that can serve HTTP.
type memStore struct {
	mu    sync.RWMutex
	files map[string]memFile // relative path (slash-separated) -> file content
	dirs  map[string]struct{}
}

type memFile struct {
	data    []byte
	modTime time.Time
}

func newMemStore() *memStore {
	return &memStore{
		files: make(map[string]memFile),
		dirs:  make(map[string]struct{}),
	}
}

// loadDir recursively reads all files from dir into memory.
func (m *memStore) loadDir(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			m.mu.Lock()
			m.dirs[rel] = struct{}{}
			m.mu.Unlock()
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		m.mu.Lock()
		m.files[rel] = memFile{data: data, modTime: info.ModTime()}
		m.markDirsLocked(rel)
		m.mu.Unlock()
		return nil
	})
}

func (m *memStore) get(path string) ([]byte, string, bool) {
	path = cleanMemPath(path)
	m.mu.RLock()
	file, ok := m.files[path]
	m.mu.RUnlock()
	if ok {
		return file.data, mimeByExt(path), true
	}
	// For directory paths, try index.html.
	if path == "" || strings.HasSuffix(path, "/") {
		m.mu.RLock()
		file, ok = m.files[path+"index.html"]
		m.mu.RUnlock()
		if ok {
			return file.data, "text/html; charset=utf-8", true
		}
	}
	// If path has no file extension, also try <path>/index.html (e.g. "about" → "about/index.html").
	if !strings.Contains(filepath.Base(path), ".") {
		m.mu.RLock()
		file, ok = m.files[path+"/index.html"]
		m.mu.RUnlock()
		if ok {
			return file.data, "text/html; charset=utf-8", true
		}
	}
	return nil, "", false
}

func (m *memStore) put(path string, data []byte) {
	m.PutFile(path, data, time.Now())
}

func (m *memStore) PutFile(path string, data []byte, modTime time.Time) {
	path = cleanMemPath(path)
	if modTime.IsZero() {
		modTime = time.Now()
	}
	copied := append([]byte(nil), data...)
	m.mu.Lock()
	m.files[path] = memFile{data: copied, modTime: modTime}
	m.markDirsLocked(path)
	m.mu.Unlock()
}

func (m *memStore) FileFresh(path string, sourceInfo os.FileInfo) bool {
	path = cleanMemPath(path)
	m.mu.RLock()
	file, ok := m.files[path]
	m.mu.RUnlock()
	if !ok || len(file.data) == 0 || sourceInfo == nil {
		return false
	}
	return !sourceInfo.ModTime().After(file.modTime)
}

func (m *memStore) putHTML(path string, content string) {
	m.put(path, []byte(content))
}

func (m *memStore) putAll(pages map[string]string) {
	m.mu.Lock()
	for path, content := range pages {
		clean := cleanMemPath(path)
		m.files[clean] = memFile{data: []byte(content), modTime: time.Now()}
		m.markDirsLocked(clean)
	}
	m.mu.Unlock()
}

func (m *memStore) replaceHTMLPages(previous map[string]struct{}, pages map[string]string) map[string]struct{} {
	cleanPages := make(map[string]string, len(pages))
	for path, content := range pages {
		cleanPages[cleanMemPath(path)] = content
	}
	next := make(map[string]struct{}, len(pages))
	m.mu.Lock()
	for path := range previous {
		clean := cleanMemPath(path)
		if _, keep := cleanPages[clean]; !keep {
			delete(m.files, clean)
		}
	}
	now := time.Now()
	for clean, content := range cleanPages {
		m.files[clean] = memFile{data: []byte(content), modTime: now}
		m.markDirsLocked(clean)
		next[clean] = struct{}{}
	}
	m.mu.Unlock()
	return next
}

func (m *memStore) deleteFiles(paths map[string]struct{}) {
	if len(paths) == 0 {
		return
	}
	m.mu.Lock()
	for path := range paths {
		delete(m.files, cleanMemPath(path))
	}
	m.mu.Unlock()
}

func (m *memStore) markDirsLocked(path string) {
	dir := filepath.ToSlash(filepath.Dir(path))
	for dir != "." && dir != "/" && dir != "" {
		m.dirs[dir] = struct{}{}
		parent := filepath.ToSlash(filepath.Dir(dir))
		if parent == dir {
			break
		}
		dir = parent
	}
	m.dirs[""] = struct{}{}
}

// ServeHTTP implements http.Handler, serving files from memory.
func (m *memStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	urlPath := r.URL.Path
	// Redirect clean directory paths to trailing-slash form.
	if urlPath != "/" && !strings.HasSuffix(urlPath, "/") && !strings.Contains(filepath.Base(urlPath), ".") {
		m.mu.RLock()
		_, hasDir := m.dirs[cleanMemPath(urlPath)]
		m.mu.RUnlock()
		if hasDir {
			http.Redirect(w, r, urlPath+"/", http.StatusMovedPermanently)
			return
		}
	}
	data, contentType, ok := m.get(urlPath)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Write(data)
}

func cleanMemPath(path string) string {
	path = strings.Trim(filepath.ToSlash(path), "/")
	return strings.ReplaceAll(path, "//", "/")
}

func mimeByExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	switch ext {
	case ".svg":
		return "image/svg+xml"
	case ".woff2":
		return "font/woff2"
	case ".woff":
		return "font/woff"
	case ".ttf":
		return "font/ttf"
	default:
		return "application/octet-stream"
	}
}
