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
- **A Lease is a row keyed on a top-level Task, and it covers that Task's whole nested tree.** A `BEFORE INSERT` trigger rejects one keyed on a Subtask. A write five levels down needs the root's Lease; that blocking cost is deliberate, and `docs/adrs/0002-subtask-tree-is-one-aggregate.md` is why.
- **A Subtask nests to five levels, never moves, and cannot leave its parent complete.** All three are triggers on `task`, so no surface can offer a re-parent or a sixth level by accident. `depth` is folded from the parent's rather than walked. The completion rule consults direct children only, since an open grandchild keeps its own parent open.
- **A List and a Tag are aggregates of their own, and a Task carries the id rather than the text.** Each exists before any Task carries it and outlives the last one that did, so a rename or a recolour is one append however many Tasks show it. A Tag's frequency is counted at read time, never stored.
- **Creating or renaming a List or a Tag takes no Lease; carrying one does.** A Lease covers a top-level Task and its tree, and a List is in nobody's tree. Membership is a write to the Task, so it goes through the same guard as any other.
- **A sort orders siblings and never flattens the tree.** `Query.Sort` substitutes the key into the depth-first path, so roots and children both come back in the asked-for order. A Task with no deadline sorts after every Task that has one.
- **`Tasks` comes back depth first**, a parent followed by everything under it before the next root, so a surface indents by `Depth` rather than rebuilding the tree.
- **The guard sits on the append, not on the folded row.** The fold is a trigger on the append, so guarding the append guards every write exactly once and makes an unleased entry unrepresentable. The predicate walks to the root and then looks the Lease up by primary key; joining `lease` against the walk re-runs the recursion per row and costs ten times as much.
- **Leases are symmetric.** Nothing in the API names a holder or tells a person's Lease from an Agent's. `ErrHeld` says a Lease is held and no more.
- **Expiry is the clause `expires_at > now`, never a sweep.** No reader deletes a stale Lease row. Taking over a stale one appends Lease Broken, so a crashed run leaves a trail; there is no Lease Expired.
- **Instants are stored in the fixed-width `stamp` format, not `time.RFC3339Nano`.** RFC3339Nano trims trailing zeros from the fraction, which puts `12:00:00.5Z` before `12:00:00Z` under SQLite's TEXT comparison. `expires_at > now` and `deadline < now` both depend on lexicographic order being chronological order.
- **Reads write nothing.** Overdue is `deadline < now`, a stale Lease is a clause in a predicate, and an Occurrence is computed from its Series. A read that appends anything is a defect with a test against it.
- **Every write carries an Actor; reads are not attributed.** `TODO_ACTOR` names an Agent, and a person falls back to their login.
- **A writing verb takes the Lease covering its target's tree, writes, and gives it back.** A verb acts and exits, so it holds a 30-second TTL and never strands a tree behind a dead process.
- **An edit says what it touches and nothing else.** An attribute flag nobody typed leaves its attribute alone; one given empty clears it. The fold reads `json_type(payload, '$.x') IS NULL` for untouched and a JSON null for cleared, which is the distinction `COALESCE` cannot express, and `fs.Visit` is what makes the CLI say it.
- **Exit status is 0 for done, 1 for a failure, 2 for usage, 3 for a refusal.** A refusal is the store turning a write away — no Lease, or one another Actor holds — and a surface distinguishes it from a crash. A bad attribute value is refused where it was typed, so `store.ParseLevel` is the CLI's, not only the write path's.
- **`TODO_DB` overrides the store path**, which is how a test and an Agent run against a store of their own. The default sits under the user config directory.

## Work Guidance

- Tests first, at the seams the domain already draws: append-only, purity of reads, the Lease predicate, gaplessness under contention.
- Concurrency claims are asserted against real OS processes, not goroutines. Goroutines share one `*sql.DB` and one pool, so they never reach the file lock the defect hides behind. `TestMain` re-executes the test binary as the child.
- The store's exported vocabulary is `CONTEXT.md`'s: Task, Subtask, List, Tag, Lease, Attachment, Series, Occurrence, Actor.
- A Task's attributes are the set `docs/features.md` asks for and no more: title, description, why, creation date, deadline, estimate, priority, impact, snooze, colour, and any number of key/value pairs. Priority and impact are the same three levels, each carrying an example in `PriorityExamples` and `ImpactExamples`. The four offered snoozes are 1 hour, 1 day, 1 week, 1 month, and the month one clamps to the target month's last day.

## Verification

`go test ./...` from this directory, and `scripts/check` from the repository root for the gate CI runs.

## Child Index

None.
