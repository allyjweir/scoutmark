package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel/attribute"

	"github.com/allyjweir/scoutmark/internal/auth"
	"github.com/allyjweir/scoutmark/internal/database"
	"github.com/allyjweir/scoutmark/internal/tracing"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		allowed := os.Getenv("ALLOWED_ORIGIN")
		origin := r.Header.Get("Origin")

		if allowed != "" {
			// Production: only accept the configured origin
			return origin == allowed
		}
		// Development: allow localhost origins
		return strings.HasPrefix(origin, "http://localhost:") || origin == ""
	},
}

// ─── Message Types ──────────────────────────────────────────────────
// Using JSON over the wire for simplicity in the MVP.
// We'll use protobuf encoding once the proto codegen is wired up.

type ClientMessage struct {
	RequestID string          `json:"request_id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type ServerMessage struct {
	RequestID string `json:"request_id,omitempty"`
	Type      string `json:"type"`
	Payload   any    `json:"payload"`
}

type SaveDraftPayload struct {
	SessionID string            `json:"session_id"`
	PatrolID  string            `json:"patrol_id"`
	Scores    map[string]int    `json:"scores"`
	Comments  map[string]string `json:"comments"`
}

type DraftSavedPayload struct {
	SessionID string    `json:"session_id"`
	PatrolID  string    `json:"patrol_id"`
	SavedAt   time.Time `json:"saved_at"`
}

// DraftUpdatedPayload is broadcast to other users when someone saves a draft score.
// This enables live multiplayer: other users see the new values appear.
type DraftUpdatedPayload struct {
	SessionID string            `json:"session_id"`
	PatrolID  string            `json:"patrol_id"`
	UserID    string            `json:"user_id"`
	UserName  string            `json:"user_name"`
	Scores    map[string]int    `json:"scores"`
	Comments  map[string]string `json:"comments"`
	SavedAt   time.Time         `json:"saved_at"`
}

type PatrolSubmittedPayload struct {
	SessionID       string    `json:"session_id"`
	PatrolID        string    `json:"patrol_id"`
	UserDisplayName string    `json:"user_display_name"`
	SubmittedAt     time.Time `json:"submitted_at"`
}

// SessionFinalisedPayload is broadcast when a user finalises all their scoring.
type SessionFinalisedPayload struct {
	SessionID       string `json:"session_id"`
	UserID          string `json:"user_id"`
	UserDisplayName string `json:"user_display_name"`
	EndsAt          string `json:"ends_at"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type SubscribeSessionPayload struct {
	SessionID string `json:"session_id"`
}

type PresencePayload struct {
	SessionID    string `json:"session_id"`
	PatrolID     string `json:"patrol_id"`
	CommentingOn string `json:"commenting_on,omitempty"` // criterion_id being commented on (empty = not commenting)
}

type PresenceUpdatedPayload struct {
	SessionID    string `json:"session_id"`
	PatrolID     string `json:"patrol_id"`
	UserID       string `json:"user_id"`
	UserName     string `json:"user_name"`
	CommentingOn string `json:"commenting_on,omitempty"`
}

// PresenceEntry is a single user's presence state.
type PresenceEntry struct {
	UserID       string `json:"user_id"`
	UserName     string `json:"user_name"`
	PatrolID     string `json:"patrol_id"`
	CommentingOn string `json:"commenting_on,omitempty"`
}

// PresenceStatePayload is sent to a client with the full list of other users present in a session.
type PresenceStatePayload struct {
	SessionID string          `json:"session_id"`
	Users     []PresenceEntry `json:"users"`
}

type PatrolProgressPayload struct {
	PatrolID   string `json:"patrol_id"`
	PatrolName string `json:"patrol_name"`
	Status     string `json:"status"`
}

type UserProgressPayload struct {
	UserID      string                  `json:"user_id"`
	DisplayName string                  `json:"display_name"`
	Patrols     []PatrolProgressPayload `json:"patrols"`
}

type ProgressUpdatedPayload struct {
	SessionID string                `json:"session_id"`
	Users     []UserProgressPayload `json:"users"`
}

// ─── Client ─────────────────────────────────────────────────────────

// Client represents a single WebSocket connection.
type Client struct {
	hub          *Hub
	conn         *websocket.Conn
	user         *auth.AuthUser
	send         chan ServerMessage
	sessions     map[string]bool // subscribed session IDs
	sessionsMu   sync.RWMutex
	presence     map[string]string // session_id → patrol_id currently viewing
	commentingOn map[string]string // session_id → criterion_id being commented on
	presenceMu   sync.RWMutex
}

func (c *Client) subscribeTo(sessionID string) {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()
	c.sessions[sessionID] = true
}

func (c *Client) isSubscribedTo(sessionID string) bool {
	c.sessionsMu.RLock()
	defer c.sessionsMu.RUnlock()
	return c.sessions[sessionID]
}

func (c *Client) setPresence(sessionID, patrolID, commentingOn string) {
	c.presenceMu.Lock()
	defer c.presenceMu.Unlock()
	c.presence[sessionID] = patrolID
	if commentingOn != "" {
		c.commentingOn[sessionID] = commentingOn
	} else {
		delete(c.commentingOn, sessionID)
	}
}

func (c *Client) getPresence(sessionID string) (patrolID string, commentingOn string, ok bool) {
	c.presenceMu.RLock()
	defer c.presenceMu.RUnlock()
	patrolID, ok = c.presence[sessionID]
	commentingOn = c.commentingOn[sessionID]
	return
}

// ─── Hub ────────────────────────────────────────────────────────────

// Hub maintains the set of active clients and broadcasts messages.
type Hub struct {
	db         *database.DB
	clients    map[*Client]bool
	clientsMu  sync.RWMutex
	register   chan *Client
	unregister chan *Client
}

// NewHub creates a new WebSocket hub.
func NewHub(db *database.DB) *Hub {
	return &Hub{
		db:         db,
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub's main loop. Should be called in a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clientsMu.Lock()
			h.clients[client] = true
			h.clientsMu.Unlock()

		case client := <-h.unregister:
			h.clientsMu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.clientsMu.Unlock()
		}
	}
}

