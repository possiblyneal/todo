# apps/todo

## Purpose

The repository's single deployable: one Go binary with three modes. Bare `todo` opens the TUI, `todo <verb>` acts and exits, and `todo serve` serves the same TUI over SSH on the LAN. Three bounded contexts — Tracking, Scheduling, Change History — live in this one artifact. `docs/adrs/0001-ship-todo-as-one-go-binary.md` records why, including the three triggers that re-open the language choice.

## Ownership

- `src/cmd/todo/` — `package main`, nothing but the call into `cli`. It sits under `cmd/` because `scripts/package` names the artifact after the main package's import path, and a main at `src/` would build `src-linux-amd64`.
- `src/cli/` — mode dispatch and the verbs. `ModeOf` decides which mode an invocation asked for; `Run` takes its streams as arguments so every mode is testable without a process.
- `src/store/` — SQLite and the whole write path. Nothing else opens a database.

## Local Contracts

- **A verb and the TUI make the same in-process call.** The CLI won its place by implementing the standing obligations in exactly one place; a second path through the rules would give an Agent and a person different contracts.
- **Every connection DSN carries `_txlock=immediate`.** `database/sql` issues a deferred `BEGIN`, and the deferred path loses writes under contention. `store.dsn` holds the measurement, and `concurrency_test.go` fails if the flag goes.
- **The Change History is append-only, enforced by trigger.** `UPDATE` and `DELETE` against it `RAISE(ABORT)`. A deletion is an appended entry, never an erasure.
- **The fold runs inside the appending writer's transaction.** Current state and the entry producing it commit together. Nothing sweeps, nothing is awake between invocations, and no reader materializes.
- **Reads write nothing.** Overdue is `deadline < now`, a stale Lease is a clause in a predicate, and an Occurrence is computed from its Series. A read that appends anything is a defect with a test against it.
- **Every write carries an Actor; reads are not attributed.** `TODO_ACTOR` names an Agent, and a person falls back to their login.
- **`TODO_DB` overrides the store path**, which is how a test and an Agent run against a store of their own. The default sits under the user config directory.

## Work Guidance

- Tests first, at the seams the domain already draws: append-only, purity of reads, the Lease predicate, gaplessness under contention.
- Concurrency claims are asserted against real OS processes, not goroutines. Goroutines share one `*sql.DB` and one pool, so they never reach the file lock the defect hides behind. `TestMain` re-executes the test binary as the child.
- The store's exported vocabulary is `CONTEXT.md`'s: Task, Subtask, List, Tag, Lease, Attachment, Series, Occurrence, Actor.

## Verification

`go test ./...` from this directory, and `scripts/check` from the repository root for the gate CI runs.

## Child Index

None.
