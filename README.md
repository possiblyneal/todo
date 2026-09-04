<div align="center">

# todo

_A task tracker for a person at a keyboard and for agents working while nobody is watching._

[![Build](https://img.shields.io/github/actions/workflow/status/possiblyneal/todo/ci.yml?branch=main)](https://github.com/possiblyneal/todo/actions)
[![License](https://img.shields.io/github/license/possiblyneal/todo)](LICENSE)

[About](#about) &bull; [Shape](#shape) &bull; [Install](#install) &bull; [Contributing](#contributing) &bull; [License](#license) &bull; [Acknowledgements](#acknowledgements)

</div>

---

## About

**todo** is a task tracker with two kinds of consumer: a person at a keyboard, and agents that add, edit, and delete while nobody is watching. Both reach the same store through the same calls, so an agent gets no weaker and no stronger a contract than a person does.

> [!NOTE]
> The interesting constraint is not the features. It is that two very different actors write concurrently and neither can tell which kind holds the claim, so a lease covers a whole task tree rather than a single task. `docs/adrs/0002-subtask-tree-is-one-aggregate.md` states the cost of that in plain words.

> [!IMPORTANT]
> **Nothing is built yet.** This repository currently holds the decisions and the scaffolding, not the program. The sections below describe what has been settled, not what runs.

## Shape

One deployable, `apps/todo`, written in Go, with three modes:

| Invocation | What it does |
| --- | :--- |
| `todo` | opens the TUI |
| `todo <verb>` | acts and exits — the agent path, the same in-process call the TUI makes |
| `todo serve` | serves the same TUI over SSH so a phone reaches it, LAN only |

The store is SQLite embedded as a library, so it is not a separate process and not a separate deployable. Three bounded contexts — Tracking, Scheduling, and Change History — collapse into that one binary; `CONTEXT.md` defines the language of each, and `docs/adrs/0001-ship-todo-as-one-go-binary.md` records why one artifact rather than several, and why Go.

The decisions behind all of it were worked out as a map of tickets on this repository's own issue tracker, [#1](https://github.com/possiblyneal/todo/issues/1) through [#8](https://github.com/possiblyneal/todo/issues/8), and the operator's original wish list is kept verbatim at `docs/features.md`.

## Install

There is nothing to install yet. Install and run instructions arrive with the first release; until then the repository builds nothing.

## Contributing

This repository has no contribution guide yet. Open an [issue](https://github.com/possiblyneal/todo/issues) to raise something.

Work reaches `main` through a pull request. `scripts/check` runs the same gate CI does and is what to run before pushing; `CLAUDE.md` and `scripts/CLAUDE.md` describe the rest.

## License

Released under the [GPL-3.0-or-later](LICENSE) license.

## Acknowledgements

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Wish](https://github.com/charmbracelet/wish) — the TUI and the SSH surface the language choice was reasoned from
- [SQLite](https://sqlite.org/) — the store, embedded rather than deployed

Built by [possiblyneal](https://github.com/possiblyneal).
