---
type: Architecture Decision Record
title: Ship Todo as One Go Binary
description: The repository holds a single deployable, `apps/todo`, written in Go — not because a constraint demanded Go, but because all five choke-point constraints were checked and none binds, leaving the time-to-working-code tiebreaker to decide on the evidence gathered about the SSH surface.
scope: [apps/todo, lang:go]
tags: [deployable-boundary, language-choice, tui, ssh]
generated: { by: "agent/claude-opus-5", at: "2026-09-04T18:46:37Z" }
superseded_by:
status: accepted
---

# Ship Todo as One Go Binary

## Decision

The repository holds exactly one deployable, `apps/todo`, written in **Go**. Three
bounded contexts — Tracking, Scheduling, Change History — collapse into that one
artifact, which has three modes: bare `todo` opens the TUI, `todo <verb>` acts and
exits, and `todo serve` listens on SSH so a phone reaches the same TUI over
`charmbracelet/wish`, LAN only.

This covers how many `apps/<name>/` directories exist, what the one is called, and
what language it is written in. It does not cover what that binary does, its schema,
its store layout, or its `run` and `ships` values — those stay at the shipped
`run: none, ships: {kind: none}` until the first real commit establishes them.

## Context

The choke point is **none of the five constraints binds**, and the choice fell to
the tiebreaker: **time-to-working-code**.

That is a finding rather than a silence, and it is why this record exists. All five
lines of the rubric were put to the operator and checked — two of them, network
concurrency and a hard hardware limit, for the first time — and every one came back
negative. A constraint checked and found not to bind and a constraint nobody looked
at are indistinguishable a year later; this section is what tells them apart.

The choke point was established by **reasoning** from the Step 5 seam contracts. It
was not measured, and it was not chosen off the short-form list. The seam contracts
found that no seam is a process boundary: the person's surface, the agent's CLI, and
concurrent modification all resolve to in-process calls against SQLite embedded as a
library, so the store is zero deployables. One seam later turned out to be a process
boundary — SSH — but it carries keystrokes and rendered cells rather than Tasks,
leaving the store, the Lease, and the Change History untouched.

Go wins on the evidence, not by default. The literal tiebreaker reads *"Python, or
whatever the repository already runs"*, and this repository runs only Bash. What
decided it is where the work concentrates: SSH-serving is purpose-built in Go and
hand-rolled elsewhere, while the TUI widgets `docs/features.md` demands are at
parity across all three candidate ecosystems.

Two splits were put up and both were rejected on the record. `todo` + `todo-server`
fails because the server side has no separate choke point of its own. `todo` +
`todo-cli` fails because it undoes the reason a CLI was chosen at all — the CLI won
because it implements the standing obligations in exactly one place, and splitting
the binary puts the Lease and the Change History rules in two artifacts.

`scripts/` is not a deployable. It is repository infrastructure the template ships
into every generated repository, and nobody installs or ships it, so it gets no
`apps/` directory and no language decision.

Derived with the operator across a `/wayfinder` map of eight tickets, all closed:
`possiblyneal/todo` issues #1 through #8.

## Alternatives Considered

**None of the five bind.** In those words, because that is the finding:

1. **A UI or device constraint.** Does not bind. One surface, a TUI, reached two
   ways. A web interface was raised mid-map and rejected in favour of SSH — the
   single decision keeping this line dead and TypeScript out of the repository.
   No browser executes anything.
2. **A deployment or glue constraint.** Does not bind. The repository's glue is
   `scripts/`, which is infrastructure rather than a deployable.
3. **An ecosystem only one language has.** Applies but does **not** select. Nine of
   the ten TUI components `docs/features.md` demands have a maintained option in
   Bubble Tea, Textual, and ratatui alike, and the one soft gap is Go's. SSH-serving
   is purpose-built in Go but hand-rolled rather than absent elsewhere: complete
   in-tree reference bindings run 273 lines (russh) and 179 lines
   (prompt_toolkit), a weekend to working and an estimated 1–2 weeks to
   production-shaped. Line 3's test is *reimplemented from scratch, costing real
   weeks*, and the from-scratch part — SSHv2 itself — is already maintained by
   asyncssh and russh. So the map's original given was overturned as a reason while
   its answer stood.
4. **Many long-lived connections with per-connection flow control.** Does not bind.
   `wish` made the line live; a stated 1–10 concurrent actors kept it from binding.
5. **A hard memory or hardware limit.** Does not bind, on three independent grounds:
   attachments are pointers the tracker never copies, local inference is an outbound
   HTTP call to an existing OpenAI-compatible service rather than an embedded model,
   and the store is an embedded SQLite.

**A second language.** Not added. The polyglot cost was charged and no second choke
point demanded a second toolchain. Five languages because five choke points demanded
them is architecture; a second one here would be drift.

## Consequences

One language and one toolchain. Every check in `scripts/` has exactly one stack to
detect, and the repository stays a single `go` install away from a working
environment.

**No root manifest ships with this generation.** Wayfinding establishes which
constraint binds; it does not establish a package manager, a version, or a project
layout. A `go.mod` under `apps/todo/` and none at the root is invisible to every
check — `scripts/doctor` fails on that shape rather than passing over it — so the
root manifest is what the first real commit must bring. Until then `scripts/ci`
honestly reports nothing to check, and every check becomes required the moment a
manifest is added.

**The decision rests on an absence, so it is due for re-checking sooner than one
resting on a measurement.** Three named triggers re-open it:

- an **MCP server** is added — it would wrap the same binary without touching the
  store contract, but it is a new surface this map did not weigh;
- **Tailscale or off-LAN reach** arrives, which breaks SQLite's same-host WAL
  assumption and puts line 4 and the zero-deployable store back in play;
- the concurrent actor count leaves the **1–10** range that keeps line 4 dead.
