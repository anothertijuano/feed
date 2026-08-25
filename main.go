package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Static frontend assets, embedded so the binary is self-contained.
//
//go:embed frontend
var embeddedFS embed.FS

// frontendFS roots the embedded FS at the frontend/ directory, so files
// are served at /index.html, /styles.css, … as before.
var frontendFS = func() fs.FS {
	sub, err := fs.Sub(embeddedFS, "frontend")
	if err != nil {
		panic(err)
	}
	return sub
}()

func main() {
	var (
		addr          = flag.String("addr", ":8000", "address to listen on")
		data          = flag.String("data", "data", "directory for the on-disk database")
		refresh       = flag.Duration("refresh", 15*time.Minute, "how often to poll subscriptions")
		htpasswd      = flag.String("htpasswd", "", "path to an htpasswd file (enables HTTP Basic auth)")
		pushThreshold = flag.Float64("push-threshold", 0.3, "rank threshold for push notifications (default policy)")
		notifyAge     = flag.Duration("notify-age", 48*time.Hour, "max item age eligible for push notifications")
		vapidSubject  = flag.String("vapid-subject", "mailto:admin@localhost", "VAPID JWT subject (an email or URL)")
		genVapid      = flag.Bool("gen-vapid", false, "print a VAPID key pair and exit")
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if *genVapid {
		pub, priv, err := GenerateVAPID()
		if err != nil {
			logger.Error("generate vapid", "err", err)
			os.Exit(1)
		}
		fmt.Printf("vapid-public:  %s\nvapid-private: %s\n", pub, priv)
		return
	}

	api, err := newAPI(appOptions{
		addr:          *addr,
		dataDir:       *data,
		refresh:       *refresh,
		htpasswd:      *htpasswd,
		pushThreshold: *pushThreshold,
		notifyAge:     *notifyAge,
		vapidSubject:  *vapidSubject,
		log:           logger,
	})
	if err != nil {
		logger.Error("init", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := api.Run(ctx); err != nil {
		logger.Error("server", "err", err)
		os.Exit(1)
	}
}

func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"dur", time.Since(start).Round(time.Microsecond),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