// GetSessionPresence returns the list of presence entries for a session,
// optionally excluding one client.
func (h *Hub) GetSessionPresence(sessionID string, exclude *Client) []PresenceEntry {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	var entries []PresenceEntry
	for c := range h.clients {
		if c == exclude {
			continue
		}
		if !c.isSubscribedTo(sessionID) {
			continue
		}
		if patrolID, commentingOn, ok := c.getPresence(sessionID); ok {
			entries = append(entries, PresenceEntry{
				UserID:       c.user.ID,
				UserName:     c.user.DisplayName,
				PatrolID:     patrolID,
				CommentingOn: commentingOn,
			})
		}
	}
	return entries
}

// BroadcastToSession sends a message to all clients subscribed to a session,
// optionally excluding one client.
func (h *Hub) BroadcastToSession(sessionID string, msg ServerMessage, exclude *Client) {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	subscribers := lo.Filter(lo.Keys(h.clients), func(c *Client, _ int) bool {
		return c != exclude && c.isSubscribedTo(sessionID)
	})

	lo.ForEach(subscribers, func(c *Client, _ int) {
		select {
		case c.send <- msg:
		default:
			// Client buffer full; disconnect
			h.clientsMu.RUnlock()
			h.clientsMu.Lock()
			delete(h.clients, c)
			close(c.send)
			h.clientsMu.Unlock()
			h.clientsMu.RLock()
		}
	})
}

