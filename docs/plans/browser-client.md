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
encoding and no rules. Every handler reaches the store the way a verb does,
which is more than the bare call: a lifecycle write is `s.WithLease(actor, id,
store.WriteTTL, ...)` around it in `src/cli/verbs.go`, and a handler calling
`s.CompleteTask` alone is refused. So the take-write-release shape moves
somewhere both callers share before `src/api/` repeats it; repeating it is the
second path through the rules ADR 0003 warns about. A handler that validates
something the store does not is the other defect this plan is most likely to
introduce.

- `GET /api/state` — everything one screen needs in one response: the Tasks
  `Store.Tasks` returns for a `store.Query` with their `Marks` and `Depth`, the
  Lists, and the Tags. The `ETag` is `store.WALToken` hashed together with the
  query that produced the response, because the token is store-global and the
  response is not: changing a filter or a sort with no write in between would
  otherwise be answered `304` for a different representation. An empty token means
  the log could not be stat'd rather than that nothing changed, so it is not an
  `ETag` and the response carries none. The TUI polled the write-ahead log once a
  second; the client does the same thing over HTTP.
- `POST /api/tasks`, `PATCH /api/tasks/{id}`, `POST /api/tasks/{id}/{verb}` for
  the four lifecycle verbs, `POST /api/tasks/{id}/subtasks`.
- `POST /api/lists` to create, since `AddList` mints the id, and
  `PATCH|DELETE /api/lists/{id}`; the same for tags.
- `GET|PUT /api/tasks/{id}/series`, plus the Occurrence marks: tick, skip,
  detach.
- `POST /api/capture`, `POST /api/ask`, `POST /api/breakdown` — the Broker calls.
  Each returns what the Broker read and writes nothing; a following ordinary write
  is what makes anything durable. `capture` and `ask` are the two the client is
  built around, not extras hung off the side of it.
- `GET /api/tasks/{id}/history` — what has happened to one Task, for the detail
  page: the `Entry` rows as they are, Actor verbatim. This is the first of the two
  store additions the plan owes, below.
- `GET /api/history` — the same rows across every Task, newest first, for the
  activity screen. `Store.History` returns the whole log oldest first and unbounded,
  so this route needs a page of it rather than all of it: the second store addition
  the plan owes, below.

Status codes carry the CLI's four exit meanings: `200`/`201` for done, `400` for
usage, `409` for a refusal (`store.ErrRefused`, `store.ErrHeld`), `500` for a
failure. An error body is `{"error": "<the sentence the CLI would print>"}`.

Auth is nothing, deliberately, and the listener binds to the LAN address. That is
`todo serve`'s posture minus the public key, and it is what ADR 0003's first
re-check trigger is about.

## The client

Three screens and a box, which is what a phone has room for.

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

The who is two halves, on the terms `CONTEXT.md` sets out under **Actor**. The
client splits on the first slash so the log says that Opus completed this one and
Fable added that one, and which agent each was acting as, and it draws the raw
string whenever there is no slash. It recognises no model by name and judges
nothing it does not recognise.

**The activity screen is what agents did.** The Change History across every Task,
newest first, each entry drawn as who, what and when, with a filter for the writes
whose Actor has a slash in it. Agents add and complete Tasks while nobody is
watching, and this is the screen that makes a week of that visible without opening
one Task at a time to find it. It is a read of rows that already exist: it takes no
Lease, appends nothing, and shows entries whatever their Actor looks like. Lease
bookkeeping is not activity: `lease_taken`, `lease_released` and `lease_broken`
bracket every guarded write under the writer's own Actor, so the screen drops those
kinds or it is two thirds plumbing. The
filter narrows and never hides: the screen opens unfiltered, and an Agent that
named itself with no slash — one run with no `TODO_ACTOR`, or anything appended
before the convention existed — is in that view. `CONTEXT.md` makes such an Actor
legal, so a screen that only ever showed slashes would be the one place a badly
named Agent disappears.

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
3. **The rest of the writes, the detail screen and the activity screen**:
   lifecycle, subtasks, lists and tags, `GET /api/tasks/{id}/history` and the
   history log it draws, `GET /api/history` and the activity screen over it.
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

The store, with two named exceptions. No schema change, no new table, no new rule.
If a stage here needs one beyond those two, that is a finding worth stopping on
rather than a step to take quietly.

**Both exceptions are reads**, and both are the same missing shape: `Store.History`
returns every entry there is, oldest first and unbounded, and nothing narrows it.
The detail screen needs it narrowed by subject. The activity screen needs it newest
first and bounded to a page, because drawing it means decoding the whole log on
every load and `HistoryLength` is the only thing that counts today. Neither appends,
folds or enforces anything, which is what keeps them inside "reads write nothing"
rather than outside this plan.
