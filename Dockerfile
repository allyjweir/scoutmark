# ── Stage 1: Build frontend ──────────────────────────────────────────
FROM node:22-alpine AS frontend

WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ── Stage 2: Build Go binaries ──────────────────────────────────────
FROM golang:1.24-alpine AS backend

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY gen/ gen/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/scoutmark ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/scoutmark-admin ./cmd/admin

# ── Stage 3: Production image ──────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binaries
COPY --from=backend /bin/scoutmark /bin/scoutmark
COPY --from=backend /bin/scoutmark-admin /bin/scoutmark-admin

# Copy built frontend
COPY --from=frontend /app/frontend/dist frontend/dist

# Copy migrations (applied on startup)
COPY migrations/ migrations/

EXPOSE 8080

CMD ["/bin/scoutmark"]
