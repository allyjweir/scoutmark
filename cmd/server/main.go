package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/allyjweir/scoutmark/internal/auth"
	"github.com/allyjweir/scoutmark/internal/database"
	"github.com/allyjweir/scoutmark/internal/handlers"
	"github.com/allyjweir/scoutmark/internal/tracing"
	"github.com/allyjweir/scoutmark/internal/websocket"
)

const version = "0.1.0"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	// ─── Tracing ────────────────────────────────────────────────
	shutdown, err := tracing.Init(ctx, "scoutmark", version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: tracing init failed: %v (continuing without traces)\n", err)
	} else {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			shutdown(shutdownCtx)
		}()
	}

	// ─── Database ───────────────────────────────────────────────
	db, err := database.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(ctx, "migrations"); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// ─── Session Cleanup (expired auth tokens) ─────────────────
	cleanupExpiredSessions(ctx, db)

	// ─── WebSocket Hub ──────────────────────────────────────────
	hub := websocket.NewHub(db)
	go hub.Run()

	// ─── Handlers ───────────────────────────────────────────────
	authHandler := handlers.NewAuthHandler(db)
	sessionHandler := handlers.NewSessionHandler(db, hub)
	reportHandler := handlers.NewReportHandler(db, handlers.LoadLogoPNG())
	authMiddleware := auth.Middleware(db)

	// ─── Routes ─────────────────────────────────────────────────
	mux := http.NewServeMux()

	// ─── Rate limiter (5 login attempts per IP per minute) ────
	loginLimiter := newRateLimiter(5, 1*time.Minute)

	// Auth routes (unauthenticated)
	mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if !loginLimiter.allow(clientIP(r)) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"too many login attempts, try again later"}`))
			return
		}
		authHandler.Login(w, r)
	})

	// Auth routes (authenticated)
	mux.Handle("POST /api/auth/logout", authMiddleware(http.HandlerFunc(authHandler.Logout)))
	mux.Handle("GET /api/auth/me", authMiddleware(http.HandlerFunc(authHandler.GetCurrentUser)))
	mux.Handle("POST /api/auth/change-password", authMiddleware(http.HandlerFunc(authHandler.ChangePassword)))

	// Session routes (authenticated)
	mux.Handle("GET /api/sessions", authMiddleware(http.HandlerFunc(sessionHandler.ListSessions)))
	mux.Handle("GET /api/sessions/{id}", authMiddleware(http.HandlerFunc(sessionHandler.GetSession)))
	mux.Handle("GET /api/sessions/{session_id}/patrols/{patrol_id}/draft", authMiddleware(http.HandlerFunc(sessionHandler.GetDraft)))
	mux.Handle("POST /api/sessions/{session_id}/patrols/{patrol_id}/submit", authMiddleware(http.HandlerFunc(sessionHandler.SubmitScores)))
	mux.Handle("POST /api/sessions/{session_id}/finalise", authMiddleware(http.HandlerFunc(sessionHandler.FinaliseSession)))
	mux.Handle("POST /api/sessions/{session_id}/revise", authMiddleware(http.HandlerFunc(sessionHandler.ReviseSession)))
	mux.Handle("GET /api/sessions/{session_id}/submissions", authMiddleware(http.HandlerFunc(sessionHandler.ListSubmissions)))
	mux.Handle("GET /api/sessions/{session_id}/patrols/{patrol_id}/scores", authMiddleware(http.HandlerFunc(sessionHandler.GetSubmissionScores)))
	mux.Handle("POST /api/sessions/{session_id}/awards", authMiddleware(http.HandlerFunc(sessionHandler.SaveAward)))
	mux.Handle("GET /api/sessions/{session_id}/previous-scores", authMiddleware(http.HandlerFunc(sessionHandler.GetPreviousScores)))
	mux.Handle("GET /api/camp-chief/sessions/{session_id}/progress", authMiddleware(auth.RequireCampChief(http.HandlerFunc(sessionHandler.GetCampChiefSessionProgress))))

	// Per-user comment routes (authenticated)
	mux.Handle("GET /api/sessions/{session_id}/patrols/{patrol_id}/comments", authMiddleware(http.HandlerFunc(sessionHandler.GetDraftComments)))
	mux.Handle("PUT /api/sessions/{session_id}/patrols/{patrol_id}/comments/{criterion_id}", authMiddleware(http.HandlerFunc(sessionHandler.SaveDraftComment)))
	mux.Handle("DELETE /api/sessions/{session_id}/patrols/{patrol_id}/comments/{criterion_id}", authMiddleware(http.HandlerFunc(sessionHandler.DeleteDraftComment)))
	mux.Handle("GET /api/sessions/{session_id}/patrols/{patrol_id}/submitted-comments", authMiddleware(http.HandlerFunc(sessionHandler.GetSubmittedComments)))

	// Report routes (authenticated)
	mux.Handle("GET /api/sessions/{session_id}/report-card", authMiddleware(http.HandlerFunc(reportHandler.GetReportCard)))

	// Admin routes
	mux.Handle("GET /api/admin/users", authMiddleware(auth.RequireAdmin(http.HandlerFunc(authHandler.ListAdminUsers))))
	mux.Handle("POST /api/admin/users", authMiddleware(auth.RequireAdmin(http.HandlerFunc(authHandler.CreateAdminUser))))
	mux.Handle("PUT /api/admin/users/{user_id}/password", authMiddleware(auth.RequireAdmin(http.HandlerFunc(authHandler.ResetAdminUserPassword))))
	mux.Handle("GET /api/admin/subcamps", authMiddleware(auth.RequireAdmin(http.HandlerFunc(authHandler.ListAdminSubcamps))))
	mux.Handle("PUT /api/admin/sessions/{session_id}", authMiddleware(auth.RequireAdmin(http.HandlerFunc(sessionHandler.UpdateAdminSession))))
	mux.Handle("GET /api/admin/sessions/{session_id}/subcamps", authMiddleware(auth.RequireAdmin(http.HandlerFunc(sessionHandler.ListAdminSessionSubcamps))))
	mux.Handle("POST /api/admin/sessions/{session_id}/subcamps/{subcamp_id}/lock", authMiddleware(auth.RequireAdmin(http.HandlerFunc(sessionHandler.LockAdminSessionSubcamp))))
	mux.Handle("POST /api/admin/sessions/{session_id}/subcamps/{subcamp_id}/unlock", authMiddleware(auth.RequireAdmin(http.HandlerFunc(sessionHandler.UnlockAdminSessionSubcamp))))
	mux.Handle("PUT /api/admin/sessions/{session_id}/patrols/{patrol_id}/scores", authMiddleware(auth.RequireAdmin(http.HandlerFunc(sessionHandler.UpdateAdminPatrolScores))))
	mux.Handle("POST /api/submissions/{id}/unlock", authMiddleware(auth.RequireAdmin(http.HandlerFunc(sessionHandler.UnlockSubmission))))
	mux.Handle("POST /api/admin/sessions/{session_id}/lock", authMiddleware(auth.RequireAdminOrCampChief(http.HandlerFunc(sessionHandler.LockSession))))
	mux.Handle("POST /api/admin/sessions/{session_id}/unlock", authMiddleware(auth.RequireAdminOrCampChief(http.HandlerFunc(sessionHandler.UnlockSession))))
	mux.Handle("POST /api/admin/sessions/{session_id}/round2", authMiddleware(auth.RequireAdmin(http.HandlerFunc(sessionHandler.EnsureRound2))))
	mux.Handle("GET /api/admin/sessions/{session_id}/round2/finalists", authMiddleware(auth.RequireAdminOrCampChief(http.HandlerFunc(sessionHandler.GetRound2Finalists))))
	mux.Handle("PUT /api/admin/sessions/{session_id}/round2/finalists/{subcamp_id}", authMiddleware(auth.RequireAdminOrCampChief(http.HandlerFunc(sessionHandler.SetRound2Finalist))))
	mux.Handle("GET /api/admin/sessions/{session_id}/progress", authMiddleware(auth.RequireAdminOrCampChief(http.HandlerFunc(sessionHandler.GetSessionProgress))))
	mux.Handle("GET /api/admin/sessions/{session_id}/comments", authMiddleware(auth.RequireAdmin(http.HandlerFunc(sessionHandler.GetSessionComments))))
	mux.Handle("GET /api/admin/sessions/{session_id}/users/{user_id}/scores", authMiddleware(auth.RequireAdmin(http.HandlerFunc(sessionHandler.GetAdminUserScores))))

	// WebSocket
	mux.Handle("GET /api/ws", authMiddleware(http.HandlerFunc(hub.HandleWebSocket)))

	// Health check (unauthenticated)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			http.Error(w, `{"status":"error"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Serve frontend static files with SPA fallback
	mux.Handle("/", spaHandler("frontend/dist"))

	// ─── Security middleware stack ─────────────────────────────
	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
	handler := securityHeaders(corsMiddleware(tracing.HTTPMiddleware(mux), allowedOrigin))

	// ─── Server ─────────────────────────────────────────────────
	addr := envOrDefault("SERVER_ADDR", ":8080")
	server := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	fmt.Printf("Scoutmark v%s starting on %s (started at %s)\n", version, addr, time.Now().Format("15:04:05 MST"))
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

func corsMiddleware(next http.Handler, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		if allowedOrigin != "" {
			// Production: only allow the configured origin
			if origin == allowedOrigin {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
		} else {
			// Development: allow localhost origins
			if strings.HasPrefix(origin, "http://localhost:") {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// HSTS: instruct browsers to always use HTTPS (1 year)
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// ─── Rate Limiter ───────────────────────────────────────────────

type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		attempts: make(map[string][]time.Time),
		max:      max,
		window:   window,
	}
}

// allow checks if the given key (e.g. IP) is within the rate limit.
func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Prune old entries
	valid := make([]time.Time, 0, len(rl.attempts[key]))
	for _, t := range rl.attempts[key] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.max {
		rl.attempts[key] = valid
		return false
	}

	rl.attempts[key] = append(valid, now)
	return true
}

// clientIP extracts the client IP, respecting X-Forwarded-For from Fly.io's proxy.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// First IP in the chain is the real client
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	// Strip port from RemoteAddr
	host, _, found := strings.Cut(r.RemoteAddr, ":")
	if found {
		return host
	}
	return r.RemoteAddr
}

func cleanupExpiredSessions(ctx context.Context, db *database.DB) {
	// Clean up once at startup
	if n, err := db.DeleteExpiredSessions(ctx); err == nil && n > 0 {
		fmt.Fprintf(os.Stderr, "cleaned up %d expired sessions\n", n)
	}

	// Then periodically every hour
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n, err := db.DeleteExpiredSessions(context.Background()); err == nil && n > 0 {
					fmt.Fprintf(os.Stderr, "cleaned up %d expired sessions\n", n)
				}
			}
		}
	}()
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// spaHandler serves static files from dir, falling back to index.html
// for any path that doesn't match a real file (SPA client-side routing).
func spaHandler(dir string) http.Handler {
	fs := http.Dir(dir)
	fileServer := http.FileServer(fs)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to open the requested file
		path := r.URL.Path
		if path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Check if the file exists
		f, err := fs.Open(path)
		if err != nil {
			// File doesn't exist — serve index.html for SPA routing
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()

		// File exists — serve it normally
		fileServer.ServeHTTP(w, r)
	})
}
