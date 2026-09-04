# Scripts

## Purpose

Every portable shell script, whether a person runs it or a workflow does. Project logic lives here rather than in YAML so it runs locally exactly as it runs in GitHub Actions.

## Ownership

One flat directory, not a `ci/` and a `scripts/` split. The boundary that split would need does not hold: `check` calls `ci` and `security`, `release` calls `ci`, and every script sources the same detection library, so a file's caller is not a property that stays put.

- `doctor`, `check`, `fix`, `clean`, `run`, `package` — run by a person
- `ci`, `security`, `release`, `detect` — called by `.github/workflows/`
- `repo-settings check` — hosted GitHub state, run explicitly
- `adr-index` — called by pre-commit; regenerates `docs/adrs/index.md`
- `structure` — called by `scripts/check` and by pre-commit; audits where files sit; the script itself holds the rules
- `changelog-check` — called by pre-commit at `pre-push`, and by `ci.yml` over a pull-request range; validates `CHANGELOG.md` structure
- `protect-branch` — called by pre-commit at `pre-push`; refuses a push whose destination ref is `main` or `master`
- `worktree-cleanup` — called by pre-commit at `post-checkout` and `post-merge`, and by the SessionStart hook with `--report`; prunes Git's records for worktrees whose directories are gone
- `libs/detect.sh` — the detection library, sourced by all of the above
- `libs/unit.sh` — which unit an invocation acts on and what that unit declared; sourced by `run`, `package`, and `release`
- `libs/precommit.sh` — which git hooks the config asks for and which this clone lacks; sourced by `doctor` and by `~/.claude/hooks/session-start.sh`
- `libs/result.sh` — how a check reports: the four Result states, the printed layout, findings, and the tally; sourced by `libs/detect.sh`, and so by every command, and by `structure`
- `libs/quadlet.sh` — `quadlet_validate <dir> <label>`, validation of a `quadlet` unit's `deploy/quadlet/` pair, the label naming it in findings; sourced by `package`
- `tests/*-test` — assertions about the wiring itself
- `tests/libs/harness.sh` — the assertion counting those tests share, and the fixture primitives they compose: `scratch_repo`, `fixture`, `declare_unit`, `quadlet_pair`, `stub`, `minimal_path`, `skip`. `fixture <name>` is a fresh scratch repository named as the current case, the default a suite overrides when its cases need more; `minimal_path <tool…>` builds a directory of symlinks to just those tools and echoes it, for a case that runs a command with a named tool absent from `PATH`

A helper that must be built before it runs belongs in `tools/`, not here.

## Local Contracts

- **`libs/detect.sh` owns every language decision.** Commands name a capability, never an adapter; `DETECT_LANGUAGES` and `DETECT_PRUNE_DIRS` are the lists.
- **The orphan rule tests the exact root manifest, not language presence.** `detect_orphan_manifests` prints a line per orphan; `judge_orphan_manifests` records them and stops `ci` and `security`.
- **A check that did not run is not a check that passed.** `unavailable` fails the run whenever the check was expected; a Kotlin or Swift audit without a lockfile and the secret scan without gitleaks are `not-applicable`.
- **`libs/result.sh` owns how a check reports, and its printed layout is an interface.** `result` refuses a fifth state, `record` dedups, `tally` is a command's last line and its exit status. `would-run` is a marker, not a state.
- **A check owns no stdin.** `check` closes fd 0 for the suites and `structure`, and the dispatch closes it for every capability but `run`, so a stdin-draining tool cannot hang the gate; the `*_each` helpers take their list on fd 3, and pipes end in a variable, never a reader that exits early, since `pipefail` turns SIGPIPE into 141, read as absent.
- **Four runners exit non-zero without a real failure and are translated**: pytest on no tests, npm's placeholder test, `uv run` on a missing tool, `uv audit` on an older uv.
- **Go capabilities run once per module** through `go_each`, which fails on an empty `go list -m`; gofmt stays at the root. `_go_main_packages` yields one program, `NO_RUNNER`, or a refusal.
- **An adapter is honest about what it did.** Capture the tool's exit status, not only its output. No function stands in for an absent one: the probe reads `absent` off the function table, the dispatch reports `unavailable`, and `NO_RUNNER` never leaves it. Node's `run` refuses a missing `bin`.
- **`libs/*.sh` and `harness.sh` are sourced, never executed** — no shebang, no executable bit, `.sh`. Every other script is extensionless and executable; pre-commit reads a shebang only on one.
- **`harness.sh` owns the counting and the primitives**: `scratch_repo`, `fixture`, `declare_unit`, `quadlet_pair`, `stub`, `minimal_path`, `skip`. No suite changes directory, and only a case about `run` lets a stub read its stdin; `report` fails a suite that passed nothing and skipped something.
- **`libs/precommit.sh` owns which git hooks are owed, and parses `default_install_hook_types` rather than grepping: three valid YAML forms.** `adr-index` and `structure` are hooks, not capabilities, and run `always_run` with no `files:` filter, since staged paths exclude deletions.
- **`structure` holds the layout rules as code**; the only file it opens is a `.unit.json`. Depth stops at the first `src/`; a domain is its `src/`, not its name. It enumerates with `git ls-files`, needs `jq`, and reports the three content rules `not-applicable`.
- **`run` and `package` take their context as globals** from `libs/unit.sh`. `package` clears `dist/` before dispatch, only where `packaging_writes_dist` and never on a dry run, so a stale binary cannot satisfy `release`'s output guard.
- **`release` is the only script here that writes to GitHub.** Changelog section or generated notes, never both; it packages first, checks an `executable` unit produced a file, and runs `ci`.
- **`changelog-check` validates only the paths it is given, at `pre-push`** — form, not whether an entry was owed; CI re-runs it over the PR range.
- **`protect-branch` reads the push destination, not the current branch**, and is silent when unset.
- **`worktree-cleanup` prunes metadata, never a directory**, and reads only `--report`.
- **`check` globs tests with `nullglob` and sweeps untracked files in a second pass by path**, which `--all-files` cannot see.
- **Local commands stay off hosted GitHub state** — the boundary is hosted state, not connectivity, and only `repo-settings check` crosses it.
- **`repo-settings` reports and never changes**, since a ruleset write replaces rather than merges. One fetch, then judgments off it: preflight absences stop green, a plan 403 is `not-applicable` and any other refusal `unavailable`, CODEOWNERS above the admin gate.

