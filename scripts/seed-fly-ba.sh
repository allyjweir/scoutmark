#!/usr/bin/env bash
#
# Seed the Blair Atholl demo data set in the scoutmark-ba Fly environment.
#
# Usage:
#   ./scripts/seed-fly-ba.sh
#
# Requires:
#   - flyctl authenticated to the Fly account that owns scoutmark-ba
#   - the scoutmark-ba app deployed with a build containing seed-ba-demo
#   - DATABASE_URL set on the Fly app
#
set -euo pipefail

APP="scoutmark-ba"
CONFIG="fly-ba.toml"
URL="https://${APP}.fly.dev"

echo "═══════════════════════════════════════════════"
echo "  Scoutmark — Seeding Blair Atholl demo on Fly"
echo "═══════════════════════════════════════════════"
echo

if [[ ! -f "$CONFIG" ]]; then
  echo "error: $CONFIG not found; run this script from the repository root" >&2
  exit 1
fi

if ! command -v flyctl >/dev/null 2>&1; then
  echo "error: flyctl is required" >&2
  exit 1
fi

configured_app=$(awk -F\' '/^app = / { print $2 }' "$CONFIG")
if [[ "$configured_app" != "$APP" ]]; then
  echo "error: $CONFIG points at '$configured_app', expected '$APP'" >&2
  exit 1
fi

echo "Target: $APP"
echo "Waking app and letting startup migrations run..."
curl -fsS --retry 12 --retry-delay 2 "$URL/healthz" >/dev/null
echo

echo "Running remote seed command..."
flyctl ssh console -a "$APP" --config "$CONFIG" -C "/bin/scoutmark-admin seed-ba-demo"
echo

echo "Verifying sessions..."
flyctl ssh console -a "$APP" --config "$CONFIG" -C "/bin/scoutmark-admin list-sessions"

echo
echo "═══════════════════════════════════════════════"
echo "  ✓ Blair Atholl demo seeded"
echo ""
echo "  URL:      $URL"
echo "  Password: password"
echo "═══════════════════════════════════════════════"