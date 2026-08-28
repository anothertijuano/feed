package main

import (
	"crypto/sha1"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func shaB64(pass string) string {
	sum := sha1.Sum([]byte(pass))
	return "{SHA}" + base64.StdEncoding.EncodeToString(sum[:])
}

func writeHtpasswd(t *testing.T, entries []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "htpasswd")
	if err := os.WriteFile(path, []byte(strings.Join(entries, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHtpasswdCheck(t *testing.T) {
	bcryptHash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	h, err := LoadHtpasswd(writeHtpasswd(t, []string{
		"alice:" + string(bcryptHash),
		"bob:" + shaB64("shapass"),
		"carol:plainpass",
		"# comment line",
		"broken-line",
	}))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		user, pass string
		want       bool
	}{
		{"alice", "secret", true},
		{"alice", "wrong", false},
		{"bob", "shapass", true},
		{"bob", "nope", false},
		{"carol", "plainpass", true},
		{"carol", "x", false},
		{"nobody", "secret", false},
		{"", "secret", false},
	}
	for _, tc := range cases {
		if got := h.Check(tc.user, tc.pass); got != tc.want {
			t.Errorf("Check(%q, %q) = %v, want %v", tc.user, tc.pass, got, tc.want)
		}
	}
}

func TestHtpasswdHotReload(t *testing.T) {
	path := writeHtpasswd(t, []string{"alice:first-password-with-a-longer-length"})
	h, err := LoadHtpasswd(path)
	if err != nil {
		t.Fatal(err)
	}
	if !h.Check("alice", "first-password-with-a-longer-length") {
		t.Fatal("initial password rejected")
	}
	if h.Check("bob", "whatever") {
		t.Fatal("bob should not exist yet")
	}

	// Rewrite the file with a different size so the mtime/size check fires.
	if err := os.WriteFile(path, []byte("alice:first-password-with-a-longer-length\nbob:second-password-with-a-longer-length\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !h.Check("bob", "second-password-with-a-longer-length") {
		t.Fatal("reload did not pick up bob")
	}
}

func TestAuthMiddleware(t *testing.T) {
	bcryptHash, _ := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.MinCost)
	h, err := LoadHtpasswd(writeHtpasswd(t, []string{"admin:" + string(bcryptHash)}))
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := OpenTokenStore(filepath.Join(t.TempDir(), "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := tokens.Create("test")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /api/feed", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := requireAuth(h, tokens, "feed", mux)

	get := func(path string, mutate func(*http.Request)) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if mutate != nil {
			mutate(req)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	// The UI shell is public so the PWA can always load.
	for _, path := range []string{"/", "/manifest.json", "/api/health"} {
		if code := get(path, nil); code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200 (public)", path, code)
		}
	}

	// The API requires credentials.
	if code := get("/api/feed", nil); code != http.StatusUnauthorized {
		t.Fatalf("GET /api/feed without auth = %d, want 401", code)
	}

	// Bearer token.
	if code := get("/api/feed", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+raw)
	}); code != http.StatusOK {
		t.Fatal("valid bearer token rejected")
	}
	if code := get("/api/feed", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer ft_deadbeef")
	}); code != http.StatusUnauthorized {
		t.Fatal("invalid bearer token accepted")
	}

	// Basic credentials still work.
	if code := get("/api/feed", func(r *http.Request) {
		r.SetBasicAuth("admin", "hunter2")
	}); code != http.StatusOK {
		t.Fatal("valid basic credentials rejected")
	}
	if code := get("/api/feed", func(r *http.Request) {
		r.SetBasicAuth("admin", "wrong")
	}); code != http.StatusUnauthorized {
		t.Fatal("wrong basic credentials accepted")
	}
}

func TestAuthOpenWhenUnconfigured(t *testing.T) {
	tokens, err := OpenTokenStore(filepath.Join(t.TempDir(), "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/feed", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := requireAuth(nil, tokens, "feed", mux)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/feed", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (open when nothing is configured)", rec.Code)
	}
}

func TestCORSMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := corsMiddleware(inner)

	// Preflight is answered without touching the inner handler.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/api/feed", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Allow-Origin = %q", got)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/feed", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Allow-Origin = %q", got)
	}
}
