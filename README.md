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
> **There is no release yet.** Everything runs from a checkout, and [Install](#install) builds and places the two artifacts by hand. `docs/plans/browser-client.md` is the order the work landed in.

## Shape

Two deployables. `apps/todo` is a Go binary with three modes:

| Invocation | What it does |
| --- | :--- |
| `todo` | prints usage and exits |
| `todo <verb>` | acts and exits — the agent path, the same in-process call a request makes |
| `todo api` | serves the JSON and the browser client so a phone reaches it, LAN only |

`apps/web` is the second: a TypeScript browser client, built to static files `todo api` serves beside the JSON. The store is SQLite embedded as a library, so it is not a separate process and not a deployable of its own. Three bounded contexts — Tracking, Scheduling, and Change History — collapse into that one binary; `CONTEXT.md` defines the language of each, and `docs/adrs/0001-ship-todo-as-one-go-binary.md` records why one artifact rather than several, and why Go.

The decisions behind all of it were worked out as a map of tickets on this repository's own issue tracker, [#1](https://github.com/possiblyneal/todo/issues/1) through [#8](https://github.com/possiblyneal/todo/issues/8), and the operator's original wish list is kept verbatim at `docs/features.md`.

## Install

Nothing is released, so the two artifacts are built from a checkout and placed by hand. Build them:

```sh
scripts/package todo   # apps/todo/dist/todo-linux-amd64, todo-macos-arm64
npm ci && npm run build # apps/web/dist/
```

Place them, and start the service that serves both:

```sh
install -D -m755 apps/todo/dist/todo-linux-amd64 ~/.local/bin/todo
rm -rf ~/.local/share/todo/web && mkdir -p ~/.local/share/todo/web
cp -a apps/web/dist/. ~/.local/share/todo/web/

install -D -m644 apps/todo/deploy/systemd/todo-api.service ~/.config/systemd/user/todo-api.service
systemctl --user daemon-reload
systemctl --user enable --now todo-api.service
```

The client is then at `http://<host>:8080`, and `todo <verb>` at the terminal reaches the same store: the unit names no `TODO_DB` and no `TODO_ACTOR`, so the service and a verb both resolve `~/.config/todo/todo.db` and the login name.

`:8080` is a wildcard bind, so the listener is reachable on every interface the host has; ADR 0003 puts it on the LAN with no authentication, and the network is what keeps it there. To narrow it, override `ExecStart` in a drop-in rather than editing the shipped unit — and clear it with a bare `ExecStart=` first. Without that line the drop-in appends, and a `Type=simple` unit with two `ExecStart=` settings is refused at load: `daemon-reload` reports a bad unit file setting and the service does not start at all.

```sh
systemctl --user edit todo-api.service
```
```ini
[Service]
ExecStart=
ExecStart=%h/.local/bin/todo api -addr 10.0.0.2:8080 -web %h/.local/share/todo/web
```

A specific address has to exist before the bind, and a user service has no `network-online.target` to wait for, so it leans on `Restart=on-failure` to catch a boot that got there first. That budget is finite: five tries five seconds apart, so an interface later than about twenty-five seconds leaves the unit failed rather than retrying.

`loginctl enable-linger $USER` is what makes a user service start at boot without a login. To run a build without installing anything, `scripts/run todo` starts one in the foreground.

## Contributing

This repository has no contribution guide yet. Open an [issue](https://github.com/possiblyneal/todo/issues) to raise something.

Work reaches `main` through a pull request. `scripts/check` runs the same gate CI does and is what to run before pushing; `CLAUDE.md` and `scripts/CLAUDE.md` describe the rest.

## License

Released under the [GPL-3.0-or-later](LICENSE) license.

## Acknowledgements

- [SQLite](https://sqlite.org/) — the store, embedded rather than deployed

Built by [possiblyneal](https://github.com/possiblyneal).
