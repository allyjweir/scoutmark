# Scoutmark — Design Document

## Overview

Scoutmark is a mobile-first web application for scoring scout patrols across configurable criteria. It is designed for two primary use cases: weekly scout meetings and large-scale summer camps.

---

## Use Cases

### Weekly Meetings (MVP)

- 1 user (scout leader)
- 2 patrols
- A new scoring session each week
- Simple, fast scoring during a Tuesday night meeting

### Summer Camp (Primary Target)

- 6 users (one per subcamp)
- ~20 patrols per user
- A new session each morning for patrol inspection
- Users walk through their patrols in physical order, scoring each one sequentially
- UX must be seamless and fast — leaders need to focus on working with young people, not fighting an app

---

## Core Concepts

| Concept | Description |
|---|---|
| **Event** | Top-level grouping of sessions (e.g. "Summer Camp 2026", "Term 1 Meetings") |
| **Patrol** | A scout patrol that can be scored |
| **User** | A login identity representing a subcamp team. One login is shared by everyone within the same subcamp. Users own an ordered list of patrols. |
| **Criteria Template** | A reusable, named set of scoring dimensions. Defined once, attached to many sessions. |
| **Criterion** | A single scoring dimension within a template. Has a title, description, configurable min and max values, and a sort order. |
| **Session** | A time-windowed scoring event. Belongs to an event, uses a criteria template. Has a start and end time that determines its status (upcoming, active, closed). Can optionally reference a previous session for chaining (e.g. consecutive camp days). |
| **Draft** | In-progress scores for a specific (user, session, patrol) combination. Auto-saved over WebSocket. Deleted once submitted. |
| **Submission** | Finalised, locked scores. Created when a user submits a draft. Locked by default; an admin can unlock. |
| **Award** | An optional per-session recognition (e.g. Best Patrol, Most Improved). Auto-calculated from scores, user-overridable. Saved incrementally, locked on finalise. |

---

## User Flow

1. **Login** — Simple username/password. One account per subcamp.
2. **Dashboard** — Shows active sessions (tappable) and upcoming sessions (visible but disabled). Grouped by status.
3. **Open a session** — User sees their ordered patrol list. The app starts at the first unsubmitted patrol.
4. **Score a patrol** — Each criterion is presented as a slider (drag to set score). Scores auto-save as a draft over WebSocket (debounced at 500ms).
5. **Submit** — A prominent "Submit Scores" button finalises the scores for that patrol. Scores are locked. The draft is deleted.
6. **Navigate** — Prev/Next buttons move between patrols. A scrollable patrol strip at the top shows all patrols with completion status. Progress bar shows "X/Y submitted".
7. **Resume** — If the user closes the browser and returns, their in-progress draft is restored from the server.

---

## Key Design Decisions

### User = Subcamp Identity

Rather than modelling users and subcamps separately, a user account *is* the subcamp identity. This avoids a `UserSubcamp` join table and simplifies auth. At weekly meetings there might be just one shared login; at camp there are ~6.

### Criteria Templates

Criteria are not defined per-session. Instead, a reusable template is created (e.g. "Camp Inspection" with 6 criteria) and sessions reference a template. This means the same inspection criteria can be reused across every morning of camp without duplication.

### Session Status from Time

Sessions have `starts_at` and `ends_at` timestamps. Status is computed at query time:
- `now < starts_at` → **UPCOMING**
- `starts_at <= now <= ends_at` → **ACTIVE**
- `now > ends_at` → **CLOSED**

No manual status management needed. Sessions are created ahead of time (e.g. one for each day of camp) and become active/closed automatically.

### Patrol Ordering

Patrols are ordered per-user via the `user_patrols` join table's `sort_order` column. The physical layout of patrols in a subcamp determines this order, making it efficient to walk through them sequentially during scoring.

### Draft Auto-Save over WebSocket

A single WebSocket connection per user session (not per page). When the user adjusts any slider, scores are debounced at 500ms and sent to the server as a draft save. If the connection drops, changes queue locally and replay on reconnect. This means:
- No data loss if the browser crashes or signal drops
- Drafts are always available on the server if the user switches devices
- The "Submit" button does a final HTTP POST (not WebSocket) — this must succeed with confirmed connectivity

### Submissions Are Locked

Once submitted, scores are locked by default. Only an admin can unlock a submission (e.g. if a mistake was discovered). There is no user-facing edit flow for submitted scores.

### Drafts Are Deleted on Submit

Once a draft is promoted to a submission, the draft is deleted. No audit trail of draft history — we keep it simple.

### Sessions Apply to All Users

Every session is visible to every user. There is no session scoping per-user or per-subcamp. This may change in future but is sufficient for both current use cases.

### Incremental Save

All user-generated data is saved to the server incrementally as the user interacts — not batched on form submission. This is a foundational design principle. Drafts auto-save over WebSocket. Award selections save via HTTP as soon as the user changes them. This ensures:
- No data loss from browser crashes, signal drops, or accidental navigation
- Server always has the latest state, enabling seamless device switching
- Explicit "finalise" or "submit" actions are confirmation steps, not data-save steps

