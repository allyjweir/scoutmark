# Scoutmark

A mobile-first web application for scoring scout patrols across configurable criteria during meetings and camps.

## Architecture

- **Backend**: Go (standard library + samber/lo), protobuf API, MySQL, OpenTelemetry → Honeycomb
- **Frontend**: React + TypeScript, GitHub Primer UI, react-router, lodash
- **Real-time**: WebSocket-based draft auto-saving with offline resilience

## Concepts

| Concept | Description |
|---------|-------------|
| **Event** | Top-level grouping (e.g., "Summer Camp 2026", "Term 1 Meetings") |
| **Patrol** | A scout patrol that can be scored |
| **User** | Login identity representing a subcamp team, owns an ordered list of patrols |
| **Criteria Template** | A reusable set of scoring dimensions |
| **Criterion** | Individual scoring dimension (title, min, max) within a template |
| **Session** | A time-windowed scoring event linked to an event and criteria template |
| **Draft** | In-progress scores auto-saved via WebSocket |
| **Submission** | Finalised, locked scores (admin can unlock) |

## Prerequisites

- Go 1.22+
- Node.js 20+
- Postgres
- [Buf CLI](https://buf.build/docs/installation)
- Honeycomb API key (for tracing)

## Quick Start

```bash
# 1. Start dependencies
docker-compose up -d

# 2. Run migrations
go run ./cmd/migrate

# 3. Generate protobuf code
buf generate

# 4. Start the Go backend
HONEYCOMB_API_KEY=your-key go run ./cmd/server

# 5. Start the frontend dev server
cd frontend && npm ci && npm run dev
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | MySQL connection string | `root:scoutmark@tcp(localhost:3306)/scoutmark?parseTime=true` |
| `HONEYCOMB_API_KEY` | Honeycomb API key for traces | (required) |
| `HONEYCOMB_DATASET` | Honeycomb dataset name | `scoutmark` |
| `SERVER_ADDR` | Server listen address | `:8080` |
| `SESSION_SECRET` | Secret for signing session tokens | `scoutmark-dev-secret` |

## Project Structure

```
scoutmark/
├── cmd/
│   ├── server/          # Main server entrypoint
│   └── migrate/         # Database migration tool
├── internal/
│   ├── auth/            # Authentication middleware & session management
│   ├── database/        # MySQL connection & query helpers
│   ├── handlers/        # HTTP request handlers (protobuf API)
│   ├── models/          # Domain models
│   ├── tracing/         # OpenTelemetry setup (Honeycomb)
│   └── websocket/       # WebSocket hub & draft sync
├── proto/
│   └── scoutmark/v1/    # Protobuf service definitions
├── migrations/          # SQL migration files
├── frontend/
│   └── src/
│       ├── components/  # Reusable UI components
│       ├── pages/       # Route pages
│       ├── hooks/       # Custom React hooks
│       └── lib/         # API client, WebSocket client, utilities
├── docker-compose.yml
└── Makefile
```
