# Credential `throttled` Status Design

## Goal

Introduce a first-class `throttled` credential status for both Codex and Gemini so temporary scheduler backoff is visible in the admin UI, remains distinct from permanent `disabled`, and automatically returns to `enabled` when the throttle window expires.

## Problem

The current implementation only has `enabled` and `disabled`.

- Temporary request backoff is stored in quota tables via `throttled_until`.
- Scheduler cache removes throttled credentials from the available pool.
- Admin users cannot distinguish a temporarily throttled credential from a healthy one unless they infer it from quota timestamps.
- Reusing `disabled` for throttling would conflate operator intent, auth failure, and temporary scheduler state.

This makes it hard to identify which credentials are waiting for retry and which ones need manual intervention.

## Scope

This change applies to both credential systems:

- Codex credentials
- Gemini credentials

This change includes:

- domain status enum updates
- scheduler state transitions for temporary throttling
- automatic recovery after throttle expiry
- admin API filtering and batch status updates
- admin UI status display and filtering

This change does not include:

- new database columns
- manual setting of `throttled` from the admin API
- broad refactors of quota scoring or retry strategy

## Status Model

Credential status becomes a three-state model:

- `enabled`: eligible for selection, subject to existing quota and expiry checks
- `throttled`: temporarily unavailable due to scheduler backoff; expected to recover automatically
- `disabled`: permanently unavailable until manual re-enable or re-import

### Status Ownership

- Operators may set a credential to `enabled` or `disabled`
- The system may set a credential to `throttled`
- The system may restore a `throttled` credential to `enabled`
- The system may set a credential to `disabled` for terminal auth/account failures

The admin batch status update API should continue to accept only `enabled` and `disabled`. `throttled` is a system-managed transient state, not an operator command.

## Data Model

No schema migration is required because `status` is stored as free text and `reason` already exists for both Codex and Gemini tables.

The shared account status constants will be expanded from:

- `enabled`
- `disabled`

to:

- `enabled`
- `disabled`
- `throttled`

Parsing and validation helpers must accept all three values where system reads database state, but operator-facing mutation validation remains limited to `enabled` and `disabled`.

## Scheduler Behavior

### Temporary Throttle Transition

When a scheduler decides a request failure should trigger backoff:

1. persist `throttled_until` in the relevant quota table
2. update the credential row status to `throttled`
3. set `reason` to a short machine-readable message that explains why the throttle happened
4. remove the credential, or the specific tier, from the in-memory available pool

Recommended reason format:

- Codex: `temporary throttle: <reason>`
- Gemini: `temporary throttle: <reason>`

Examples:

- `temporary throttle: retry-after`
- `temporary throttle: consecutive failures`

For tier-specific throttle:

- credential status still becomes `throttled`
- tier-specific throttle timestamps continue to control actual scheduling availability
- UI can show the credential as throttled without needing a second state model

This intentionally prefers operator visibility over exact per-tier presentation.

### Automatic Recovery

When throttle windows expire, the system must restore `throttled` credentials back to `enabled`.

Recovery happens in the scheduler refresh path:

1. before rebuilding the available cache, update any credentials whose status is `throttled` and whose persisted throttle window is no longer in the future
2. clear expired transient in-memory throttle entries
3. rebuild available credentials from database state

The restore query must only target credentials that are both:

- currently `throttled`
- no longer throttled according to quota timestamps

This avoids reviving manually `disabled` credentials.

For Codex, recovery should consider both default and spark throttle windows.

For Gemini, recovery should consider all tier throttle windows.

A credential is restorable only when all relevant throttle windows are expired or unset.

On successful recovery:

- status becomes `enabled`
- reason is cleared

This recovery step must run from persisted database state during normal refreshes, including startup refresh, not only when an in-memory throttle marker has expired. Otherwise a process restart could leave rows stuck in `throttled`.

## Terminal Failure Behavior