// BroadcastSessionProgress fetches the current scoring progress for a session
// and broadcasts it to all subscribed clients.
func (h *Hub) BroadcastSessionProgress(ctx context.Context, sessionID string) {
	progress, err := h.db.GetSessionProgress(ctx, sessionID)
	if err != nil {
		tracing.RecordError(ctx, err)
		return
	}

	// Group progress rows by user
	userMap := make(map[string]*UserProgressPayload)
	var userOrder []string
	for _, row := range progress {
		up, exists := userMap[row.UserID]
		if !exists {
			up = &UserProgressPayload{
				UserID:      row.UserID,
				DisplayName: row.DisplayName,
			}
			userMap[row.UserID] = up
			userOrder = append(userOrder, row.UserID)
		}
		up.Patrols = append(up.Patrols, PatrolProgressPayload{
			PatrolID:   row.PatrolID,
			PatrolName: row.PatrolName,
			Status:     row.Status,
		})
	}

	users := make([]UserProgressPayload, 0, len(userOrder))
	for _, id := range userOrder {
		users = append(users, *userMap[id])
	}

	h.BroadcastToSession(sessionID, ServerMessage{
		Type: "progress_updated",
		Payload: ProgressUpdatedPayload{
			SessionID: sessionID,
			Users:     users,
		},
	}, nil)
}

// BroadcastCommentUpdated sends a comment_updated message to all session subscribers.
// This satisfies the CommentBroadcaster interface used by handlers.
func (h *Hub) BroadcastCommentUpdated(sessionID string, payload any, exclude any) {
	h.BroadcastToSession(sessionID, ServerMessage{
		Type:    "comment_updated",
		Payload: payload,
	}, nil) // broadcast to everyone including the sender for consistency
}

// BroadcastSessionFinalised sends a session_finalised message to all session subscribers.
func (h *Hub) BroadcastSessionFinalised(sessionID, userID, displayName, endsAt string) {
	h.BroadcastToSession(sessionID, ServerMessage{
		Type: "session_finalised",
		Payload: SessionFinalisedPayload{
			SessionID:       sessionID,
			UserID:          userID,
			UserDisplayName: displayName,
			EndsAt:          endsAt,
		},
	}, nil)
}

// HandleWebSocket handles the WebSocket upgrade and message loop.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		tracing.RecordError(r.Context(), err)
		return
	}

	client := &Client{
		hub:          h,
		conn:         conn,
		user:         user,
		send:         make(chan ServerMessage, 256),
		sessions:     make(map[string]bool),
		presence:     make(map[string]string),
		commentingOn: make(map[string]string),
	}

	h.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		var msg ClientMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			c.sendError(msg.RequestID, "INVALID_MESSAGE", "could not parse message")
			continue
		}

		c.handleMessage(msg)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleMessage(msg ClientMessage) {
	ctx := context.Background()
	ctx, span := tracing.Tracer().Start(ctx, fmt.Sprintf("ws.%s", msg.Type))
	defer span.End()

	tracing.AddUserAttrs(ctx, c.user.ID, c.user.DisplayName)
	span.SetAttributes(attribute.String("ws.request_id", msg.RequestID))

	switch msg.Type {
	case "save_draft":
		c.handleSaveDraft(ctx, msg)
	case "subscribe_session":
		c.handleSubscribeSession(ctx, msg)
	case "presence":
		c.handlePresence(ctx, msg)
	default:
		c.sendError(msg.RequestID, "UNKNOWN_TYPE", fmt.Sprintf("unknown message type: %s", msg.Type))
	}
}

