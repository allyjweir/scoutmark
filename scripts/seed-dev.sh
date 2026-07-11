#!/usr/bin/env bash
#
# Seed a development database with Blair Atholl 2026 data.
# Uses the admin CLI — no raw SQL.
#
# Usage:
#   ./scripts/seed-dev.sh
#   make seed
#
# Requires:
#   - PostgreSQL running (docker-compose up -d postgres)
#   - Migrations applied (go run ./cmd/server starts them)
#
set -euo pipefail

ADMIN="go run ./cmd/admin"
PASSWORD="blairatholl"

echo "═══════════════════════════════════════════════"
echo "  Scoutmark — Seeding development database"
echo "═══════════════════════════════════════════════"
echo

echo "Running migrations"
go run ./cmd/migrate

# ─── Event ───────────────────────────────────────────────────────────

echo "Creating event..."
$ADMIN create-event \
  -id evt-ba-2026 \
  -name "Blair Atholl 2026" \
  -description "Blair Atholl Jamborette 2026"
echo

# ─── Criteria Template ──────────────────────────────────────────────

echo "Creating criteria template..."
$ADMIN create-template \
  -id tpl-camp \
  -name "Camp Inspection" \
  -description "Daily camp inspection criteria"
echo

echo "Adding criteria..."
$ADMIN add-criterion -template tpl-camp -id crt-tent \
  -title "Tent & Bedding" \
  -description "Tents properly pitched, bedding aired and rolled" \
  -min 0 -max 10 -order 1

$ADMIN add-criterion -template tpl-camp -id crt-kit \
  -title "Kit Layout" \
  -description "Personal kit neatly stored and accessible" \
  -min 0 -max 10 -order 2

$ADMIN add-criterion -template tpl-camp -id crt-hygiene \
  -title "Hygiene Area" \
  -description "Wash stands clean, toiletries organised" \
  -min 0 -max 10 -order 3

$ADMIN add-criterion -template tpl-camp -id crt-kitchen \
  -title "Kitchen Area" \
  -description "Cooking area clean, fire properly maintained" \
  -min 0 -max 10 -order 4

$ADMIN add-criterion -template tpl-camp -id crt-gadgets \
  -title "Camp Gadgets" \
  -description "Quality and creativity of pioneering projects" \
  -min 0 -max 10 -order 5

$ADMIN add-criterion -template tpl-camp -id crt-spirit \
  -title "Camp Spirit" \
  -description "Enthusiasm, teamwork and patrol morale" \
  -min 0 -max 10 -order 6
echo

# ─── Users ───────────────────────────────────────────────────────────

echo "Creating users..."
$ADMIN create-user -id usr-campchief -username campchief -password "$PASSWORD" -display-name "Camp Chief" -admin
$ADMIN create-user -id usr-morrison  -username morrison  -password "$PASSWORD" -display-name "Morrison"
$ADMIN create-user -id usr-macdonald -username macdonald -password "$PASSWORD" -display-name "MacDonald"
$ADMIN create-user -id usr-maclean   -username maclean   -password "$PASSWORD" -display-name "MacLean"
$ADMIN create-user -id usr-murray    -username murray    -password "$PASSWORD" -display-name "Murray"
$ADMIN create-user -id usr-robertson -username robertson -password "$PASSWORD" -display-name "Robertson"
$ADMIN create-user -id usr-stewart   -username stewart   -password "$PASSWORD" -display-name "Stewart"
$ADMIN create-user -id usr-stacey    -username stacey    -password "$PASSWORD" -display-name "Stacey"
$ADMIN create-user -id usr-ally      -username ally      -password "$PASSWORD" -display-name "Ally"
$ADMIN create-user -id usr-iona      -username iona      -password "$PASSWORD" -display-name "Iona"
echo

# ─── Subcamps ───────────────────────────────────────────────────────

echo "Creating subcamps..."
$ADMIN create-subcamp -id sub-morrison -name "Morrison"
$ADMIN create-subcamp -id sub-macdonald -name "MacDonald"
$ADMIN create-subcamp -id sub-maclean -name "MacLean"
$ADMIN create-subcamp -id sub-murray -name "Murray"
$ADMIN create-subcamp -id sub-robertson -name "Robertson"
$ADMIN create-subcamp -id sub-stewart -name "Stewart"
echo

echo "Assigning users to subcamps..."
$ADMIN assign-user-subcamp -user usr-morrison -subcamp sub-morrison
$ADMIN assign-user-subcamp -user usr-stacey -subcamp sub-morrison
$ADMIN assign-user-subcamp -user usr-ally -subcamp sub-morrison
$ADMIN assign-user-subcamp -user usr-iona -subcamp sub-morrison
$ADMIN assign-user-subcamp -user usr-macdonald -subcamp sub-macdonald
$ADMIN assign-user-subcamp -user usr-maclean -subcamp sub-maclean
$ADMIN assign-user-subcamp -user usr-murray -subcamp sub-murray
$ADMIN assign-user-subcamp -user usr-robertson -subcamp sub-robertson
$ADMIN assign-user-subcamp -user usr-stewart -subcamp sub-stewart
echo

# ─── Patrols ─────────────────────────────────────────────────────────

echo "Creating patrols..."

