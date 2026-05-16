package app

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"MetaBlog/internal/pathutil"
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
	v, ok := s.versions[clean]
	s.mu.RUnlock()
	if ok {
		return v
	}
	if decoded, err := url.PathUnescape(path); err == nil && decoded != path {
		clean = normalizeReloadPath(decoded)
		s.mu.RLock()
		v = s.versions[clean]
		s.mu.RUnlock()
		return v
	}
	return 0
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
	if err != nil || !pathutil.IsWithinDir(cleanOut, cleanPath) {
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
	page = stripLiveReloadScripts(page)
	script := `<script>
(function() {
  var endpoint = "` + html.EscapeString(liveReloadEndpoint) + `";
  var currentVersion = "` + strconv.FormatUint(initialVersion, 10) + `";
  var minReloadInterval = 3000;
  var reloadTimer = null;
  var storageKey = "metablog-live-reload:" + window.location.pathname;
  function lastReloadTime() {
    try {
      return Number(window.sessionStorage.getItem(storageKey)) || 0;
    } catch (err) {
      return 0;
    }
  }
  function markReloadTime() {
    try {
      window.sessionStorage.setItem(storageKey, String(Date.now()));
    } catch (err) {}
  }
  function scheduleReload() {
    if (reloadTimer !== null) {
      return;
    }
    var elapsed = Date.now() - lastReloadTime();
    var delay = Math.max(0, minReloadInterval - elapsed);
    reloadTimer = window.setTimeout(function() {
      markReloadTime();
      window.location.reload();
    }, delay);
  }
  function check() {
    fetch(endpoint + "?path=" + encodeURIComponent(window.location.pathname), { cache: "no-store" })
      .then(function(resp) { return resp.text(); })
      .then(function(text) {
        var version = text.trim();
        if (version !== currentVersion) {
          currentVersion = version;
          scheduleReload();
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

func stripLiveReloadScripts(page string) string {
	lower := strings.ToLower(page)
	offset := 0
	for {
		start := findNextScriptTag(lower, offset)
		if start < 0 {
			return page
		}
		openEndRel := strings.Index(lower[start:], ">")
		if openEndRel < 0 {
			return page
		}
		endRel := strings.Index(lower[start+openEndRel+1:], "</script>")
		if endRel < 0 {
			return page
		}
		end := start + openEndRel + 1 + endRel + len("</script>")
		block := page[start:end]
		if strings.Contains(block, liveReloadEndpoint) {
			page = page[:start] + page[end:]
			lower = strings.ToLower(page)
			offset = start
			continue
		}
		offset = end
	}
}

func findNextScriptTag(s string, offset int) int {
	for offset < len(s) {
		idx := strings.Index(s[offset:], "<script")
		if idx < 0 {
			return -1
		}
		idx += offset
		if isScriptTagStart(s, idx) {
			return idx
		}
		offset = idx + len("<script")
	}
	return -1
}

func isScriptTagStart(s string, start int) bool {
	after := start + len("<script")
	if after >= len(s) {
		return true
	}
	switch s[after] {
	case '>', '/', ' ', '\t', '\n', '\r', '\f':
		return true
	default:
		return false
	}
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