func (c *Client) handleSaveDraft(ctx context.Context, msg ClientMessage) {
	var payload SaveDraftPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		c.sendError(msg.RequestID, "INVALID_PAYLOAD", "could not parse save_draft payload")
		return
	}

	tracing.AddSessionAttrs(ctx, payload.SessionID, payload.PatrolID)

	// Verify the session is open for scoring
	session, err := c.hub.db.GetSession(ctx, payload.SessionID)
	if err != nil {
		c.sendError(msg.RequestID, "SESSION_NOT_FOUND", "session not found")
		return
	}
	if !session.AcceptsSubmissions() {
		c.sendError(msg.RequestID, "SESSION_NOT_ACTIVE", "session is not open for scoring")
		return
	}

	// Verify the user owns this patrol
	owns, err := c.hub.db.UserOwnsPatrol(ctx, c.user.ID, payload.PatrolID)
	if err != nil {
		tracing.RecordError(ctx, err)
		c.sendError(msg.RequestID, "INTERNAL_ERROR", "could not verify patrol ownership")
		return
	}
	if !owns {
		c.sendError(msg.RequestID, "FORBIDDEN", "you are not assigned to this patrol")
		return
	}

	span := tracing.Tracer()
	_, saveSpan := span.Start(ctx, "ws.save_draft.db")
	_, err = c.hub.db.SaveDraft(ctx, c.user.ID, payload.SessionID, payload.PatrolID, payload.Scores, payload.Comments)
	saveSpan.End()

	if err != nil {
		tracing.RecordError(ctx, err)
		c.sendError(msg.RequestID, "SAVE_FAILED", "could not save draft")
		return
	}

	c.send <- ServerMessage{
		RequestID: msg.RequestID,
		Type:      "draft_saved",
		Payload: DraftSavedPayload{
			SessionID: payload.SessionID,
			PatrolID:  payload.PatrolID,
			SavedAt:   time.Now(),
		},
	}

	// Broadcast the draft update to all OTHER session subscribers (live multiplayer)
	c.hub.BroadcastToSession(payload.SessionID, ServerMessage{
		Type: "draft_updated",
		Payload: DraftUpdatedPayload{
			SessionID: payload.SessionID,
			PatrolID:  payload.PatrolID,
			UserID:    c.user.ID,
			UserName:  c.user.DisplayName,
			Scores:    payload.Scores,
			Comments:  payload.Comments,
			SavedAt:   time.Now(),
		},
	}, c) // exclude the sender

	// Broadcast updated progress to all session subscribers (admin dashboard etc.)
	c.hub.BroadcastSessionProgress(ctx, payload.SessionID)
}

func (c *Client) handleSubscribeSession(ctx context.Context, msg ClientMessage) {
	var payload SubscribeSessionPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		c.sendError(msg.RequestID, "INVALID_PAYLOAD", "could not parse subscribe_session payload")
		return
	}

	c.subscribeTo(payload.SessionID)

	// Send back the current presence state for this session
	entries := c.hub.GetSessionPresence(payload.SessionID, c)
	if entries == nil {
		entries = []PresenceEntry{}
	}
	c.send <- ServerMessage{
		Type: "presence_state",
		Payload: PresenceStatePayload{
			SessionID: payload.SessionID,
			Users:     entries,
		},
	}

	c.send <- ServerMessage{
		RequestID: msg.RequestID,
		Type:      "subscribed",
		Payload:   map[string]string{"session_id": payload.SessionID},
	}
}

func (c *Client) handlePresence(ctx context.Context, msg ClientMessage) {
	var payload PresencePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		c.sendError(msg.RequestID, "INVALID_PAYLOAD", "could not parse presence payload")
		return
	}

	// Track this client's presence
	c.setPresence(payload.SessionID, payload.PatrolID, payload.CommentingOn)

	// Send back the current presence state to the sender so they stay in sync
	entries := c.hub.GetSessionPresence(payload.SessionID, c)
	if entries == nil {
		entries = []PresenceEntry{}
	}
	c.send <- ServerMessage{
		Type: "presence_state",
		Payload: PresenceStatePayload{
			SessionID: payload.SessionID,
			Users:     entries,
		},
	}

	// Broadcast presence to all OTHER session subscribers
	c.hub.BroadcastToSession(payload.SessionID, ServerMessage{
		Type: "presence_updated",
		Payload: PresenceUpdatedPayload{
			SessionID:    payload.SessionID,
			PatrolID:     payload.PatrolID,
			UserID:       c.user.ID,
			UserName:     c.user.DisplayName,
			CommentingOn: payload.CommentingOn,
		},
	}, c)
}

func (c *Client) sendError(requestID, code, message string) {
	c.send <- ServerMessage{
		RequestID: requestID,
		Type:      "error",
		Payload:   ErrorPayload{Code: code, Message: message},
	}
}
