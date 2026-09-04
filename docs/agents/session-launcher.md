# Session launcher: Orca

How a skill hands the next unit of work to a **fresh agent session in its own tab**, on this
machine. Used by `/wayfinder` ("Hand off the next ticket"). Orca-specific; a machine without
Orca has no launcher and the calling skill reports the handoff by hand instead.

## Resolve the CLI

Never run bare `orca` on Linux outside an Orca-managed terminal — it resolves to
`/usr/bin/orca`, the GNOME Orca screen reader, and starts speech on the user's machine.

- `ORCA_CLI_COMMAND` set → use its value.
- Else `ORCA_DEV_REPO_ROOT` set → `orca-dev`.
- Else on Linux outside an Orca terminal → **`orca-ide`**.

`ORCA` below is that resolved executable. Confirm the app is up with `ORCA status --json`
before anything else.

## Launch sequence

```bash
ORCA terminal create --worktree path:<repo> --title "<owner/repo issue #N>" --focus --json
ORCA terminal focus  --terminal <handle> --json
ORCA terminal send   --terminal <handle> --text $'\x15'
ORCA terminal send   --terminal <handle> --text "orca-ide claude-teams '--dangerously-skip-permissions'" --enter
ORCA terminal wait   --terminal <handle> --for tui-idle --timeout-ms 120000
ORCA terminal send   --terminal <handle> --text $'\x15'
ORCA terminal send   --terminal <handle> --text "<the prompt>" --enter
```

The agent invocation on line 4 is **this repo's recorded choice**, including the permission
flag. Change it here — it is deliberately not written into any skill.

## Measured gotchas

Each of these cost a failed attempt.

- **`claude-teams` is not on `PATH`.** It is an `orca-ide` subcommand. Running it bare gives
  `bash: claude-teams: command not found`, and `terminal create --command "claude-teams"`
  fails the same way. Create the terminal bare, then send the full `orca-ide claude-teams …`
  line.
- **Send text and `--enter` in one call.** Splitting them drops the text silently: the
  terminal reads back an empty prompt and the work never starts.
- **`\x15` (Ctrl+U) before every send.** A failed prior line stays on the prompt and the next
  send concatenates onto it (`claude-teamsorca-ide: command not found`).
- **`--focus` on `create` can still return `surface: "background"`** with a warning that the
  terminal could not be made discoverable. A separate `terminal focus --terminal <handle>`
  fixes it.
- **`wait --for tui-idle` times out against a bare shell** and only succeeds once the agent
  TUI is up. Treat the first timeout as "not launched yet" and read the terminal to diagnose,
  never as failure to retry blindly.

Verify the prompt landed by reading the terminal before reporting: the prompt line echoed
back plus a running indicator.

## Report, don't abandon

The launcher hands work to a **human who is present**. After submitting, tell the user the
tab title, the terminal handle, and what that session will ask them for. One successor per
session, no deeper chain.
