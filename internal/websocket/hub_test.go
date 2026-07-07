package websocket

import (
	"net/http/httptest"
	"testing"
)

func TestCheckOrigin(t *testing.T) {
	tests := []struct {
		name          string
		allowedOrigin string
		requestHost   string
		origin        string
		want          bool
	}{
		{
			name:        "allows same host production origin",
			requestHost: "scoutmark-ba.fly.dev",
			origin:      "https://scoutmark-ba.fly.dev",
			want:        true,
		},
		{
			name:          "allows same host when allowed origin is different",
			allowedOrigin: "https://scoutmark.example.com",
			requestHost:   "scoutmark-ba.fly.dev",
			origin:        "https://scoutmark-ba.fly.dev",
			want:          true,
		},
		{
			name:          "allows exact configured origin",
			allowedOrigin: "https://scoutmark.example.com",
			requestHost:   "scoutmark-ba.fly.dev",
			origin:        "https://scoutmark.example.com",
			want:          true,
		},
		{
			name:        "rejects cross host origin",
			requestHost: "scoutmark-ba.fly.dev",
			origin:      "https://evil.example.com",
			want:        false,
		},
		{
			name:        "rejects non-http origin scheme",
			requestHost: "scoutmark-ba.fly.dev",
			origin:      "wss://scoutmark-ba.fly.dev",
			want:        false,
		},
		{
			name:        "allows localhost development origin",
			requestHost: "127.0.0.1:8080",
			origin:      "http://localhost:5173",
			want:        true,
		},
		{
			name:        "allows empty development origin",
			requestHost: "127.0.0.1:8080",
			origin:      "",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ALLOWED_ORIGIN", tt.allowedOrigin)

			req := httptest.NewRequest("GET", "http://"+tt.requestHost+"/api/ws", nil)
			req.Host = tt.requestHost
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			if got := checkOrigin(req); got != tt.want {
				t.Fatalf("checkOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}
