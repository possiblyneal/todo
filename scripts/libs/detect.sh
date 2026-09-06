# shellcheck shell=bash
# Language detection, defined once.
#
# Source this; do not execute it. It defines functions and constants and sets no
# state, so a caller keeps its own `set` options. Every function reads paths
# relative to the current directory, and every caller cds to the repository root
# before sourcing.
#
# No shebang, and not executable: running it does nothing, and pre-commit's
# check-shebang-scripts-are-executable would otherwise require an executable bit
# that says this is a command. The .sh extension marks it as shell to the
# linter, and the directive above names the dialect the shebang used to.
#
# This exists because detection used to be copied across eight files, and the
# copies answered the same question differently. A language wired into some of
# them is worse than one wired into none: the checks that know about it pass,
# and the ones that do not report a clean run over work they never looked at.
#
# Pipes here end in a variable or a `-print -quit` test rather than feeding a
# reader that exits early. Under `set -euo pipefail` a `grep -q` or `head` that
# closes the pipe kills the writer with SIGPIPE and pipefail reports 141, which
# a detection function reads as "absent" — the one wrong answer that is silent
# rather than loud.

# Every result line here goes through the result library, so a caller that
# sources this file has the layout and the tally without sourcing it twice.
# shellcheck source-path=SCRIPTDIR
# shellcheck source=result.sh
source "$(dirname "${BASH_SOURCE[0]}")/result.sh"

# The single place the supported set is written down. Callers iterate this
# rather than keeping their own list.
DETECT_LANGUAGES=(node python go rust swift kotlin)

has_node() { [[ -s package.json ]]; }
has_python() { [[ -s pyproject.toml ]]; }
has_go() { [[ -s go.mod || -s go.work ]]; }
has_rust() { [[ -s Cargo.toml ]]; }
has_kotlin() { [[ -s settings.gradle.kts || -s build.gradle.kts ]]; }

# Directories holding manifests that belong to a dependency or another
# repository rather than to this one. Searching them reports another project's
# packages as this project's. `tmp` is here for the second reason: it is
# gitignored scratch, so nothing in it is this repository's to check, and a
# whole repository materialized there is the case that would otherwise fail
# every check that walks the tree.
DETECT_PRUNE_DIRS=(.git node_modules .build target .venv venv vendor tmp)

# DETECT_PRUNE_DIRS as a find(1) `-name a -o -name b ...` expression, assigned
# into the array named by $1. Its own copy of this list is the second copy
# scripts/clean once kept, and the one this repository's detection module
# exists to prevent.
detect_prune_expr() {
  local -n _detect_prune_out="$1"
  local dir
  _detect_prune_out=()
  for dir in "${DETECT_PRUNE_DIRS[@]}"; do
    _detect_prune_out+=(-name "$dir" -o)
  done
  unset '_detect_prune_out[${#_detect_prune_out[@]}-1]'
}

# find(1) with the directories above pruned. Arguments after the filename are
# forwarded, so callers choose -print, -print -quit, or a test of their own.
detect_find() {
  local name="$1" prune=()
  shift

  detect_prune_expr prune

  find . \( "${prune[@]}" \) -prune -o -name "$name" -type f "$@"
}

# Swift is the one supported language with no root manifest: SwiftPM has no
# workspace file, so packages sit at apps/<name>/ and libs/<name>/ and nothing
# at the root names them. Found by search, and the result is cached because that
# search walks the tree and no manifest appears or disappears mid-run.
_detect_swift_cache=""
has_swift() {
  if [[ -z "$_detect_swift_cache" ]]; then
    if [[ -n "$(detect_find Package.swift -print -quit)" ]]; then
      _detect_swift_cache=yes
    else
      _detect_swift_cache=no
    fi
  fi
  [[ "$_detect_swift_cache" == yes ]]
}

has_language() { "has_$1"; }

# Present languages, one per line, in DETECT_LANGUAGES order.
detect_present_languages() {
  local lang
  for lang in "${DETECT_LANGUAGES[@]}"; do
    if has_language "$lang"; then echo "$lang"; fi
  done
}

# Whether the repository is a project at all. A caller that found no runner
# needs this to tell "nothing to check yet" from "a manifest is present and its
# checks silently did not run", which produce the same output otherwise.
has_any_manifest() {
  local lang
  for lang in "${DETECT_LANGUAGES[@]}"; do
    if has_language "$lang"; then return 0; fi
  done
  return 1
}

# Returned by a check whose language has no wired-up runner, or whose runner is
# not installed here. Distinct from any exit code a real tool is likely to
# return, so "the tool reported a problem" is never read as "no tool ran".
#
# Assigned only when unset: re-assigning a readonly is an error, and under
# `set -e` that would abort a caller that sources this file twice.
if [[ -z "${NO_RUNNER:-}" ]]; then
  readonly NO_RUNNER=199
