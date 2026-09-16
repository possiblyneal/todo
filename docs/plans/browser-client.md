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
- `POST /api/capture`, `POST /api/ask`, `POST /api/breakdown` — the Broker calls.
  Each returns what the Broker read and writes nothing; a following ordinary write
  is what makes anything durable. `capture` and `ask` are the two the client is
  built around, not extras hung off the side of it.
- `GET /api/tasks/{id}/history` — what has happened to one Task, for the detail
  page: the `Entry` rows as they are, Actor verbatim. This is the one store
  addition the plan owes, below.

Status codes carry the CLI's four exit meanings: `200`/`201` for done, `400` for
usage, `409` for a refusal (`store.ErrRefused`, `store.ErrHeld`), `500` for a
failure. An error body is `{"error": "<the sentence the CLI would print>"}`.

Auth is nothing, deliberately, and the listener binds to the LAN address. That is
`todo serve`'s posture minus the public key, and it is what ADR 0003's first
re-check trigger is about.

## The client

Two screens and a box, which is what a phone has room for.

**The box is the front door.** A Dump typed one-handed is the most frequent thing
anybody does here, so it is one tap from the list and nothing is stacked in front
of it: type the Task the way you think of it, hand it over, and the ordinary add
sheet opens filled in for correction. Submitting is the only thing that writes, so
a Dump read and then abandoned leaves nothing behind. Asking a question sits on the
same box under the same thumb, because a question and a Dump are both "say a
sentence about the list"; the answer comes back as prose over the Tasks in view and
appends nothing.

**The list is the screen a person drives themselves**, and it is the one thing the
Broker does not touch. The tree indented by `Depth`, filtered by the sidebar's
states and Tags, sorted through `store.Sorts`, searched. A row is a tap to open it
and a swipe or a long press for the verbs that were `taskVerbs`.

**The detail screen is what a tap on a Task opens.** Everything the Task carries,
its Subtasks, its Series if it has one, the breakdown that proposes more Subtasks,
and its history: the entries the Change History holds for that Task, newest first,
each one who did it, what they did and when. That last part is the reason the
Change History is append-only made visible, and it is where an Agent's unattended
writes show up as somebody's writes rather than as changes that merely appeared.

The who is two halves. An Agent's Actor is `<harness>/<model>`, so the log says
that Opus completed this one and Fable added that one, and which agent each was
acting as; a person's is a bare login and draws as itself. The client splits on the
first slash and shows the raw string when there is no slash, which is what every
entry appended before the convention looks like. It validates nothing and it
recognises no model by name: OmniRoute fronts an open set and the list turns over,
so an unfamiliar model half is drawn, not judged.

Touch targets no smaller than 44px, one thumb, no hover.

## Order

Each stage is a branch and a pull request of its own.

1. **`todo api` with `GET /api/state` only, and a root `package.json` and
   `apps/web` scaffold that renders the list read-only.** This is the stage that
   proves the shape on the phone before anything is deleted.
2. **The box, and the writes underneath it**: `POST /api/capture` and the add and
   edit path it opens, plus `POST /api/ask`. Capture is sequenced here rather than
   later because it cannot land before the write path exists, not because it is
   secondary; it is the thing the client is for, and a stage that shipped the
   writes without it would be shipping the wrong half first.
3. **The rest of the writes and the detail screen**: lifecycle, subtasks, lists
   and tags, `GET /api/tasks/{id}/history`, and the history log it draws.
4. **Scheduling and the breakdown**: the Series screen, its four marks, and
   `POST /api/breakdown` with approval by position.
5. **The deletion**: `src/tui/`, `src/serve/`, `ModeTUI`, `ModeServe`, and the
   charm and wish requires out of `go.mod`. Bare `todo` becomes usage output.
   `apps/todo/CLAUDE.md` loses the TUI and serve contracts in the same commit.
6. **Packaging**: `apps/web/.unit.json` as `ships: {kind: none}` static files,
   and whatever serves the two on boot.

The deletion is late on purpose. The TUI stays working until the browser does
everything it did, so there is no window where the tracker has no usable surface.

## What this plan does not touch

The store, with one named exception. No schema change, no new table, no new rule.
If a stage here needs one beyond the exception, that is a finding worth stopping on
rather than a step to take quietly.

**The exception is a read.** `Store.History` returns every entry there is and
nothing reads one Task's, so the detail screen's history log needs a read narrowed
by subject. It appends nothing, folds nothing and enforces nothing, which is what
keeps it inside "reads write nothing" rather than outside this plan.
