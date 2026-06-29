#!/usr/bin/env bash
#
# Minimal development seed — fast smoke-test data set.
# Uses the admin CLI — no raw SQL.
#
#   1 camp chief, 1 scorer, 1 admin
#   1 event + 1 template (2 criteria)
#   2 subcamps, 2 patrols each
#   1 session that opens NOW and stays open 12h
#
# Usage:
#   ./scripts/seed-min.sh
#
# Requires:
#   - PostgreSQL running (docker-compose up -d postgres)
#
set -euo pipefail

ADMIN="go run ./cmd/admin"
PASSWORD="password"

echo "═══════════════════════════════════════════════"
echo "  Scoutmark — Minimal dev seed"
echo "═══════════════════════════════════════════════"
echo

echo "Running migrations"
go run ./cmd/migrate
echo

# ─── Event ───────────────────────────────────────────────────────────

echo "Creating event..."
$ADMIN create-event \
  -id evt-min \
  -name "Dev Camp" \
  -description "Minimal seed event"
echo

# ─── Criteria Template ──────────────────────────────────────────────

echo "Creating criteria template..."
$ADMIN create-template \
  -id tpl-min \
  -name "Quick Inspection" \
  -description "Two-criteria inspection"

$ADMIN add-criterion -template tpl-min -id crt-tent \
  -title "Tent & Bedding" \
  -description "Tents properly pitched, bedding aired and rolled" \
  -min 0 -max 10 -order 1

$ADMIN add-criterion -template tpl-min -id crt-kitchen \
  -title "Kitchen Area" \
  -description "Cooking area clean, fire properly maintained" \
  -min 0 -max 10 -order 2
echo

# ─── Users ───────────────────────────────────────────────────────────

echo "Creating users..."
$ADMIN create-user -id usr-chief   -username chief   -password "$PASSWORD" -display-name "Camp Chief" -role camp_chief
$ADMIN create-user -id usr-scorer  -username scorer  -password "$PASSWORD" -display-name "Scorer One" -role scorer
$ADMIN create-user -id usr-scorer2 -username scorer2 -password "$PASSWORD" -display-name "Scorer Two" -role scorer
$ADMIN create-user -id usr-admin   -username admin   -password "$PASSWORD" -display-name "Admin"      -role admin
echo

# ─── Subcamps ────────────────────────────────────────────────────────

echo "Creating subcamps..."
$ADMIN create-subcamp -id sub-a -event evt-min -name "Subcamp A"
$ADMIN create-subcamp -id sub-b -event evt-min -name "Subcamp B"
echo

# ─── Patrols ─────────────────────────────────────────────────────────

echo "Creating patrols..."
$ADMIN create-patrol -id pat-1 -subcamp sub-a -name "Patrol 1"
$ADMIN create-patrol -id pat-2 -subcamp sub-a -name "Patrol 2"
$ADMIN create-patrol -id pat-3 -subcamp sub-b -name "Patrol 3"
$ADMIN create-patrol -id pat-4 -subcamp sub-b -name "Patrol 4"
echo

# ─── User-Subcamp Assignments ───────────────────────────────────────

echo "Assigning subcamps..."
$ADMIN assign-subcamp -user usr-scorer  -subcamp sub-a -order 1
$ADMIN assign-subcamp -user usr-scorer2 -subcamp sub-b -order 1
$ADMIN assign-subcamp -user usr-chief   -subcamp sub-a -order 1
$ADMIN assign-subcamp -user usr-chief   -subcamp sub-b -order 2
echo

# ─── Session (open now, 12h) ────────────────────────────────────────

echo "Creating session..."
$ADMIN create-session \
  -id ses-now -event evt-min -template tpl-min \
  -name "Open Session" -start now -duration 12h \
  -award-best-patrol
echo

echo "═══════════════════════════════════════════════"
echo "  ✓ Minimal database seeded!"
echo ""
  echo "  Users:    chief (camp_chief), scorer, scorer2, admin"
echo "  Password: $PASSWORD"
echo "  Session:  ses-now — open for 12h"
echo "═══════════════════════════════════════════════"
