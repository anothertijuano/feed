package main

import (
	"bufio"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// Htpasswd validates HTTP Basic credentials against an Apache-style
// htpasswd file. It supports bcrypt ($2a$/$2b$/$2y$), {SHA} and plaintext
// entries, and hot-reloads the file when it changes on disk.
type Htpasswd struct {
	mu      sync.RWMutex
	path    string
	modTime int64
	size    int64
	users   map[string]string
}

// LoadHtpasswd reads the file at path. The file must exist; it is
// re-read automatically whenever it changes.
func LoadHtpasswd(path string) (*Htpasswd, error) {
	h := &Htpasswd{path: path}
	if err := h.reload(); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *Htpasswd) reload() error {
	f, err := os.Open(h.path)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	users := make(map[string]string)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		user, hash, ok := strings.Cut(line, ":")
		if !ok || user == "" || hash == "" {
			continue
		}
		users[user] = hash
	}
	if err := sc.Err(); err != nil {
		return err
	}

	h.mu.Lock()
	h.users = users
	h.modTime = info.ModTime().UnixNano()
	h.size = info.Size()
	h.mu.Unlock()
	return nil
}

// reloadIfChanged re-reads the file when its mtime/size changed.
func (h *Htpasswd) reloadIfChanged() {
	info, err := os.Stat(h.path)
	if err != nil {
		return
	}
	h.mu.RLock()
	same := info.ModTime().UnixNano() == h.modTime && info.Size() == h.size
	h.mu.RUnlock()
	if !same {
		_ = h.reload()
	}
}

// Check validates a username/password pair.
func (h *Htpasswd) Check(user, pass string) bool {
	if user == "" {
		return false
	}
	h.reloadIfChanged()
	h.mu.RLock()
	hash, ok := h.users[user]
	h.mu.RUnlock()
	if !ok {
		return false
	}

	switch {
	case strings.HasPrefix(hash, "$2a$"), strings.HasPrefix(hash, "$2b$"), strings.HasPrefix(hash, "$2y$"):
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pass)) == nil
	case strings.HasPrefix(hash, "{SHA}"):
		sum := sha1.Sum([]byte(pass))
		want := base64.StdEncoding.EncodeToString(sum[:])
		return subtle.ConstantTimeCompare([]byte(hash[5:]), []byte(want)) == 1
	default:
		// Plaintext (or unsupported scheme — treat literally).
		return subtle.ConstantTimeCompare([]byte(hash), []byte(pass)) == 1
	}
}

// requireBasicAuth wraps a handler with HTTP Basic authentication.
// Static PWA plumbing (manifest, service worker, icons) stays public so
// the browser can fetch it outside the authenticated page context; it
// contains no data. Everything else requires credentials.
func requireBasicAuth(ht *Htpasswd, realm string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || !ht.Check(user, pass) {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm=%q, charset="UTF-8"`, realm))
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isPublicPath lists the paths served without authentication.
func isPublicPath(path string) bool {
	switch {
	case path == "/api/health": // uptime monitors
		return true
	case path == "/manifest.json", path == "/sw.js":
		return true
	case strings.HasPrefix(path, "/icons/"):
		return true
	}
	return false
}

// corsMiddleware adds permissive CORS headers so third-party web clients
// can call the API, and answers preflight requests.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		h.Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