Existing permanent-disable flows remain unchanged in intent:

- direct terminal rejection statuses
- invalid or missing refresh token
- post-refresh verification that still fails with auth rejection

These flows continue to set `disabled`, never `throttled`.

If a credential is already `throttled` and later hits a terminal failure, `disabled` takes precedence.

## Availability Queries

`ListAvailableCodex` and `ListAvailableGemini` continue to select only `status = 'enabled'`.

This is important because:

- `throttled` credentials should never be accidentally selected just because an in-memory throttle entry is missing
- scheduler availability remains consistent across process restarts

The quota-table `throttled_until` values still participate in score suppression exactly as they do today, but `status = 'throttled'` becomes the primary persisted visibility signal.

## Admin API

### Filtering

Credential list filters should accept:

- `enabled`
- `disabled`
- `throttled`

This applies to both Codex and Gemini admin list endpoints.

### Batch Status Updates

Batch update request validation remains:

- `enabled`
- `disabled`

Behavior:

- setting `enabled` clears any previous manual disable state and clears `reason`
- setting `disabled` sets manual disable state and may clear throttle timestamps only if existing code already does so; otherwise throttle timestamps may remain as historical data

The API must reject manual writes of `throttled` so operators do not create ambiguous recovery conditions.

If an operator manually sets a credential to `enabled` before its throttle timestamps have expired, availability queries may still keep it out of rotation until those timestamps expire. This is acceptable and should be documented in the UI if needed, but it should not silently rewrite persisted throttle timestamps.

## Admin UI

The UI should treat `throttled` as a first-class visible status:

- display the `throttled` label in status columns and detail views
- allow filtering by `throttled`
- keep existing disabled/enabled affordances unchanged

If the UI already shows `reason` or `throttled_until`, those fields remain useful and do not need redesign for this change.

## Implementation Plan

The implementation will likely touch these areas:

- `utils/constants.go`
- Codex scheduler and manager flows
- Gemini scheduler flows
- SQL query filters for paged listing and available selection
- store conversion layers if any enum parsing assumptions exist
- admin handlers for filter parsing and validation
- frontend status filter options and status rendering

## Failure Modes And Safeguards

### Risk: throttled credentials never recover

Cause:

- only in-memory throttle entries expire
- DB `status` remains `throttled`

Mitigation:

- recovery must persist status restoration before or during refresh of available credentials

### Risk: disabled credentials recover incorrectly

Cause:

- recovery query only looks at throttle timestamps

Mitigation:

- recovery query must include `status = 'throttled'`

### Risk: partial-tier throttle misrepresented as full credential throttle

Cause:

- a single credential status cannot encode per-tier detail

Mitigation:

- accept this tradeoff for operational visibility
- continue to rely on per-tier `throttled_until_*` fields for actual scheduling

### Risk: status mismatch after restart

Cause:

- scheduler restarts before auto-recovery runs

Mitigation:

- recovery logic runs from persisted DB state during refresh, not only from in-memory state

## Testing Strategy

Add regression tests before production changes for:

1. status parsing accepts `throttled`
2. Codex temporary throttle marks credential `throttled`
3. Gemini temporary throttle marks credential `throttled`
4. expired throttled Codex credential auto-recovers to `enabled`
5. expired throttled Gemini credential auto-recovers to `enabled`
6. manually `disabled` credentials do not auto-recover even if quota timestamps are expired
7. admin filter accepts `throttled`
8. batch update validation still rejects manual `throttled`

Where direct unit coverage is difficult because current packages lack tests, start with focused tests around the smallest state transition helpers added for this feature.

## Recommendation

Implement `throttled` as a persisted system-owned status for both Codex and Gemini, with automatic restoration to `enabled` after quota throttle expiry.

This gives the admin surface the visibility it currently lacks without conflating temporary backoff with permanent disablement, and it avoids the larger complexity of introducing a separate system-state layer.