fi

# `uv run <tool>` exits 2 when the tool is not installed, which is the same
# shape as a tool that ran and reported problems. Reporting that as a failure
# blames the code for a missing dependency, so the tool is looked up first and
# its absence reported as no runner, which scripts/doctor then explains.
uv_run() {
  command -v uv >/dev/null 2>&1 || return "$NO_RUNNER"
  uv run --quiet "$1" --version >/dev/null 2>&1 || return "$NO_RUNNER"
  uv run "$@"
}

# Whether anything here needs trivy, which .github/workflows/security.yml reads
# to decide whether to install it. Package.resolved is committed by SwiftPM;
# Gradle writes gradle.lockfile only once dependency locking is turned on, so a
# Gradle repository without one has no resolved versions to scan.
has_trivy_target() {
  [[ -n "$(detect_find Package.resolved -print -quit)" ]] ||
    [[ -n "$(detect_find gradle.lockfile -print -quit)" ]]
}

# Runs trivy over every lockfile matching a name, since Swift and Gradle have no
# first-party audit command. Every lockfile is scanned even after one reports a
# vulnerability, so the first hit does not hide the rest.
#
# Read on fd 3, not stdin: trivy reads stdin itself in some modes, and a loop
# reading the manifest list on fd 0 would have that same read consumed by the
# command it runs, silently skipping every lockfile after the first.
trivy_each() {
  local lockfile status=0 found=1

  while IFS= read -r lockfile <&3; do
    [[ -n "$lockfile" ]] || continue
    found=0
    trivy fs --exit-code 1 --severity HIGH,CRITICAL --scanners vuln "$lockfile" || status=1
  done 3< <(detect_find "$1" -print)

  (( found == 0 )) || return "$NO_RUNNER"
  return "$status"
}

# Runs a command once per Swift package directory. Every package runs even after
# one fails, so a single broken package does not hide the state of the rest,
# while any failure still decides the exit status.
#
# Read on fd 3, not stdin, for the same reason as trivy_each: swift test/build
# can read stdin, which would otherwise consume the remaining manifest list.
swift_each() {
  local manifest status=0
  while IFS= read -r manifest <&3; do
    ( cd "$(dirname "$manifest")" && "$@" ) || status=1
  done 3< <(detect_find Package.swift -print)
  return "$status"
}

# Runs a command once per Go module. This is not a single run from the
# repository root because `./...` matches only packages inside a module, and the
# root of a workspace is not itself in one: `go build ./...` there refuses the
# pattern and exits 1, so every Go check on a repository whose only module sits
# under apps/ failed on the invocation rather than on the code, and no change to
# the code could turn it green. `go list -m` names the directories to enter --
# the main module for a plain go.mod, every `use` entry under a go.work.
#
# A listing that fails or comes back empty is a failure, not an empty success:
# it means the module graph could not be read at all.
#
# Read on fd 3, not stdin, for the same reason as swift_each: go test and go run
# can read stdin, which would otherwise consume the remaining module list.
go_each() {
  local dir status=0 modules
  modules="$(go list -m -f '{{.Dir}}')" || return 1
  if [[ -z "$modules" ]]; then
    echo "go list -m named no module directories" >&2
    return 1
  fi

  while IFS= read -r dir <&3; do
    [[ -n "$dir" ]] || continue
    ( cd "$dir" && "$@" ) || status=1
  done 3<<<"$modules"

  return "$status"
}

# Gradle exposes lint and format tasks only when the matching plugin is applied,
# so the task list decides which checks exist. Listing costs a daemon start, so
# the result is read once and reused.
_detect_gradle_tasks=""
gradle_has_task() {
  has_kotlin && [[ -x ./gradlew ]] || return 1

  if [[ -z "$_detect_gradle_tasks" ]]; then
    _detect_gradle_tasks="$(./gradlew -q tasks --all 2>/dev/null || true)"
    # A newline marks "listed, found nothing" so the next call does not relist.
    [[ -n "$_detect_gradle_tasks" ]] || _detect_gradle_tasks=$'\n'
  fi

  grep -qE "^$1( |$)" <<<"$_detect_gradle_tasks"
}

# `npm init -y` writes a placeholder test script that prints an error and exits
# 1. Counting it as a configured runner turns a scaffold that has never had a
# test into a failing test run, which reads as a broken suite. It is passed as
# an argument rather than written into the -e program, which would need a
# backtick or an escaped quote in a string the shell must not expand.
NPM_PLACEHOLDER_TEST='echo "Error: no test specified" && exit 1'

# Reads package.json directly. `npm run` writes its listing to stderr and prints
# nothing under --silent, so parsing its output silently matches nothing.
has_npm_script() {
  has_node &&
    command -v node >/dev/null 2>&1 &&
    command -v npm >/dev/null 2>&1 &&
    node -e 'const s=require("./package.json").scripts||{};const v=s[process.argv[1]];process.exit(v&&v!==process.argv[2]?0:1)' "$1" "$NPM_PLACEHOLDER_TEST" 2>/dev/null
}

