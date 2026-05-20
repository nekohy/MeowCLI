# Admin Realtime Events Design

Date: 2026-05-20

## Goal

Replace import-job polling and add realtime admin updates for logs, credentials, and import jobs through one authenticated WebSocket event stream.

## Scope

- Add an admin WebSocket endpoint at `/admin/api/events`.
- Publish three event types:
  - `log.updated`
  - `credential.updated`
  - `import_job.updated`
- Remove the old frontend import-job polling path.
- Remove the old import-job polling and acknowledgement APIs.
- Add per-page realtime refresh toggles for logs and credentials.

## Non-Goals

- Do not add polling fallback.
- Do not use exponential reconnect backoff.
- Do not push full credential list snapshots over WebSocket.
- Do not replace the existing REST list APIs for logs or credentials.

## Event Transport

The admin frontend opens one WebSocket connection after admin authentication is ready. The connection uses the existing admin token and is served by `/admin/api/events`.

The client reconnects every 3 seconds while authenticated if the socket closes unexpectedly. If the socket is disconnected, realtime UI state shows that events are not connected. No polling fallback runs while disconnected.

The backend owns a small event hub:

- Tracks active admin WebSocket subscribers.
- Broadcasts events without blocking request or scheduler code.
- Uses bounded per-client buffers.
- Drops or closes slow clients instead of allowing event publication to block backend work.
- Cleans up subscribers when the request context ends or the socket fails.

## Event Shape

All WebSocket messages are JSON objects:

```json
{
  "type": "credential.updated",
  "data": {}
}
```

TypeScript shape:

```ts
type AdminEvent =
  | { type: 'log.updated'; data: { item: LogItem } }
  | {
      type: 'credential.updated'
      data: {
        handler: string
        ids: string[]
        action: 'created' | 'updated' | 'deleted' | 'quota' | 'status'
      }
    }
  | { type: 'import_job.updated'; data: ImportJobSnapshot }
```

The event names intentionally use `updated` consistently. More specific meaning belongs in the payload.

## Backend Publication Points

### Logs

Whenever a log is successfully persisted, publish `log.updated`.

The event should contain enough fields to render a log row without an immediate REST refetch. If the current log store insert path does not return the inserted row, publish a row-shaped object from the original insert parameters plus the current timestamp.

### Credentials

Publish `credential.updated` after credential-affecting writes:

- Import creates a credential: `action: 'created'`
- Manual enable or disable: `action: 'status'`
- Manual delete: `action: 'deleted'`
- Scheduler throttles or disables credentials: `action: 'status'`
- Quota sync writes quota data: `action: 'quota'`

The event payload includes handler and credential IDs. The frontend will refetch the active credential page instead of trying to patch complex computed quota/status cards locally.

### Import Jobs

Publish `import_job.updated` when an import job is created, progresses, and finishes.

The old frontend polling composable should be replaced with WebSocket-driven state updates. Completed job dismissal is local frontend state; the UI no longer fetches or acknowledges jobs through REST endpoints.

## Frontend Behavior

### Shared Event Client

Add a shared admin event composable that:

- Connects after `admin.authReady` becomes true.
- Disconnects on logout or auth invalidation.
- Reconnects every 3 seconds while authenticated.
- Exposes connection state.
- Dispatches incoming events to page-level handlers.

### Logs Page

Add a realtime refresh toggle, default on.

When enabled:

- If the user is on page 1 and the incoming `log.updated` item matches current filters, insert it at the top.
- Keep at most `pageSize` rows in the visible list.
- Update total and summary counts for the visible page state.

When disabled:

- Do not mutate the current list.
- The page may show a new-log count while realtime events arrive.

If the user is not on page 1, do not auto-insert into the current page. The page may show a new-log count.

### Credentials Page

Add a realtime refresh toggle, default on.

When enabled:

- If a `credential.updated` event matches the active handler, debounce and refetch the current credential page.
- Refresh overview in the same batch.

When disabled:

- Do not refetch credentials automatically.
- The page may show a pending-update indicator.

### Import Jobs

Import job dock state is updated only from `import_job.updated` events and local submit responses.

The frontend no longer starts or maintains a 5-second polling timer for import jobs.

## API Cleanup

Remove frontend usage of:

- `GET /admin/api/jobs`

Remove backend route and handler for `GET /admin/api/jobs`.

Remove backend route and handler for `DELETE /admin/api/jobs/:id`. Job dismissal is local frontend state after this change.

## Error Handling

- WebSocket authentication failures close the socket and rely on existing auth invalid handling for REST failures.
- Malformed event messages are ignored by the frontend and do not break the socket.
- Backend event publication is best effort. A failed event send must not fail the underlying credential, log, or import operation.

## Testing

Backend tests:

- Event hub broadcasts to subscribed clients.
- Slow or closed subscribers do not block publication.
- Import jobs publish progress and completion snapshots.
- Credential status/quota write paths publish `credential.updated`.

Frontend tests or focused verification:

- `useAdminEvents` connects, dispatches valid events, ignores invalid messages, and reconnects every 3 seconds.
- Logs page inserts matching new logs at the top only when realtime refresh is enabled and page is 1.
- Credentials page refetches on matching `credential.updated` only when realtime refresh is enabled.
- Import job dock updates from WebSocket events without polling.

## Rollout Notes

Existing REST list APIs remain the source of truth for full logs and credentials pages. WebSocket events only keep the current admin session fresh between explicit queries.