# Morrison's patrols
for p in "pat-mor-1:Site 1:1" "pat-mor-2:Site 2:2" "pat-mor-3:Site 3:3" "pat-mor-4:Site 4:4" "pat-mor-5:Site 5:5"; do
  IFS=: read -r id name order <<< "$p"
  $ADMIN create-patrol -id "$id" -name "$name" -subcamp sub-morrison -order "$order"
done

# MacDonald's patrols
for p in "pat-mac-1:Site 1:1" "pat-mac-2:Site 2:2" "pat-mac-3:Site 3:3" "pat-mac-4:Site 4:4" "pat-mac-5:Site 5:5"; do
  IFS=: read -r id name order <<< "$p"
  $ADMIN create-patrol -id "$id" -name "$name" -subcamp sub-macdonald -order "$order"
done

# MacLean's patrols
for p in "pat-mcl-1:Site 1:1" "pat-mcl-2:Site 2:2" "pat-mcl-3:Site 3:3" "pat-mcl-4:Site 4:4" "pat-mcl-5:Site 5:5"; do
  IFS=: read -r id name order <<< "$p"
  $ADMIN create-patrol -id "$id" -name "$name" -subcamp sub-maclean -order "$order"
done

# Murray's patrols
for p in "pat-mur-1:Site 1:1" "pat-mur-2:Site 2:2" "pat-mur-3:Site 3:3" "pat-mur-4:Site 4:4" "pat-mur-5:Site 5:5"; do
  IFS=: read -r id name order <<< "$p"
  $ADMIN create-patrol -id "$id" -name "$name" -subcamp sub-murray -order "$order"
done

# Robertson's patrols
for p in "pat-rob-1:Site 1:1" "pat-rob-2:Site 2:2" "pat-rob-3:Site 3:3" "pat-rob-4:Site 4:4" "pat-rob-5:Site 5:5"; do
  IFS=: read -r id name order <<< "$p"
  $ADMIN create-patrol -id "$id" -name "$name" -subcamp sub-robertson -order "$order"
done

# Stewart's patrols
for p in "pat-stw-1:Site 1:1" "pat-stw-2:Site 2:2" "pat-stw-3:Site 3:3" "pat-stw-4:Site 4:4" "pat-stw-5:Site 5:5"; do
  IFS=: read -r id name order <<< "$p"
  $ADMIN create-patrol -id "$id" -name "$name" -subcamp sub-stewart -order "$order"
done
echo

# ─── Sessions ────────────────────────────────────────────────────────

echo "Creating sessions..."

$ADMIN create-session \
  -id ses-sun -event evt-ba-2026 -template tpl-camp \
  -name "Sunday" -start now -duration 24h \
  -subcamps "sub-morrison,sub-macdonald,sub-maclean,sub-murray,sub-robertson,sub-stewart" \
  -award-best-patrol

$ADMIN create-session \
  -id ses-mon -event evt-ba-2026 -template tpl-camp \
  -name "Monday" -start "2026-02-09T06:00:00Z" -duration 4h \
  -subcamps "sub-morrison,sub-macdonald,sub-maclean,sub-murray,sub-robertson,sub-stewart" \
  -award-best-patrol -award-most-improved -previous-session ses-sun

$ADMIN create-session \
  -id ses-tue -event evt-ba-2026 -template tpl-camp \
  -name "Tuesday" -start "2026-02-10T06:00:00Z" -duration 4h \
  -subcamps "sub-morrison,sub-macdonald,sub-maclean,sub-murray,sub-robertson,sub-stewart" \
  -award-best-patrol -award-most-improved -previous-session ses-mon

$ADMIN create-session \
  -id ses-wed -event evt-ba-2026 -template tpl-camp \
  -name "Wednesday" -start "2026-02-11T06:00:00Z" -duration 4h \
  -subcamps "sub-morrison,sub-macdonald,sub-maclean,sub-murray,sub-robertson,sub-stewart" \
  -award-best-patrol -award-most-improved -previous-session ses-tue

$ADMIN create-session \
  -id ses-thu -event evt-ba-2026 -template tpl-camp \
  -name "Thursday" -start "2026-02-12T06:00:00Z" -duration 4h \
  -subcamps "sub-morrison,sub-macdonald,sub-maclean,sub-murray,sub-robertson,sub-stewart" \
  -award-best-patrol -award-most-improved -previous-session ses-wed

echo

# ─── Seed Scores for Thursday (closed session) ──────────────────────

echo "Seeding scores for Thursday session (for report card testing)..."

$ADMIN seed-scores -session ses-thu -user usr-morrison
$ADMIN seed-scores -session ses-thu -user usr-macdonald
$ADMIN seed-scores -session ses-thu -user usr-maclean
$ADMIN seed-scores -session ses-thu -user usr-murray
$ADMIN seed-scores -session ses-thu -user usr-robertson
$ADMIN seed-scores -session ses-thu -user usr-stewart

echo

echo "═══════════════════════════════════════════════"
echo "  ✓ Development database seeded successfully!"
echo ""
echo "  Users:    campchief (admin), morrison, macdonald,"
echo "            maclean, murray, robertson, stewart,"
echo "            stacey, ally, iona"
echo "  Password: $PASSWORD"
echo "═══════════════════════════════════════════════"
