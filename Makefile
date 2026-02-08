.PHONY: dev server frontend proto migrate seed clean

# Start everything for development
dev: proto
	@echo "Starting PostgreSQL..."
	docker-compose up -d postgres
	@echo "Waiting for PostgreSQL..."
	@sleep 3
	@make server &
	@make frontend

# Run the Go backend
server:
	go run ./cmd/server

# Run the frontend dev server
frontend:
	cd frontend && npm run dev

# Generate protobuf code
proto:
	buf generate

# Run database migrations
migrate:
	go run ./cmd/server  # Migrations run on startup

# Seed development data
seed:
	PGPASSWORD=scoutmark psql -U scoutmark -d scoutmark -h localhost < migrations/002_seed_dev.sql

# Create a new user interactively
create-user:
	go run ./cmd/admin create-user

# Change a user's password interactively
change-password:
	go run ./cmd/admin change-password

# List all users
list-users:
	go run ./cmd/admin list-users

# List all sessions with status
list-sessions:
	go run ./cmd/admin list-sessions

# Clean generated files
clean:
	rm -rf gen/
	rm -rf frontend/src/proto/
	rm -rf frontend/dist/
	rm -rf frontend/node_modules/

# Build for production
build: proto
	cd frontend && npm ci && npm run build
	go build -o bin/scoutmark ./cmd/server
	go build -o bin/scoutmark-admin ./cmd/admin

# Install frontend dependencies
install:
	cd frontend && npm install
	@which buf > /dev/null 2>&1 || brew install bufbuild/buf/buf
