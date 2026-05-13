package app

import (
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const liveReloadEndpoint = "/__metablog_live_reload"

type liveReloadState struct {
	mu       sync.RWMutex
	next     uint64
	versions map[string]uint64
}

func newLiveReloadState() *liveReloadState {
	return &liveReloadState{
		next:     1,
		versions: make(map[string]uint64),
	}
}

func (s *liveReloadState) MarkUpdated(paths ...string) {
	if s == nil || len(paths) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, path := range paths {
		clean := normalizeReloadPath(path)
		if clean == "" {
			continue
		}
		s.next++
		s.versions[clean] = s.next
	}
}

func (s *liveReloadState) Version(path string) uint64 {
	if s == nil {
		return 0
	}
	clean := normalizeReloadPath(path)
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.versions[clean]
}

type liveReloadHandler struct {
	outDir string
	store  *memStore
	base   http.Handler
	state  *liveReloadState
}

func (h liveReloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == liveReloadEndpoint {
		h.serveVersion(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.base.ServeHTTP(w, r)
		return
	}
	if r.URL.Path != "/" && !strings.HasSuffix(r.URL.Path, "/") && !strings.Contains(filepath.Base(r.URL.Path), ".") {
		h.base.ServeHTTP(w, r)
		return
	}
	initialVersion := h.state.Version(r.URL.Path)
	body, ok := h.readHTML(r.URL.Path)
	if !ok {
		h.base.ServeHTTP(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(injectLiveReloadScript(body, initialVersion)))
}

func (h liveReloadHandler) serveVersion(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = fmt.Fprint(w, h.state.Version(path))
}

func (h liveReloadHandler) readHTML(urlPath string) (string, bool) {
	if h.store != nil {
		data, contentType, ok := h.store.get(urlPath)
		if !ok || !isHTMLContent(urlPath, contentType) {
			return "", false
		}
		return string(data), true
	}
	rel := normalizeReloadPath(urlPath)
	if rel == "" {
		rel = "index.html"
	}
	path := filepath.Join(h.outDir, filepath.FromSlash(rel))
	cleanOut, err := filepath.Abs(h.outDir)
	if err != nil {
		return "", false
	}
	cleanPath, err := filepath.Abs(path)
	if err != nil || !isWithinDir(cleanOut, cleanPath) {
		return "", false
	}
	info, err := os.Stat(cleanPath)
	if err != nil || info.IsDir() {
		return "", false
	}
	if !strings.EqualFold(filepath.Ext(cleanPath), ".html") {
		return "", false
	}
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func injectLiveReloadScript(page string, initialVersion uint64) string {
	if strings.Contains(page, liveReloadEndpoint) {
		return page
	}
	script := `<script>
(function() {
  var endpoint = "` + html.EscapeString(liveReloadEndpoint) + `";
  var currentVersion = "` + strconv.FormatUint(initialVersion, 10) + `";
  function check() {
    fetch(endpoint + "?path=" + encodeURIComponent(window.location.pathname), { cache: "no-store" })
      .then(function(resp) { return resp.text(); })
      .then(function(text) {
        var version = text.trim();
        if (version !== currentVersion) {
          window.location.reload();
        }
      })
      .catch(function() {});
  }
  check();
  window.setInterval(check, 1000);
})();
</script>
`
	idx := strings.LastIndex(strings.ToLower(page), "</body>")
	if idx >= 0 {
		return page[:idx] + script + page[idx:]
	}
	return page + script
}

func normalizeReloadPath(path string) string {
	clean := cleanMemPath(path)
	if clean == "" {
		return "index.html"
	}
	if strings.HasSuffix(path, "/") || !strings.Contains(filepath.Base(clean), ".") {
		return clean + "/index.html"
	}
	return clean
}

func isHTMLContent(path, contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/html") || strings.EqualFold(filepath.Ext(path), ".html") || strings.HasSuffix(path, "/")
}

func isWithinDir(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel))
}
