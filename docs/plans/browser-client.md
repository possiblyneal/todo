# Plan: the browser client

Carries out `docs/adrs/0003-replace-the-tui-with-a-browser-client.md`. The ADR
decides the deployables and the languages; this decides the routes, the framework
and the order the work lands in.

## Shape

Two deployables, both on one host on the LAN.

- `apps/todo` — Go. The store, the CLI verbs, and a new `todo api` mode that
  listens on HTTP and serves the compiled client's files beside the JSON. One
  process, one open SQLite file, unchanged.
- `apps/web` — TypeScript. Vite, React and nothing else to start: no router, no
  data-fetching library, no component kit. It builds to static files `todo api`
  serves, so there is no second process and no CORS.

Framework choice is Vite plus React because it is the least surprising thing to
hand a phone and the checks in `scripts/` already dispatch `node` off a
`package.json`. A root `package.json` naming `apps/web` as a workspace lands in
the same commit as the app's own, because `detect_orphan_manifests` fails a
nested manifest with no root above it — the same rule `go.work` satisfies for
`apps/todo/go.mod`.

## The API

`src/api/` in the Go binary, a sibling of `src/cli/`, holding routing and JSON
encoding and no rules. Every handler is another caller of the same store call a
verb makes; a handler that validates something the store does not is the defect
this plan is most likely to introduce.

- `GET /api/state` — everything one screen needs in one response: the Tasks a
  `store.Query` returns with their `Marks` and `Depth`, the Lists, and the Tags.
  Carries `store.WALToken` as an `ETag`, so the client's poll is a conditional
  request the server answers `304` to. The TUI polled the write-ahead log once a
  second; the client does the same thing over HTTP.
- `POST /api/tasks`, `PATCH /api/tasks/{id}`, `POST /api/tasks/{id}/{verb}` for
  the four lifecycle verbs, `POST /api/tasks/{id}/subtasks`.
- `POST|PATCH|DELETE /api/lists/{id}` and the same for tags.
- `GET|PUT /api/tasks/{id}/series`, plus the Occurrence marks: tick, skip,
  detach.
- `POST /api/capture`, `POST /api/breakdown`, `POST /api/ask` — the broker calls,
  each returning proposals the client shows and nothing durable until a following
  write.

Status codes carry the CLI's four exit meanings: `200`/`201` for done, `400` for
usage, `409` for a refusal (`store.ErrRefused`, `store.ErrHeld`), `500` for a
failure. An error body is `{"error": "<the sentence the CLI would print>"}`.

Auth is nothing, deliberately, and the listener binds to the LAN address. That is
`todo serve`'s posture minus the public key, and it is what ADR 0003's first
re-check trigger is about.

## The client

One screen, which is what a phone has room for: the Task list with a bottom bar,
and everything else a sheet over it.

- The list is the tree, indented by `Depth`, each row a tap to expand and a long
  press or a swipe for the verbs that were `taskVerbs`.
- The bottom bar is add, search, sort, and the sidebar's states and Tags behind
  one filter sheet. `viewVerbs` minus quit.
- The add and edit sheet is the same gate the TUI's was: the broker fills it in,
  a person corrects it, and submitting is the only thing that writes.
- Touch targets no smaller than 44px, one thumb, no hover.

## Order

Each stage is a branch and a pull request of its own.

1. **`todo api` with `GET /api/state` only, and a root `package.json` and
   `apps/web` scaffold that renders the list read-only.** This is the stage that
   proves the shape on the phone before anything is deleted.
2. **The writes**: tasks, lifecycle, subtasks, lists and tags, in the API and in
   the client.
3. **Scheduling and the broker**: the series screen and the three AI calls.
4. **The deletion**: `src/tui/`, `src/serve/`, `ModeTUI`, `ModeServe`, and the
   charm and wish requires out of `go.mod`. Bare `todo` becomes usage output.
   `apps/todo/CLAUDE.md` loses the TUI and serve contracts in the same commit.
5. **Packaging**: `apps/web/.unit.json` as `ships: {kind: none}` static files,
   and whatever serves the two on boot.

Stage 4 is last on purpose. The TUI stays working until the browser does
everything it did, so there is no window where the tracker has no usable surface.

## What this plan does not touch

The store. No schema change, no new table, no new rule. If a stage here needs one,
that is a finding worth stopping on rather than a step to take quietly.