# Every check runs from the repository root against a root workspace manifest.
# A package nested under apps/ or libs/ with no root manifest of its language is
# therefore invisible: nothing lints it, tests it, or audits its dependencies,
# and the run is green because no check ever looked.
#
# This catches "no root manifest at all", not "root manifest that omits this
# package". Reading membership out of npm workspaces, uv sources, or Cargo
# members needs a parser per tool, so a package left out of an existing root
# manifest is still missed.
#
# A nested manifest is not always named like the root one it belongs to: a Go
# module is go.mod under a root go.work, and a Gradle project is build.gradle.kts
# under a root settings.gradle.kts. Each pair is listed rather than assumed.
#
# One line per orphan on stdout, and exit 0 whether or not there were any:
# each line is a finding for the caller to record under its own verdict, and
# an empty list is that verdict's pass. No second channel carries the answer.
_detect_orphan_report() {
  local nested="${1#./}" root="$2" noun="$3" fix="$4"

  echo "$nested has no root $root, so no check sees this $noun; $fix, or move it under an existing workspace"
}

detect_orphan_manifests() {
  local lang root nested dir

  # language:nested manifest:root manifest:noun. Swift is absent deliberately:
  # it has no root manifest, so nested packages are how a Swift repository is
  # supposed to look and this rule must never fire on one.
  local -a pairs=(
    "node:package.json:package.json:package"
    "python:pyproject.toml:pyproject.toml:package"
    "rust:Cargo.toml:Cargo.toml:crate"
    "go:go.mod:go.work:module"
    "kotlin:build.gradle.kts:settings.gradle.kts:project"
  )

  local entry nested_name
  for entry in "${pairs[@]}"; do
    IFS=: read -r lang nested_name root noun <<<"$entry"

    # The root manifest itself, not language presence: has_go accepts go.mod
    # or go.work, and has_kotlin accepts build.gradle.kts or settings.gradle.kts,
    # so either would wrongly treat a root go.mod or build.gradle.kts alone as
    # having wired up a nested module that neither file actually includes.
    if [[ -s "$root" ]]; then continue; fi

    while IFS= read -r nested; do
      [[ -n "$nested" ]] || continue
      dir="$(dirname "${nested#./}")"

      # Where nested_name differs from root (go.mod under go.work,
      # build.gradle.kts under settings.gradle.kts), searching for nested_name
      # also matches a copy sitting at the repository root: that is this
      # project's own top-level manifest, already visible to every check
      # without a workspace file, not a nested module missing one.
      [[ "$dir" != "." ]] || continue

      case "$lang" in
        go) _detect_orphan_report "$nested" "$root" "$noun" \
          "run 'go work init ./$dir' at the repository root" ;;
        kotlin) _detect_orphan_report "$nested" "$root" "$noun" \
          "add a root $root with an include() entry covering $dir" ;;
        *) _detect_orphan_report "$nested" "$root" "$noun" \
          "add a root $root covering $dir" ;;
      esac
    done < <(detect_find "$nested_name" -print)
  done
}

# The orphans as a check: each one recorded as a finding, then the detection
# verdict. Every command that runs the capabilities calls this first, since an
# orphan is a package none of them can see.
judge_orphan_manifests() {
  local orphan
  while IFS= read -r orphan; do
    record "$orphan"
  done < <(detect_orphan_manifests)
  verdict detection "every package has a root manifest"
}

# The module's external interface is `language_capabilities` below. Everything
# after this point is its private per-language implementation. Callers name a
# capability; they do not know which command, manifest, or tool provides it.

_capability_lint_node() { has_npm_script lint || return "$NO_RUNNER"; npm run lint; }
_capability_lint_python() { uv_run ruff check .; }
_capability_lint_go() { go_each go vet ./...; }
_capability_lint_rust() { cargo clippy -- -D warnings; }
_capability_lint_swift() { command -v swift >/dev/null 2>&1 || return "$NO_RUNNER"; swift_each swift format lint --recursive --strict .; }
_capability_lint_kotlin() {
  if gradle_has_task ktlintCheck; then ./gradlew ktlintCheck; return; fi
  if gradle_has_task detekt; then ./gradlew detekt; return; fi
  return "$NO_RUNNER"
}

