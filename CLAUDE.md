## What this repository is

`todo` is a task tracker with two kinds of consumer: a person at a keyboard, and agents that add, edit, and delete while nobody is watching. Both reach the same store through the same calls; neither gets a weaker or a stronger contract than the other. `CONTEXT.md` holds the language that keeps those two from meaning different things by the same word, and `docs/features.md` records the operator's wish list verbatim as source material rather than as a specification.

The repository holds **one deployable**. Three bounded contexts — Tracking, Scheduling, Change History — collapse into a single Go binary with three modes: bare `todo` opens the TUI, `todo <verb>` acts and exits, and `todo serve` serves the same TUI over SSH on the LAN. The store is SQLite embedded as a library, so it is not a deployable of its own. `docs/adrs/0001-ship-todo-as-one-go-binary.md` records why, including the three triggers that re-open the language choice.

The root manifest is `go.work`, and it is what makes the module under `apps/todo/` visible to every check. A `go.mod` there with no `go.work` above it is an orphan `scripts/doctor` fails on rather than passing over, so a new module gets a `use` line in the same commit that creates it.

## Commands

Located at `./scripts` Use these instead of per-language tools; each detects the languages present and fails when an expected check cannot run. `./scripts/CLAUDE.md` documents all of them.

## Git

- Pre-commit blocks direct commits to `main` and `master`. Branch before making changes.
- Run `scripts/check` before committing. It runs the same checks CI does, plus pre-commit across every file rather than the staged ones.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/#specification), and carry a `Generated-By: <model>` trailer. `scripts/attribute-commit` writes it at `prepare-commit-msg` by rewriting an agent's `Co-Authored-By` line; it never inserts one, so a message with no agent trailer is refused and a hand-written commit adds its own.
- This repo adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
- Pull requests merge; they are neither squashed nor rebased.
- Plan mode writes to `docs/plans/`, which is tracked. A plan lands in the diff alongside the code it describes.

## Agent skills

### Issue tracker

GitHub Issues on `possiblyneal/todo`, via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The seven canonical roles, unrenamed. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` at the root, ADRs in `docs/adrs/`. See `docs/agents/domain.md`.

### Session launcher

Orca, on this machine. See `docs/agents/session-launcher.md`.

## Child Index

- `scripts/CLAUDE.md` — the language-capabilities interface, the result states, the test harness, and what adding a language or check requires
- `apps/todo/CLAUDE.md` — the single deployable: its package layout, the store's enforced rules, and what each mode owns

`docs/` owns a `CLAUDE.md` once it holds enough to need a local contract.
