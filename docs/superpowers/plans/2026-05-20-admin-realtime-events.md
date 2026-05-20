# Admin Realtime Events Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an authenticated admin WebSocket event stream for realtime logs, credentials, and import jobs, and remove import-job polling.

**Architecture:** Add a backend event hub owned by `AdminHandler`, publish best-effort `*.updated` events from log, credential, quota, and import-job write paths, and expose them through `/admin/api/events`. Add a shared frontend event composable that connects after admin login, dispatches events, reconnects every 3 seconds, and lets logs/credentials pages apply realtime updates only when their page toggle is enabled.

**Tech Stack:** Go, Gin, `nhooyr.io/websocket`, Nuxt 4, Vue composables, Vuetify.

---

### Task 1: Backend Event Hub And WebSocket Route

**Files:**
- Create: `internal/handler/admin_events.go`
- Modify: `internal/handler/admin.go`
- Modify: `internal/router/router.go`
- Test: `internal/handler/admin_events_test.go`

- [ ] **Step 1: Write failing event hub tests**

Create `internal/handler/admin_events_test.go` with tests that verify a subscriber receives a published event and that a closed subscriber is removed without blocking a second publish.

Run: `go test ./internal/handler -run 'TestAdminEventHub' -count=1`

Expected: FAIL because `newAdminEventHub` and related types do not exist.

- [ ] **Step 2: Implement event hub and WebSocket handler**

Create `internal/handler/admin_events.go` with:

- `adminEvent{Type string, Data any}`
- `adminEventHub` with subscribe, unsubscribe, and publish methods
- bounded per-client channels
- best-effort publish that drops a subscriber if its channel is full
- `AdminHandler.Events(c *gin.Context)` that accepts WebSocket requests and writes JSON events

Use `nhooyr.io/websocket` and `wsjson.Write`.

- [ ] **Step 3: Wire the route and handler state**

Modify `internal/handler/admin.go` so `NewAdminHandler` initializes `events: newAdminEventHub()`.

Modify `internal/router/router.go` to add:

```go
apiGroup.GET("/events", deps.Admin.Events)
```

- [ ] **Step 4: Verify event hub tests**

Run: `go test ./internal/handler -run 'TestAdminEventHub' -count=1`

Expected: PASS.

### Task 2: Publish Import Job Events And Remove Job Polling API

**Files:**
- Modify: `internal/handler/admin_import_jobs.go`
- Modify: `internal/router/router.go`
- Modify: `web/composables/useImportJobs.ts`
- Modify: `web/components/ui/ImportJobDock.vue`
- Modify: `web/composables/useAdminApi.ts`
- Test: `internal/handler/admin_import_jobs_test.go`

- [ ] **Step 1: Write failing import job event test**

Add a test that starts an import job with two inputs and asserts `import_job.updated` snapshots are published for creation, progress, and completion.

Run: `go test ./internal/handler -run 'TestImportJobPublishesEvents' -count=1`

Expected: FAIL because import jobs do not publish events.

- [ ] **Step 2: Publish job snapshots**

Change `importJobManager.Start` to accept an update hook and call it:

- immediately after job creation
- after each processed token
- after finish

Change admin create endpoints to pass a hook that publishes:

```go
adminEvent{Type: "import_job.updated", Data: snapshot}
```

- [ ] **Step 3: Remove polling and acknowledgement REST endpoints**

Remove `ListJobs`, `AcknowledgeJob`, and related route registrations for:

- `GET /admin/api/jobs`
- `DELETE /admin/api/jobs/:id`

Remove `listJobs` and `acknowledgeJob` from `web/composables/useAdminApi.ts`.

- [ ] **Step 4: Convert frontend import job state to event-driven**

Update `web/composables/useImportJobs.ts`:

- remove polling timer code
- keep `add`, `merge`, `dismiss`, and `visibleJobs`
- make `dismiss` local-only with no token parameter

Update `ImportJobDock.vue` to remove startup polling and call `dismiss(job)` locally.

- [ ] **Step 5: Verify**

Run: `go test ./internal/handler -run 'TestImportJobPublishesEvents|TestAdminEventHub' -count=1`

Expected: PASS.

### Task 3: Publish Log And Credential Events

**Files:**
- Modify: `internal/handler/admin.go`
- Modify: `internal/handler/admin_codex.go`
- Modify: `internal/handler/admin_gemini.go`
- Modify: `internal/handler/admin_antigravity.go`
- Modify: `core/codex/scheduler.go`
- Modify: `core/gemini/scheduler.go`
- Modify: `core/antigravity/scheduler.go`
- Modify as needed: scheduler construction in `internal/app/app.go`
- Test: focused Go tests where existing constructors permit