## Work Guidance

Adding a language means: add it to `DETECT_LANGUAGES`, add its `has_<lang>` detector, add a `_capability_<check>_<lang>` function for each capability it can answer — a capability with no honest adapter is left absent, not stubbed — add a `_codeql_entry` row, add its toolchain to `.github/actions/setup-toolchains/action.yml`, and add its column to the table `tests/capabilities-test` holds against `language_capabilities probe`, naming which cells are deliberately `absent`. That suite fails when a present language is not dispatched to every check and when the probe's table differs from the one it holds — the one property nothing else reports on, since a dropped language produces a shorter green run rather than a failure.

Adding a check means adding it to `DETECT_CAPABILITIES` and writing an adapter per language, or declaring it not-applicable in `_capability_is_not_applicable`.

## Verification

Each suite is `scripts/tests/<name>-test`, reports through the harness, and asserts through the public surface.

- `capabilities-test` — the ten-by-six probe table, dispatch under `CI_DRY_RUN=1`, and real runs of `ci`, `security`, `run`, and `package` against stubbed toolchains
- `result-gate-test` — the four states are enforced and findings deduped, and every command ending in `tally` fails on an `unavailable` line or a recorded orphan
- `harness-test` — the harness itself, run as a process: the tally line and exit status for all-pass, one-fail, all-skip, and skip-beside-pass, plus each fixture primitive
- `clean-test` — `clean` prunes the directories `libs/detect.sh` names
- `unit-commands-test` — unit resolution, `run: none` and `ships: none`, quadlet validation with no container runtime, arguments after `--`, and `dist/` cleared only where a language packages there. Skips without `jq`
- `health-checks-test` — the offline boundary, with `gh` stubbed
- `repo-settings-test` — every preflight absence, every judgment in each of its answers, and the two orderings, with `gh` stubbed per endpoint. Skips without `jq`
- `release-test` — what `release` hands to `gh release create`, and each way the walk aborts before the tag is cut
- `adr-index-test` — the index converges and pre-commit invokes the hook
- `commitlint-test` — the `commit-msg` hook installs and commitlint judges a message. The only suite needing the network
- `worktree-cleanup-test` — pruning is correct and idempotent and tolerates each hook stage's arguments
- `session-start-test` — the operator's global `session-start.sh`, `not-applicable` where it is absent
- `changelog-check-test` — each structural rule, the shipped addon changelog, a missing file, a missing awk, an unrelated path
- `precommit-hooks-test` — `libs/precommit.sh` against each YAML form. Needs no network, which is the point: this wiring fails silently
- `structure-test` — each layout rule in both directions, `.structure-allow` at a path and at a prefix, every declaration value, and `jq` absent. Each fixture is a real repository with the script copied in, since it resolves its own root from `BASH_SOURCE`
- `protect-branch-test` — bare, qualified, near-miss, and unset destinations

`scripts/check` runs all sixteen before the checks they guard, then `structure` before `ci`. `ci.yml` runs them before toolchain setup and installs `pre-commit` first, so the two hook-wiring suites do not skip every case. shellcheck runs via pre-commit with `-x`.
