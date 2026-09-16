---
type: Architecture Decision Record
title: Replace the TUI with a Browser Client over an HTTP API
description: The person's surface becomes a TypeScript browser client in `apps/web`, reached over a LAN-only JSON API served by the Go binary; the TUI and `todo serve` are deleted, and the repository holds two deployables and two toolchains.
scope: [apps/todo, apps/web, lang:go, lang:typescript]
tags: [deployable-boundary, language-choice, ui-surface, http-api]
generated: { by: "agent/claude-opus-5", at: "2026-09-15T00:00:00Z" }
supersedes: docs/adrs/0001-ship-todo-as-one-go-binary.md
superseded_by:
status: accepted
---

# Replace the TUI with a Browser Client over an HTTP API

## Decision

The person's surface is a browser. `apps/web` is a new deployable, written in
**TypeScript**, and it is the only thing a person looks at. It reaches the tracker
over a JSON API served by `apps/todo`, which stays **Go**, keeps the store and the
CLI verbs, and gains a mode that listens on HTTP on the LAN.

`src/tui/` and `src/serve/` are deleted, and so are the two modes in `src/cli`
that start them: `runTUI` reaches for Bubble Tea itself. With those gone the Bubble
Tea, Huh, Lipgloss, bubblezone and Wish requires have no caller left and come out
of `go.mod`. `todo serve` goes with them, and bare `todo` no longer opens anything.

This covers how many `apps/<name>/` directories exist, what language each is
written in, and which of them a person touches. It does not cover the API's route
shapes, the client's framework, the rendering strategy, or how the two are served
in production; `docs/plans/browser-client.md` carries those.

It does not touch the store. Tracking, Scheduling and Change History stay where
they are, behind the same in-process calls, and every rule this repository
enforces — the append-only history, the folds as triggers, the tree-scoped Lease,
the five-level nesting — is unmoved and unrestated.

## Context

**ADR 0001's line 1 was checked and it was wrong.** It read: *"A UI or device
constraint. Does not bind. One surface, a TUI, reached two ways. A web interface
was raised mid-map and rejected in favour of SSH — the single decision keeping
this line dead and TypeScript out of the repository. No browser executes
anything."* The operator's phone is the primary client, and an SSH-served TUI is
not a touch surface: it is a keyboard program rendered onto a screen that has no
keyboard. `todo serve` answered reach and mistook it for interaction. The device
constraint binds, so line 1 is live and it selects, which is what a supersede
rather than an amendment is for.

The other four lines were re-checked at the same time and none of them moved:

- **Line 2, deployment glue.** Still `scripts/`, still infrastructure. The client
  is a deployable because somebody installs and serves it, not because it needs
  glue.
- **Line 3, a single-language ecosystem.** Now points the other way. Browser
  rendering is TypeScript's and is hand-rolled or transpiled everywhere else;
  SSH-serving, the thing that pointed at Go, is the capability being deleted.
- **Line 4, many long-lived connections.** Still dead. HTTP requests are short and
  the actor count is unchanged; the client polls or reloads rather than holding a
  session, which is strictly less than the pty per phone that `wish` held.
- **Line 5, a hard memory or hardware limit.** Unchanged. The store is still
  embedded SQLite, inference is still an outbound call, attachments are still
  pointers.

**LAN only stands, so ADR 0001's second re-check trigger is not pulled.** The Go
process owns the SQLite file on its own host and is the only thing that opens it;
the phone gets JSON over the LAN and never touches the write-ahead log. Off-LAN
reach is a separate decision with its own record, and it arrives with an auth
question this one does not answer.

## Alternatives Considered

**Go serves HTML, with or without HTMX.** Rejected. It is the cheaper answer and
it keeps one toolchain, which is exactly what ADR 0001's "a second language, not
added" was protecting. It was put up and turned down on what is wanted: an
app-shaped surface on a phone, where a tick is immediate and the thing behaves
like an application rather than like a page. Buying that back on top of
server-rendered HTML is where the JavaScript arrives anyway, and it arrives
untyped, unbuilt and spread through templates rather than in one deployable that
the checks can see.

**Keep the TUI alongside the browser.** Rejected. Two person-facing surfaces over
one store is two places every future verb has to be added and two places it can be
added wrong, paid for a surface that would stop being opened. Deleting it is
recoverable: it is in the history, and ADR 0001 records what it was for.

**A second binary for the API.** Rejected, on ADR 0001's own reasoning, which
survives this supersede intact: the CLI won its place by implementing the standing
obligations in exactly one place, and an API in a second artifact puts the Lease
and the Change History rules in two. The API handler is another caller of the same
in-process store calls, in the same binary the verbs are in.

## Consequences

**Two toolchains, and the polyglot cost is now charged and accepted.** Every
capability in `scripts/` has two stacks to detect instead of one, and the
repository is a `go` install and a Node install away from a working environment.
`node` is already in `DETECT_LANGUAGES`, so no adapter work is owed, but the
orphan-manifest rule means a `package.json` under `apps/web/` needs a root
workspace manifest in the same commit that creates it, exactly as `go.work` was
owed to `apps/todo/go.mod`.

**The API is a contract, and it is the third thing that must not become a second
path through the rules.** A verb, the API, and anything later all make the same
in-process call. A rule enforced in a handler rather than in the store is a rule
an Agent running a verb does not get.

**The tracker is now two things to start.** Bare `todo` opened a working program;
now a person needs the API listening and the client served. That is a real loss
and it is the price of the surface.

**A build step exists where none did.** The Go binary was the artifact; now there
is a compiled client beside it, and `apps/web` ships as static files rather than
as an executable.

**Three triggers re-open this:**

- **off-LAN reach arrives** — it breaks nothing about the store the way a network
  filesystem would, but it puts an unauthenticated JSON API on a wider network and
  the auth question this record deliberately left out becomes the deciding one;
- **a second person or a third device** — the client assumes one operator and no
  identity beyond the Actor string a write carries;
- **the client needs to work offline** — a phone with no route has no Tasks today,
  and changing that means a store on the device, which is a different architecture
  and not a feature of this one.