### Session Chaining (Previous Session)

Sessions can optionally reference a `previous_session_id`, forming a linked list within an event (e.g. Monday → Tuesday → Wednesday inspections). This enables features like "most improved" calculations by comparing scores across consecutive sessions.

### Awards

Sessions can optionally enable awards: **Best Patrol** (highest total score) and **Most Improved** (biggest net improvement from previous session). Awards are:
- Auto-calculated from patrol scores and presented as pre-selected dropdowns
- User-overridable — the scorer can change the selection if they disagree
- Saved incrementally as the user interacts (see Incremental Save principle)
- Locked in on finalise alongside scores
- Visible to admins on the progress view once a user has finalised

---

## Tech Stack

| Layer | Technology | Notes |
|---|---|---|
| Backend | **Go** (standard library) | Minimal dependencies. `net/http` for routing (Go 1.22+ path params). |
| Functional style | **samber/lo** | Used throughout Go code for map, filter, etc. |
| API definition | **Protobuf** (Buf CLI) | Service definitions for type safety. JSON encoding over HTTP for MVP. |
| Database | **MySQL 8** | Relational schema with foreign keys and indexes. |
| Observability | **OpenTelemetry → Honeycomb** | Traces with rich span attributes on every HTTP request, DB query, and WebSocket message. Follows Charity Majors' philosophy: wide events over logs. |
| Frontend framework | **React + TypeScript** | Vite for bundling. |
| UI library | **GitHub Primer** | `@primer/react` components. |
| Functional style (FE) | **lodash** | Used for debounce, groupBy, keyBy, etc. |
| Routing | **react-router-dom v6** | |
| Real-time | **WebSocket** | JSON messages (protobuf-structured). Auto-reconnect with exponential backoff and offline queue. |

---

## Observability Philosophy

Following Charity Majors' approach to observability:
- **Traces over logs.** Every request, DB query, and WebSocket message is a traced span with rich attributes.
- **Arbitrarily wide events.** Spans carry user ID, session ID, patrol ID, score counts, success/failure flags, and timing — all queryable in Honeycomb.
- **No structured logging framework.** Traces are the primary debugging tool. `fmt.Fprintf` to stderr only for startup errors.

---

## Data Model

```
Event 1──* Session
CriteriaTemplate 1──* Criterion
Session *──1 CriteriaTemplate
Session *──1 Event
Session 0..1──1 Session (previous_session_id)
User 1──* UserPatrol *──1 Patrol
Draft = (User, Session, Patrol) unique
Draft 1──* DraftScore ──1 Criterion
Submission = (User, Session, Patrol) unique
Submission 1──* SubmissionScore ──1 Criterion
SessionAward = (User, Session, AwardType) unique → Patrol
```

All primary keys are UUIDs stored as `VARCHAR(36)`.

---

## Authentication

- Username + bcrypt-hashed password
- Session token (64 hex chars, 32 random bytes) stored in `user_sessions` table
- Token transmitted via:
  - `Authorization: Bearer <token>` header (API calls)
  - `session_token` cookie (browser convenience)
  - `?token=<token>` query parameter (WebSocket upgrade)
- Tokens expire after 30 days
- Admin flag on user record gates admin-only endpoints (e.g. unlock submission)

---

## API Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/auth/login` | No | Login, returns token + user |
| POST | `/api/auth/logout` | Yes | Invalidate session |
| GET | `/api/auth/me` | Yes | Get current user |
| GET | `/api/sessions` | Yes | List sessions (filterable by status) |
| GET | `/api/sessions/{id}` | Yes | Get session with criteria, patrols, submissions |
| GET | `/api/sessions/{sid}/patrols/{pid}/draft` | Yes | Get draft for a patrol in a session |
| POST | `/api/sessions/{sid}/patrols/{pid}/submit` | Yes | Finalise scores |
| GET | `/api/sessions/{sid}/submissions` | Yes | List all submissions for a session |
| POST | `/api/submissions/{id}/unlock` | Admin | Unlock a locked submission |
| GET | `/api/ws` | Yes | WebSocket upgrade |

### WebSocket Messages

**Client → Server:**
- `save_draft` — `{ session_id, patrol_id, scores: { criterion_id: value } }`
- `subscribe_session` — `{ session_id }` (receive broadcast updates)

**Server → Client:**
- `draft_saved` — Acknowledgement with timestamp
- `patrol_submitted` — Broadcast when any user submits a patrol
- `error` — Error with code and message

---

## Future Considerations

These are explicitly out of scope for MVP but were discussed:

- **Results dashboard** — Viewing aggregated scores after a session closes
- **Per-user patrol assignment permissions** — Restricting which patrols a user can score
- **Session scoping** — Sessions that only apply to certain users/subcamps
- **Criteria template management UI** — Currently DB-only
- **Event/session creation UI** — Currently DB-only
- **User management UI** — Currently DB-only
- **Offline-first PWA** — Service worker for full offline support beyond the current queue-and-replay approach
