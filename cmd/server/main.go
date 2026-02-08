package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
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

	// ─── WebSocket Hub ──────────────────────────────────────────
	hub := websocket.NewHub(db)
	go hub.Run()

	// ─── Handlers ───────────────────────────────────────────────
	authHandler := handlers.NewAuthHandler(db)
	sessionHandler := handlers.NewSessionHandler(db, hub)
	authMiddleware := auth.Middleware(db)

	// ─── Routes ─────────────────────────────────────────────────
	mux := http.NewServeMux()

	// Auth routes (unauthenticated)
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)

	// Auth routes (authenticated)
	mux.Handle("POST /api/auth/logout", authMiddleware(http.HandlerFunc(authHandler.Logout)))
	mux.Handle("GET /api/auth/me", authMiddleware(http.HandlerFunc(authHandler.GetCurrentUser)))

	// Session routes (authenticated)
	mux.Handle("GET /api/sessions", authMiddleware(http.HandlerFunc(sessionHandler.ListSessions)))
	mux.Handle("GET /api/sessions/{id}", authMiddleware(http.HandlerFunc(sessionHandler.GetSession)))
	mux.Handle("GET /api/sessions/{session_id}/patrols/{patrol_id}/draft", authMiddleware(http.HandlerFunc(sessionHandler.GetDraft)))
	mux.Handle("POST /api/sessions/{session_id}/patrols/{patrol_id}/submit", authMiddleware(http.HandlerFunc(sessionHandler.SubmitScores)))
	mux.Handle("POST /api/sessions/{session_id}/finalise", authMiddleware(http.HandlerFunc(sessionHandler.FinaliseSession)))
	mux.Handle("POST /api/sessions/{session_id}/revise", authMiddleware(http.HandlerFunc(sessionHandler.ReviseSession)))
	mux.Handle("GET /api/sessions/{session_id}/submissions", authMiddleware(http.HandlerFunc(sessionHandler.ListSubmissions)))
	mux.Handle("GET /api/sessions/{session_id}/patrols/{patrol_id}/scores", authMiddleware(http.HandlerFunc(sessionHandler.GetSubmissionScores)))

	// Admin routes
	mux.Handle("POST /api/submissions/{id}/unlock", authMiddleware(auth.RequireAdmin(http.HandlerFunc(sessionHandler.UnlockSubmission))))
	mux.Handle("GET /api/admin/sessions/{session_id}/progress", authMiddleware(auth.RequireAdmin(http.HandlerFunc(sessionHandler.GetSessionProgress))))

	// WebSocket
	mux.Handle("GET /api/ws", authMiddleware(http.HandlerFunc(hub.HandleWebSocket)))

	// Serve frontend static files (in production, built React app)
	mux.Handle("/", http.FileServer(http.Dir("frontend/dist")))

	// ─── CORS middleware for development ────────────────────────
	handler := corsMiddleware(tracing.HTTPMiddleware(mux))

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

	fmt.Printf("Scoutmark v%s starting on %s\n", version, addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