_capability_format_check_node() { has_npm_script format:check || return "$NO_RUNNER"; npm run format:check; }
_capability_format_check_python() { uv_run ruff format --check .; }
_capability_format_check_go() {
  local unformatted status=0
  unformatted="$(gofmt -l .)" || status=$?
  (( status == 0 )) || { echo "gofmt failed (exit $status)"; return "$status"; }
  [[ -z "$unformatted" ]] || { echo "gofmt needed: $unformatted"; return 1; }
}
_capability_format_check_rust() { cargo fmt --check; }
_capability_format_check_swift() { command -v swift >/dev/null 2>&1 || return "$NO_RUNNER"; swift_each swift format lint --recursive .; }
_capability_format_check_kotlin() { gradle_has_task ktlintCheck || return "$NO_RUNNER"; ./gradlew ktlintCheck; }

_capability_typecheck_node() { has_npm_script typecheck || return "$NO_RUNNER"; npm run typecheck; }
_capability_typecheck_python() { uv_run ty check .; }

_capability_test_node() { has_npm_script test || return "$NO_RUNNER"; npm run test; }
_capability_test_python() {
  local status=0
  uv_run pytest || status=$?
  if (( status == 5 )); then
    echo "pytest collected no tests."
    return 0
  fi
  return "$status"
}
_capability_test_go() { go_each go test ./...; }
_capability_test_rust() { cargo test; }
_capability_test_swift() { command -v swift >/dev/null 2>&1 || return "$NO_RUNNER"; swift_each swift test; }
_capability_test_kotlin() { [[ -x ./gradlew ]] || return "$NO_RUNNER"; ./gradlew test; }

_capability_build_node() { has_npm_script build || return "$NO_RUNNER"; npm run build; }
_capability_build_go() { go_each go build ./...; }
_capability_build_rust() { cargo build --locked; }
_capability_build_swift() { command -v swift >/dev/null 2>&1 || return "$NO_RUNNER"; swift_each swift build; }
_capability_build_kotlin() { [[ -x ./gradlew ]] || return "$NO_RUNNER"; ./gradlew build -x test; }

_capability_audit_node() {
  if [[ -s package-lock.json ]] && command -v npm >/dev/null 2>&1; then
    npm audit --audit-level=moderate
    return
  fi
  if [[ -s pnpm-lock.yaml ]] && command -v pnpm >/dev/null 2>&1; then
    pnpm audit --audit-level moderate
    return
  fi
  if [[ -s yarn.lock ]] && command -v yarn >/dev/null 2>&1; then
    yarn npm audit --severity moderate
    return
  fi
  return "$NO_RUNNER"
}
# `uv audit` is a uv subcommand rather than a tool uv runs, so uv_run does not
# apply: there is nothing to look up on PATH, and the version probe it does
# would run the audit itself. A uv predating the subcommand exits 2 for an
# unknown one, the same shape as an audit that ran and found vulnerabilities,
# so support is probed with --help first and its absence reported as no runner.
#
# The subcommand is still experimental and prints a warning on every run unless
# the preview feature is named. Passing it silences that; an older uv that
# rejects the flag has already been sent to NO_RUNNER by the probe above.
_capability_audit_python() {
  command -v uv >/dev/null 2>&1 || return "$NO_RUNNER"
  uv audit --help >/dev/null 2>&1 || return "$NO_RUNNER"
  uv audit --preview-features audit-command
}
_capability_audit_go() { command -v govulncheck >/dev/null 2>&1 || return "$NO_RUNNER"; go_each govulncheck ./...; }
_capability_audit_rust() { command -v cargo-audit >/dev/null 2>&1 || return "$NO_RUNNER"; cargo audit; }
_capability_audit_swift() { command -v trivy >/dev/null 2>&1 || return "$NO_RUNNER"; trivy_each Package.resolved; }
_capability_audit_kotlin() { command -v trivy >/dev/null 2>&1 || return "$NO_RUNNER"; trivy_each gradle.lockfile; }

_capability_toolchain_node() { command -v npm >/dev/null 2>&1 || return "$NO_RUNNER"; }
_capability_toolchain_python() { command -v uv >/dev/null 2>&1 || return "$NO_RUNNER"; }
_capability_toolchain_go() { command -v go >/dev/null 2>&1 || return "$NO_RUNNER"; }
_capability_toolchain_rust() { command -v cargo >/dev/null 2>&1 || return "$NO_RUNNER"; }
_capability_toolchain_swift() { command -v swift >/dev/null 2>&1 || return "$NO_RUNNER"; }
_capability_toolchain_kotlin() {
  command -v java >/dev/null 2>&1 || return "$NO_RUNNER"
  [[ -x ./gradlew ]] || return "$NO_RUNNER"
}

_capability_format_write_node() { has_npm_script format || return "$NO_RUNNER"; npm run format; }
_capability_format_write_python() { uv_run ruff format .; }
_capability_format_write_go() { gofmt -w .; }
_capability_format_write_rust() { cargo fmt; }
_capability_format_write_swift() { command -v swift >/dev/null 2>&1 || return "$NO_RUNNER"; swift_each swift format --in-place --recursive .; }
_capability_format_write_kotlin() { gradle_has_task ktlintFormat || return "$NO_RUNNER"; ./gradlew ktlintFormat; }

