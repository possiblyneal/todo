# apps/web

## Purpose

The person's surface: a TypeScript browser client, and the only thing a person
looks at once `docs/plans/browser-client.md` reaches its deletion stage. It
reads the tracker over the JSON `todo api` serves and builds to static files
that same process serves beside the JSON, so there is no second process and no
CORS. `docs/adrs/0003-replace-the-tui-with-a-browser-client.md` records why the
surface moved off the terminal.

Stage 1 of the plan is what is here: the list, read-only, over `GET /api/state`.

## Ownership

- `src/state.ts` — the wire shapes and the one call that reads them. It mirrors
  `apps/todo/src/api/state.go`, which is the side that decides them.
- `src/App.tsx` — the list. It draws what the read returned and works nothing
  out for itself.
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

## Child Index

None.
