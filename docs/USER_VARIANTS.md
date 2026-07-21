# User Variants

This document describes the three user variants currently used in Scoutmark and how each one behaves in the app.

## Summary

| Variant | How Identified | Main Access |
|---|---|---|
| Standard Scorer | `is_admin = false` | Scores regular sessions only |
| Admin | `is_admin = true` and not Camp Chief account | Full admin tools plus scoring views |
| Camp Chief | Account with `username = campchief` or `id = usr-campchief` | Round 2 leadership/finalisation flow only |

## 1. Standard Scorer

### Identification

A signed-in user where `is_admin` is false.

### What They Can Do

- Access dashboard and scoring flow.
- Open active/locked/closed regular scoring sessions through `/sessions/:sessionId`.
- Submit and review their own scoring for their assigned scope.

### What They Cannot Do

- Access admin routes:
  - `/admin/sessions/:sessionId`
  - `/admin/sessions/:sessionId/scorer/:userId`
- Access Camp Chief route:
  - `/campchief/sessions/:sessionId`
- Use admin quick-access controls on dashboard.

### Dashboard Behavior

- Only regular sessions are shown (round 2 sessions are hidden for non-admin users).
- Can see session status groupings (active, upcoming, locked, closed) for visible sessions.

## 2. Admin

### Identification

A signed-in user where `is_admin` is true, except the dedicated Camp Chief account.

### What They Can Do

- Access all standard scoring routes.
- Access admin progress/session management route:
  - `/admin/sessions/:sessionId`
- Access per-scorer admin route:
  - `/admin/sessions/:sessionId/scorer/:userId`
- Use admin quick-access panel on dashboard.

### What They Cannot Do

- They are not forced into Camp Chief-only round 2 UX.

### Dashboard Behavior

- Sees full session set (including round 2 sessions).
- Gets admin quick-access links for active/closed session progress views.

## 3. Camp Chief

### Identification

Special-cased account detected by either:

- `username === "campchief"`, or
- `id === "usr-campchief"`

This account may have `is_admin = true`, but it is intentionally treated as its own variant.

### What They Can Do

- Access dashboard and scoring routes.
- For round 2 sessions, navigation is routed to Camp Chief view:
  - `/campchief/sessions/:sessionId`
- Use round 2 finalisation workflow in `AdminSessionPage` with Camp Chief-specific guard behavior.

### What They Cannot Do

- Access normal admin routes:
  - `/admin/sessions/:sessionId`
  - `/admin/sessions/:sessionId/scorer/:userId`
- See admin quick-access controls on dashboard.
- Use Camp Chief route for non-round-2 sessions (guarded and redirected).

### Dashboard Behavior

- Keeps regular session behavior.
- For round 2 sessions, clicks open Camp Chief route instead of standard scoring route.

## Shared Gate for All Variants

All three variants must pass the same base authentication gate before role logic applies:

1. User must be authenticated.
2. If password change is required, user is redirected to `/change-password`.

Only after these checks does role/variant route logic run.