# scripts/run resolves the unit's declared run fact into $unit_run and the
# arguments after `--` into $unit_args; scripts/package resolves the declared
# targets into $unit_targets. An adapter takes no parameters -- every other one
# in this file reads the working directory alone -- so this context arrives the
# same way, already resolved by the caller.
#
# The run dispatch is unevenly load-bearing, and that is the trap. In Go, Rust
# and Kotlin the command is identical whichever value is declared, so anyone
# auditing those three concludes correctly that the branch changes nothing and
# can be deleted. It cannot: in Node and Python it chooses between the
# program's own entry point and a dev server, which are different programs.
# Delete the branch and a CLI starts a web server, or fails looking for one.
#
# All three are declared in libs/unit.sh, beside the reader that fills the
# declared ones, and nowhere here: a caller with no unit leaves them empty, and
# each adapter reads an empty value as the default -- the `${unit_run:-}` test
# falls to the long-lived branch, the `+` expansion reads an absent array as
# empty rather than tripping `set -u`. shellcheck reads each array's first use
# below as unassigned, and the directive there names the declaration's home.

# The paths `bin` names, one per line. A string names one and an object names
# several; either way the field is the package's own declaration of what it is
# when run, which is what [project.scripts] declares in Python.
npm_bin_entries() {
  has_node &&
    command -v node > /dev/null 2>&1 &&
    node -e 'const b = require("./package.json").bin || {};
for (const p of typeof b === "string" ? [b] : Object.values(b)) console.log(p);' 2> /dev/null
}

# A one-shot runs the entry point `bin` names, because naming one is how a
# package says it is a CLI. A one-shot that is not a CLI -- a script, a job --
# names none, and `npm start` is how it is started instead. A long-lived unit
# runs the dev server whether or not the package also ships a CLI.
_capability_run_node() {
  local path paths_out
  local -a paths=()

  if [[ "${unit_run:-}" != oneshot ]]; then
    has_npm_script dev || return "$NO_RUNNER"
    # shellcheck disable=SC2154 # declared in libs/unit.sh, filled by scripts/run
    npm run dev -- ${unit_args[@]+"${unit_args[@]}"}
    return
  fi

  paths_out="$(npm_bin_entries)" || return "$NO_RUNNER"

  while IFS= read -r path; do
    if [[ -n "$path" ]]; then paths+=("$path"); fi
  done <<< "$paths_out"

  case "${#paths[@]}" in
    0) has_npm_script start || return "$NO_RUNNER"; npm start -- ${unit_args[@]+"${unit_args[@]}"} ;;
    1)
      # A TypeScript bin conventionally names build output, which a fresh clone
      # does not have. Node's own failure for that is a module-not-found trace
      # naming a file rather than the build that was never run, and the unit is
      # well-formed and the command correct, so the absence is caught here and
      # what to do is named instead. The condition is a bin that is not on disk
      # rather than anything about TypeScript, and a plain JavaScript unit --
      # whose bin is a source file -- never reaches it.
      if [[ ! -f "${paths[0]}" ]]; then
        echo "package.json names bin ${paths[0]}, and there is no such file here." >&2
        if has_npm_script build; then
          echo "In TypeScript that path is build output rather than source. Build it first:" >&2
          echo "  npm run build" >&2
        elif command -v npm > /dev/null 2>&1; then
          # Only with npm present is the absence of a build script a fact about
          # the package. Without it has_npm_script is false for the tool rather
          # than for package.json, and the line above is all this knows.
          echo "This package names no build script either, so nothing here produces it." >&2
        fi
        return 1
      fi
      node "${paths[0]}" ${unit_args[@]+"${unit_args[@]}"}
      ;;
    *)
      echo "Several bin entries here, and a run is one foreground process." >&2
      echo "Start the one meant by path:" >&2
      printf '  node %s\n' "${paths[@]}" >&2
      return 1
      ;;
  esac
}
# A Python service has no conventional start command -- uvicorn, gunicorn,
# manage.py runserver and a bare module are all ordinary -- so the long-lived
# form has nothing to dispatch to rather than a wrong guess to make. A one-shot
# does: [project.scripts] is where a Python CLI names its entry point, and it is
# read with the interpreter the project already requires rather than by a shell
# pattern over TOML.
_capability_run_python() {
  local name names_out
  local -a names=()

  [[ "${unit_run:-}" == oneshot ]] || return "$NO_RUNNER"
  command -v uv > /dev/null 2>&1 || return "$NO_RUNNER"

  names_out="$(uv run --quiet python -c 'import pathlib, tomllib
project = tomllib.loads(pathlib.Path("pyproject.toml").read_text()).get("project") or {}
print("\n".join(project.get("scripts") or {}))' 2> /dev/null)" || return "$NO_RUNNER"

  while IFS= read -r name; do
    if [[ -n "$name" ]]; then names+=("$name"); fi
  done <<< "$names_out"

  case "${#names[@]}" in
    0) return "$NO_RUNNER" ;;
    1) uv run "${names[0]}" ${unit_args[@]+"${unit_args[@]}"} ;;
    *)
      echo "Several console scripts here, and a run is one foreground process." >&2
      echo "Start the one meant by name:" >&2
      printf '  uv run %s\n' "${names[@]}" >&2
      return 1
      ;;
  esac
}
# `go run .` and `go build .` need a main package in the directory they are
# called from, which neither the root of a workspace nor an app whose entry
# point sits under src/ has -- both reported `no Go files`, and no arrangement
# of the code cleared it. Ask the modules which packages are main instead.
#
# Fills the array named by $1. A listing that fails is a failure rather than an
# empty result -- it means the module graph could not be read at all -- and a
# command substitution inside mapfile would have swallowed that status.
_go_main_packages() {
  local -n _mains_out="$1"
  local package packages
  packages="$(go_each go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./...)" || return 1
  _mains_out=()
  # An `if` rather than a `&&`: this loop is the function's last statement, and
  # a `&&` whose test fails on the trailing empty line would make an empty
  # listing return 1 -- a failure where the answer is "no main package here".
  while IFS= read -r package; do
    if [[ -n "$package" ]]; then _mains_out+=("$package"); fi
  done <<< "$packages"
}
# One main package is the program; several is the ambiguity scripts/run already
# refuses for stacks, named rather than guessed at; none means there is nothing
# here to start, which is no runner rather than a failure -- a library is not
# broken for being a library.
_capability_run_go() {
  local -a mains=()
  _go_main_packages mains || return 1

  case "${#mains[@]}" in
    0) return "$NO_RUNNER" ;;
    1) go run "${mains[0]}" ${unit_args[@]+"${unit_args[@]}"} ;;
    *)
      echo "Several main packages here, and a run is one foreground process." >&2
      echo "Start the one meant by name:" >&2
      printf '  go run %s\n' "${mains[@]}" >&2
      return 1
      ;;
  esac
}
_capability_run_rust() { cargo run -- ${unit_args[@]+"${unit_args[@]}"}; }
# Gradle takes program arguments as one string rather than a list, so this is
# the one adapter that cannot pass them through unchanged: an argument
# containing a space arrives as two.
_capability_run_kotlin() {
  gradle_has_task run || return "$NO_RUNNER"
  ./gradlew run ${unit_args[@]+--args="${unit_args[*]}"}
}

