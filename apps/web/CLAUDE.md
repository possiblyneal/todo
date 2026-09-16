# apps/web

## Purpose

The person's surface: a TypeScript browser client, and the only thing a person
looks at once `docs/plans/browser-client.md` reaches its deletion stage. It
reads the tracker over the JSON `todo api` serves and builds to static files
that same process serves beside the JSON, so there is no second process and no
CORS. `docs/adrs/0003-replace-the-tui-with-a-browser-client.md` records why the
surface moved off the terminal.

Stages 1 and 2 of the plan are what is here: the list, read-only, over
`GET /api/state`, and the box that hands a dump to the Broker and opens the add
sheet filled in.

## Ownership

- `src/api.ts` — the fetch plumbing every call shares: one JSON body out and
  one back, and the one place a failed response becomes an Error.
- `src/state.ts` — the wire shapes and the one call that reads them. It mirrors
  `apps/todo/src/api/state.go`, which is the side that decides them.
- `src/write.ts` — the wire shapes the write and Broker routes take, and the
  calls that reach them. It mirrors `apps/todo/src/api/tasks.go` and
  `apps/todo/src/api/broker.go`.
- `src/Box.tsx` — the box: a dump or a question, in the same field under the
  same thumb. Neither call writes.
- `src/Sheet.tsx` — the add sheet: what the Broker read, open for correction.
  Submitting it is the only thing that writes.
- `src/App.tsx` — the box above the list. It draws what the read returned and
  works nothing out for itself.
- `src/main.tsx` — the mount, and nothing else.
- `index.html`, `vite.config.ts`, `tsconfig.json`, `eslint.config.js` — the
  build. The dev server proxies `/api` so development has the one origin
  production has.

## Local Contracts

- **Vite, React, and nothing else.** No router, no data-fetching library, no
  component kit. A dependency added here is a decision, not a convenience.
- **The client works nothing out that the store already did.** `Task.marks` is
  drawn as it arrives; a Task that read as snoozed from a keyboard cannot read
  as plain here.
- **`Depth` is 1 for a top-level Task.** `Tasks` comes back depth first, so the
  list indents by what a Task has beyond the top rather than by `depth` itself.
- **A 304 is nothing to redraw, not an empty state.** `fetchState` returns
  `null` for one and the view keeps what it has; the ETag is handed back on the
  next poll, which is the TUI's once-a-second write-ahead-log poll over HTTP.
- **One poll at a time.** The next is scheduled once the one before it has
  settled rather than on an interval, so two are never in flight together: the
  slower of an overlapping pair draws its older list over the newer one and
  leaves a stale ETag to be answered `304` against.
- **A failed poll takes nothing off the screen.** The error is drawn over the
  list it interrupted, because a read that failed says nothing about the Tasks
  already drawn, and a phone that walked out of range gets them back when it
  walks back rather than losing them on the way out.
- **An API error is shown in the API's own words.** The body's sentence is the
  one the CLI would have printed, so it is drawn rather than restated.
- **The sheet is the gate, and nothing before it writes.** Handing a dump over
  is a read: the Task comes back as a draft, and a draft abandoned leaves
  nothing behind. Only submitting the sheet calls `POST /api/tasks`.
- **Nothing the Broker said is dropped on the way to a field.** A deadline is
  typed rather than picked and a level that is none of the three is offered as
  a fourth, because a control that can hold only what it can parse would blank
  the Broker's answer before anybody saw it. What the API cannot read it says
  so about, in its own words, with the value still in the field.
- **A dump survives backing out of the sheet.** Somebody who changed their mind
  about the Task has not changed their mind about having typed the sentence.
- **The list redraws on the next poll, not on the write.** A write answers with
  an id and nothing else; the poll a second later is what puts the Task on the
  screen, so there is one description of the list and it is the read's.
- **Touch targets no smaller than 44px, one thumb, no hover.**

## Work Guidance

- Formatting is Prettier's alone. `eslint-config-prettier` sits last in
  `eslint.config.js`, so `npm run lint` and `npm run format:check` cannot
  disagree about a line.
- The npm scripts here are the interface `scripts/libs/detect.sh` dispatches
  node on, through the root workspace manifest. A script renamed here goes
  unrun and reports `unavailable`, which fails the gate.
- No `public/`: `scripts/structure` reads a folder under a unit that is neither
  `src/` nor a scoped folder as a domain and requires a `src/` inside it.

## Verification

`npm run lint`, `npm run typecheck`, `npm run test` and `npm run build` from
the repository root, or `scripts/check` for the gate CI runs.

Tests run under `environment: 'node'`: they reach the modules that talk to the
API and not the components, since rendering one would mean a DOM environment
and a testing library, which are dependencies nobody has decided on.

## Child Index

None.
