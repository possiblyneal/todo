# apps/todo

## Purpose

The repository's single deployable: one Go binary with three modes. Bare `todo` opens the TUI, `todo <verb>` acts and exits, and `todo serve` serves the same TUI over SSH on the LAN. Three bounded contexts — Tracking, Scheduling, Change History — live in this one artifact. `docs/adrs/0001-ship-todo-as-one-go-binary.md` records why, including the three triggers that re-open the language choice.

## Ownership

- `src/cmd/todo/` — `package main`, nothing but the call into `cli`. It sits under `cmd/` because `scripts/package` names the artifact after the main package's import path, and a main at `src/` would build `src-linux-amd64`.
- `src/cli/` — mode dispatch and the verbs. `ModeOf` decides which mode an invocation asked for; `Run` takes its streams as arguments so every mode is testable without a process.
- `src/store/` — SQLite and the whole write path. Nothing else opens a database.
- `src/schedule/` — Scheduling's rule arithmetic: parsing a recurrence rule, writing it back, and the dates it produces. It opens no database and knows nothing about a Task.
- `src/datepicker/` — a calendar component for Bubble Tea v2, written here because nothing in Go ships one that compiles against it. It knows about dates and nothing about a Task, and reads no clock but its own `Now`.
- `src/tui/` — the main view, the add and edit screens, and the slash palette, built on `charm.land/bubbletea/v2` and `charm.land/huh/v2`. It writes through the same store calls the verbs use.

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
- **An Attachment is a pointer, and the tracker never holds a copy.** A file path or a web address, written down and nothing more: a path is resolved to an absolute one where it was typed, an address is kept as typed, and nothing is fetched. A target that moves leaves a dead link the tracker cannot tell you about, decided at issue #3, so there is no checker and never will be.
- **Deleting a Task collects nothing.** Nothing was copied in, so there is nothing to collect. The pointers stay on the deleted Task and what they name is untouched.
- **A Series is a rule and nothing else.** Scheduling's two tables hold ids, a rule, a date and a state; no title, description or estimate crosses into it, and `TestNoTaskContentEntersScheduling` reads both tables to check. Tracking points at the rule through `task.series_id`.
- **An Occurrence is computed on every look.** There is no row for a date until somebody ticks, skips or Detaches it, and then it is keyed by (series, date). A reader that shows next year's dates appends nothing and stores nothing.
- **Detaching one date lifts it out into an ordinary Task.** The copy carries the recurring Task's content, deadlined on that date and repeating on nothing, and the rule stops producing the date. Later edits to the rule do not reach it, which is the point of detaching rather than editing the Series.
- **A mark is an ordinary guarded write; changing the rule is too.** Each takes the Lease covering the Task's tree. Creating a Series and pointing the Task at it are one transaction, so a refusal leaves no Series behind.
- **Nothing in Scheduling runs on a schedule.** No sweep materializes dates and nothing is awake to notice one arriving; `src/schedule` says so in its package comment.
- **The charm modules are the `charm.land/...` v2 paths**, not `github.com/charmbracelet/...`. `bubblezone/v2` requires `charm.land/bubbletea/v2`, so mixing the two roots would put two incompatible `tea.Model` types in one program.
- **The TUI's mouse zones come from a per-Model `zone.New()`**, never `zone.NewGlobal()`. `todo serve` runs many sessions in one process, and a global manager would hand one session's hit boxes to another.
- **The v2 View carries the screen and mouse modes.** `Model.View` sets `AltScreen` and `MouseMode`; nothing is toggled through a program option behind the model's back.
- **The TUI never holds a write transaction across think-time.** A form gathers, `save` writes, and the Lease is taken and released inside that one call. Getting this wrong queues every other writer behind a person staring at a text box, which is why `busy_timeout` is ten seconds and not longer.
- **The TUI hears about other writes by polling the write-ahead log.** `Store.WALToken` is a stat, compared once a second. SQLite cannot push, there is no watcher service, and when the TUI is closed nothing watches because nothing needs to.
- **"/" opens the slash palette and "f" opens the searchbox.** Both are bubbles/list's fuzzy filter, one over verbs and one over Tasks; the palette gives the list back its "/" by rebinding `KeyMap.Filter`.
- **A deadline is picked, not typed.** The calendar is a `huh.Field` over `src/datepicker`, in the add and edit screens and again in the `/snooze` popup, with the four offered snoozes as its shortcut keys. There is no date validator left, because a date that cannot be typed cannot be mistyped.
- **The picker is the ecosystem gap being paid for.** The only Go date picker pins `bubbletea` v0.24.2; Textual and ratatui both ship one. ADR 0001 counted this cost before any code was written, and this is the entry against it.
- **A form's values live behind a pointer on the Model, never in a field on it.** `Model` is a value, so a form handed `&m.x` writes into the copy that built it and the copy that reads the answer sees nothing. `draft` and `popupDraft` are both pointers for that reason.
- **The file selector is `bubbles/filepicker`, opened with ctrl+a over the add or edit screen.** What it picks lands on the draft and the form is rebuilt around it, the way creating a List does. A web address cannot be walked to, so the form's Attach line takes one typed.
- **A form names every attribute it showed**, so one left empty is cleared. That is the opposite of the CLI's rule, and for the same reason: a surface says what the person saw and left.
- **The main view reads and writes nothing, asserted.** `TestTheMainViewWritesNothing` clicks, filters, sorts and expands, then compares `HistoryLength` with what it was.
- **Bare `todo` without a terminal is a usage error**, exit 2, rather than a crash inside the renderer. An Agent that runs `todo` by accident gets a sentence, not a hung process.
- **The Tag sidebar is ranked by frequency with variation drawn in.** Weighted sampling without replacement, so the most-carried Tag usually leads and the list does not read the same every time.
- **`TODO_DB` overrides the store path**, which is how a test and an Agent run against a store of their own. The default sits under the user config directory.

## Work Guidance

- Tests first, at the seams the domain already draws: append-only, purity of reads, the Lease predicate, gaplessness under contention.
- Concurrency claims are asserted against real OS processes, not goroutines. Goroutines share one `*sql.DB` and one pool, so they never reach the file lock the defect hides behind. `TestMain` re-executes the test binary as the child.
- `todo attach` is the pointers: bare with an id it lists them, a target adds one, and `-off` takes one off.
- `todo repeat` is the one verb that reaches Scheduling: bare it shows the rule and the next dates, words after the id set the rule, and `-tick`, `-skip`, `-detach` and `-off` are the rest.
- The store's exported vocabulary is `CONTEXT.md`'s: Task, Subtask, List, Tag, Lease, Attachment, Series, Occurrence, Actor.
- A Task's attributes are the set `docs/features.md` asks for and no more: title, description, why, creation date, deadline, estimate, priority, impact, snooze, colour, and any number of key/value pairs. Priority and impact are the same three levels, each carrying an example in `PriorityExamples` and `ImpactExamples`. The four offered snoozes are 1 hour, 1 day, 1 week, 1 month, and the month one clamps to the target month's last day.

## Verification

`go test ./...` from this directory, and `scripts/check` from the repository root for the gate CI runs.

## Child Index

None.