# Targets are the template's own vocabulary, so a declaration reads the same
# whatever the unit is written in and the translation is the adapter's
# business. A target this adapter has no translation for is not a failure: the
# vocabulary may outgrow one language before another.
_package_go_target() {
  case "$1" in
    linux-amd64) echo "linux amd64" ;;
    macos-arm64) echo "darwin arm64" ;;
    *) return 1 ;;
  esac
}
_package_rust_target() {
  case "$1" in
    linux-amd64) echo x86_64-unknown-linux-gnu ;;
    macos-arm64) echo aarch64-apple-darwin ;;
    *) return 1 ;;
  esac
}

# A result line per target, because a single per-language result cannot say
# that one target built and another could not be reached from here. A target
# the toolchain genuinely cannot reach is not-applicable with the reason named;
# an unwired language is unavailable. They look alike and are different facts,
# and reporting the first as the second makes an impossible build read as a
# broken install.
_capability_package_go() {
  local target pair goos goarch package status=0
  local -a mains=()

  command -v go > /dev/null 2>&1 || return "$NO_RUNNER"
  _go_main_packages mains || return 1
  if (( ${#mains[@]} == 0 )); then
    echo "No main package here, so there is no executable to build." >&2
    return "$NO_RUNNER"
  fi

  mkdir -p dist
  # shellcheck disable=SC2154 # declared and filled in libs/unit.sh
  for target in ${unit_targets[@]+"${unit_targets[@]}"}; do
    if ! pair="$(_package_go_target "$target")"; then
      result "$target" not-applicable "no Go GOOS/GOARCH is named for it"
      continue
    fi
    read -r goos goarch <<< "$pair"
    for package in "${mains[@]}"; do
      # A packaged binary is one that leaves this machine: `release` uploads
      # what lands in `dist/`. Without CGO_ENABLED=0 a cross target builds
      # static anyway, for want of a cross C toolchain, and the host target
      # does not -- so the one build that reaches a release is the one that
      # fails wherever libc differs. -trimpath keeps the runner's own paths
      # out of a published artifact.
      CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
        go build -trimpath -ldflags='-s -w' -o "dist/$(basename "$package")-$target" "$package" || status=1
    done
  done
  return "$status"
}

# cargo names the binaries; reading them out of the metadata beats guessing the
# crate name, which is not always the binary's.
_cargo_bin_names() {
  cargo metadata --no-deps --format-version 1 2> /dev/null |
    jq -r '.packages[].targets[] | select(.kind[] == "bin") | .name'
}
_capability_package_rust() {
  local target triple name status=0

  command -v cargo > /dev/null 2>&1 || return "$NO_RUNNER"
  # Both absences are no runner, but they read differently to whoever is
  # holding the failure: cargo missing is the language not installed, and jq
  # missing is a Rust unit that would package if one more tool were here. The
  # library reports either as unavailable, so this one says what is actually
  # missing before it goes quiet.
  if ! command -v jq > /dev/null 2>&1; then
    echo "cargo is here but jq is not, and the binary names are read out of cargo metadata." >&2
    return "$NO_RUNNER"
  fi

  mkdir -p dist
  for target in ${unit_targets[@]+"${unit_targets[@]}"}; do
    if ! triple="$(_package_rust_target "$target")"; then
      result "$target" not-applicable "no Rust target triple is named for it"
      continue
    fi
    # rustup is how a cross target is installed, so its list is the honest
    # answer to whether this host can reach the target at all. Without rustup
    # there is no list to consult and cargo decides, which is a real failure
    # rather than a skip.
    if command -v rustup > /dev/null 2>&1 &&
      ! grep -qx "$triple" <<< "$(rustup target list --installed 2> /dev/null)"; then
      result "$target" not-applicable "the Rust target $triple is not installed (rustup target add $triple)"
      continue
    fi

    if ! cargo build --release --locked --target "$triple"; then
      status=1
      continue
    fi
    while IFS= read -r name; do
      [[ -n "$name" && -f "target/$triple/release/$name" ]] || continue
      cp "target/$triple/release/$name" "dist/$name-$target"
    done < <(_cargo_bin_names)
  done
  return "$status"
}

# No packaging adapter for Node, Python, Swift, or Kotlin, and no run adapter
# for Swift, and each gap is a signpost rather than a mystery: a Node or Python
# executable means bundling an interpreter, and a Swift or Kotlin one means a
# toolchain decision this template has not made. The dispatch reports each as
# `unavailable` and `language_capabilities probe` reports it `absent`, so the
# gap is visible without a function standing in for the adapter that is not
# there.

# Whether any of the languages named packages into the unit's dist/. Only the
# adapters above that produce a file write there; a language with no adapter
# writes nothing, so for a unit in one of those languages dist/ is not the packaging
# command's output but whatever the unit's own build left -- which is what
# scripts/ci produces before the release walk reads it. scripts/package empties
# dist/ before dispatching, and asks this first so it empties only a directory
# it is about to refill. Kept beside the adapters so the two move together.
packaging_writes_dist() {
  local lang
  for lang in "$@"; do
    case "$lang" in
      go|rust) return 0 ;;
    esac
  done
  return 1
}

_capability_is_not_applicable() {
  case "$1:$2" in
    typecheck:go|typecheck:rust|typecheck:swift|typecheck:kotlin|build:python) return 0 ;;
    # Package.resolved is committed by SwiftPM whenever a package has
    # dependencies; gradle.lockfile exists only once dependency locking is
    # turned on, which is not the Gradle default. Neither is guaranteed, so a
    # trivy target absent here means nothing to scan, not a missing tool.
    audit:swift) [[ -z "$(detect_find Package.resolved -print -quit)" ]] ;;
    audit:kotlin) [[ -z "$(detect_find gradle.lockfile -print -quit)" ]] ;;
    *) return 1 ;;
  esac
}