- [ ] **Step 1: Add publisher interfaces**

Add a small interface in backend code:

```go
type AdminEventPublisher interface {
  PublishAdminEvent(eventType string, data any)
}
```

Expose `PublishAdminEvent` on `AdminHandler`.

- [ ] **Step 2: Publish credential events from admin handlers**

After manual status updates, deletes, OAuth/import creates, and quota sync scheduling, publish `credential.updated` with `{handler, ids, action}`.

- [ ] **Step 3: Publish log events from scheduler insert paths**

In each scheduler `insertLog` method, publish `log.updated` after `InsertLog` succeeds. The payload is a row-shaped object derived from `InsertLogParams` plus `created_at: time.Now()`.

- [ ] **Step 4: Publish credential events from scheduler status and quota writes**

Publish `credential.updated` after throttling, disabling, and quota persistence succeeds.

- [ ] **Step 5: Verify Go tests**

Run: `go test ./internal/handler ./core/codex ./core/gemini ./core/antigravity -count=1`

Expected: PASS.

### Task 4: Frontend Event Client

**Files:**
- Create: `web/composables/useAdminEvents.ts`
- Modify: `web/types/admin.ts`
- Modify: `web/app.vue`

- [ ] **Step 1: Add event types**

Extend `web/types/admin.ts` with `AdminEvent`, `AdminEventType`, and `CredentialUpdatedEventData` matching the spec.

- [ ] **Step 2: Implement `useAdminEvents`**

Create a shared composable that:

- stores connection state
- connects to `/admin/api/events?token=<token>` after auth readiness
- reconnects every 3 seconds while authenticated
- exposes `onEvent(type, handler)` for page handlers
- ignores malformed JSON

- [ ] **Step 3: Start and stop from app shell**

In `web/app.vue`, initialize the event client after boot/login readiness and close it during logout/auth invalidation.

- [ ] **Step 4: Typecheck**

Run: `npm run typecheck` in `web`.

Expected: PASS.

### Task 5: Realtime Logs Page

**Files:**
- Modify: `web/pages/logs.vue`

- [ ] **Step 1: Add UI state**

Add a realtime refresh toggle defaulting to on, a new-log counter, and connection state display.

- [ ] **Step 2: Handle `log.updated`**

When realtime is enabled, page is 1, and the log matches current filters, insert it at the top and trim to `pageSize`. Update totals and status-code summary counts.

When realtime is disabled or page is not 1, increment a pending count and leave the current list stable.

- [ ] **Step 3: Verify typecheck**

Run: `npm run typecheck` in `web`.

Expected: PASS.

### Task 6: Realtime Credentials Page

**Files:**
- Modify: `web/pages/credentials.vue`

- [ ] **Step 1: Add UI state**

Add a realtime refresh toggle defaulting to on, a pending-update indicator, and connection state display.

- [ ] **Step 2: Handle `credential.updated` and `import_job.updated`**

For matching active handler credential events, debounce and run:

```ts
await Promise.all([
  admin.loadOverview(admin.token.value, true),
  loadCredentials(page.value, pageSize.value),
])
```

When realtime is off, set pending state without refetching.

For `import_job.updated`, update the import job store. When a matching job completes and realtime is on, trigger the same credential refresh.

- [ ] **Step 3: Remove old import polling call**

Remove `importJobs.ensurePolling(admin.token.value)` after creating import jobs.

- [ ] **Step 4: Verify typecheck**

Run: `npm run typecheck` in `web`.

Expected: PASS.

### Task 7: Full Verification

**Files:**
- `go.mod`
- `go.sum`
- changed Go and frontend files

- [ ] **Step 1: Format**

Run:

```powershell
gofmt -w internal\handler\admin_events.go internal\handler\admin_events_test.go internal\handler\admin.go internal\handler\admin_import_jobs.go internal\handler\admin_codex.go internal\handler\admin_gemini.go internal\handler\admin_antigravity.go internal\router\router.go core\codex\scheduler.go core\gemini\scheduler.go core\antigravity\scheduler.go
```

- [ ] **Step 2: Backend tests**

Run:

```powershell
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Frontend typecheck and build**

Run from `web`:

```powershell
npm run typecheck
npm run build
```

Expected: PASS.

- [ ] **Step 4: Review diff**

Run:

```powershell
git diff --stat
git diff --check
```

Expected: no whitespace errors and diff scoped to realtime events.
