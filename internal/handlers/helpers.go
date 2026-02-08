package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/allyjweir/scoutmark/internal/tracing"
)

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// readJSON decodes a JSON request body into v.
// Limits body size to 1MB to prevent memory exhaustion.
func readJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20) // 1 MB
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, r *http.Request, status int, message string) {
	tracing.RecordError(r.Context(), fmt.Errorf(message))
	writeJSON(w, status, map[string]string{"error": message})
}

// pathParam extracts a path segment by position from the URL.
// e.g., for "/api/sessions/abc123/patrols", pathParam(r, 3) returns "abc123"
func pathParam(r *http.Request, segment int) string {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if segment < len(parts) {
		return parts[segment]
	}
	return ""
}

// Router is a simple HTTP router using the standard library.
// It matches on method + path prefix and delegates to handlers.
type Router struct {
	mux *http.ServeMux
}

// NewRouter creates a new Router.
func NewRouter() *Router {
	return &Router{mux: http.NewServeMux()}
}

// Handle registers a handler for a pattern.
func (rt *Router) Handle(pattern string, handler http.Handler) {
	rt.mux.Handle(pattern, handler)
}

// HandleFunc registers a handler function for a pattern.
func (rt *Router) HandleFunc(pattern string, handler http.HandlerFunc) {
	rt.mux.HandleFunc(pattern, handler)
}

// ServeHTTP implements the http.Handler interface.
func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rt.mux.ServeHTTP(w, r)
}

// methodHandler routes by HTTP method.
type methodHandler struct {
	get    http.HandlerFunc
	post   http.HandlerFunc
	put    http.HandlerFunc
	delete http.HandlerFunc
}

func (m *methodHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if m.get != nil {
			m.get(w, r)
			return
		}
	case http.MethodPost:
		if m.post != nil {
			m.post(w, r)
			return
		}
	case http.MethodPut:
		if m.put != nil {
			m.put(w, r)
			return
		}
	case http.MethodDelete:
		if m.delete != nil {
			m.delete(w, r)
			return
		}
	}
	http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
}