DETECT_CAPABILITIES=(lint format-check typecheck test build audit toolchain format-write run package)

# _in_list <word> <items…>: whether the word is one of the items.
_in_list() {
  local word="$1" item
  shift
  for item in "$@"; do
    [[ "$word" == "$item" ]] && return 0
  done
  return 1
}

_language_capabilities_run() {
  local dry_run="" selected="" cap lang function status
  local unavailable=0
  local -a capabilities=() languages=()

  while (( $# > 0 )); do
    case "$1" in
      --dry-run) dry_run=1; shift ;;
      --language)
        (( $# >= 2 )) || { echo "--language needs a value" >&2; return 2; }
        selected="$2"
        shift 2
        ;;
      --) shift; break ;;
      *) break ;;
    esac
  done

  (( $# > 0 )) || { echo "run needs at least one capability" >&2; return 2; }
  capabilities=("$@")

  for cap in "${capabilities[@]}"; do
    _in_list "$cap" "${DETECT_CAPABILITIES[@]}" || { echo "Unknown capability: $cap" >&2; return 2; }
  done

  if [[ -n "$selected" ]]; then
    _in_list "$selected" "${DETECT_LANGUAGES[@]}" || { echo "Unsupported language: $selected" >&2; return 2; }
    has_language "$selected" || { echo "Language is not present: $selected" >&2; return 2; }
    languages=("$selected")
  else
    mapfile -t languages < <(detect_present_languages)
  fi

  for cap in "${capabilities[@]}"; do
    for lang in "${languages[@]}"; do
      if _capability_is_not_applicable "$cap" "$lang"; then
        result "$cap" not-applicable "$lang"
        continue
      fi

      function="_capability_${cap//-/_}_${lang}"
      if ! declare -F "$function" >/dev/null; then
        result "$cap" unavailable "$lang"
        unavailable=1
        continue
      fi

      if [[ -n "$dry_run" ]]; then
        result "$cap" would-run "$lang"
        continue
      fi

      # A check never reads stdin, and a check that inherits one holds the whole
      # gate open: any tool that drains fd 0 -- swift test, gradle, npm test --
      # blocks until the caller's stdin reaches EOF, which for a terminal or an
      # agent harness is never, with no output to diagnose from. `run` is the
      # one capability that starts the unit's own program in the foreground, so
      # it is the one that keeps the stdin it was given.
      status=0
      if [[ "$cap" == run ]]; then
        "$function" || status=$?
      else
        "$function" </dev/null || status=$?
      fi
      case "$status" in
        0) result "$cap" pass "$lang" ;;
        "$NO_RUNNER")
          result "$cap" unavailable "$lang"
          unavailable=1
          ;;
        *) result "$cap" FAIL "$lang (exit $status)" ;;
      esac
    done
  done

  # Said once, here, rather than decoded from the return value by each caller.
  # The outcome is not returned at all: every line above went through result,
  # so the tally holds it, and a second channel would be one more thing to keep
  # agreeing with the first. Only a usage error returns non-zero.
  if (( unavailable )); then
    echo "Install the tool behind each unavailable line, or add its private adapter to scripts/libs/detect.sh."
  fi
}

_codeql_entry() {
  case "$1" in
    node) echo 'javascript-typescript none ubuntu-latest' ;;
    python) echo 'python none ubuntu-latest' ;;
    go) echo 'go autobuild ubuntu-latest' ;;
    rust) echo 'rust none ubuntu-latest' ;;
    kotlin) echo 'java-kotlin autobuild ubuntu-latest' ;;
    swift) echo 'swift autobuild macos-latest' ;;
    *) return 1 ;;
  esac
}

_language_capabilities_github_output() {
  local lang
  for lang in "${DETECT_LANGUAGES[@]}"; do
    if has_language "$lang"; then
      echo "$lang=true"
    else
      echo "$lang=false"
    fi
  done

  if has_trivy_target; then echo "trivy=true"; else echo "trivy=false"; fi
}

_language_capabilities_codeql_matrix() {
  local lang entry language build_mode runner
  local -a entries=()

  while IFS= read -r lang; do
    [[ -n "$lang" ]] || continue
    entry="$(_codeql_entry "$lang")" || continue
    read -r language build_mode runner <<<"$entry"
    entries+=("{\"language\":\"$language\",\"build-mode\":\"$build_mode\",\"runner\":\"$runner\"}")
  done < <(detect_present_languages)

  entries+=('{"language":"actions","build-mode":"none","runner":"ubuntu-latest"}')
  printf 'matrix={"include":[%s]}\n' "$(IFS=,; echo "${entries[*]}")"
}

# One line per capability and language: `wired` when an adapter is defined for
# the pair, `absent` when none is. Wiring alone -- no manifest is read and no
# tool is looked for -- so the answer is the same on every machine, and a suite
# can hold the whole table against the one this file is meant to have without
# naming a private function.
_language_capabilities_probe() {
  local cap lang state
  for cap in "${DETECT_CAPABILITIES[@]}"; do
    for lang in "${DETECT_LANGUAGES[@]}"; do
      state=absent
      ! declare -F "_capability_${cap//-/_}_${lang}" >/dev/null || state=wired
      result_line "$cap" "$state" "$lang"
    done
  done
}

language_capabilities() {
  local command="${1:-}"
  (( $# == 0 )) || shift

  case "$command" in
    supported) printf '%s\n' "${DETECT_LANGUAGES[@]}" ;;
    present) detect_present_languages ;;
    has-any) has_any_manifest ;;
    github-output) _language_capabilities_github_output ;;
    codeql-matrix) _language_capabilities_codeql_matrix ;;
    probe) _language_capabilities_probe ;;
    run) _language_capabilities_run "$@" ;;
    ""|-h|--help|help)
      echo "Usage: language_capabilities supported|present|has-any|github-output|codeql-matrix|probe|run"
      ;;
    *) echo "Unknown language capabilities command: $command" >&2; return 2 ;;
  esac
}
