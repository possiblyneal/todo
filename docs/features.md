# Features — as given by the operator

Source material, recorded verbatim as the operator wrote it. This is a wish list, not a
specification and not a commitment. It is kept here because the wayfinding map cites it and
because a document pasted into a chat window is a document that is gone.

Where a line below contradicts a decision recorded on the map, the map's ticket is the
authority and names the contradiction explicitly.

---

~~Complies with GHM and Commonmark~~ **— withdrawn by the operator, 2026-09-03: markdown is not the store.** The `yaml/json` alternative in the original line was decided against at issue #5: the store is SQLite, embedded as a library, zero deployables.
TUI only **— confirmed and extended, 2026-09-04 (issue #6): still one surface, now reached two ways.** The phone gets the same TUI over SSH — `todo serve`, the same binary, via `charmbracelet/wish`. LAN only. A *web* interface was raised and rejected in favour of SSH, which is what keeps a UI constraint from binding and TypeScript out of the repository.
Supports different lists
~~Cleanup tasks older than 30 days or duration of choosing (completed only or completed and uncompleted)~~ **— withdrawn by the operator, 2026-09-03. Not a feature.**
Sort lists by deadline or title
tasks can be declined or deferred
reoccuring tasks google calendar has great options for this
slash commands with dropdown
local ai - breakdown large tasks into smaller todos, query list,

Task attributes:
Title
description
why
creation date
deadline
subtask support for 5 levels of nesting
attachments
websites
filelocations
#Tags
lists
time estimate
priority (low, med ,high) w/ examples for each cat.
impact (low, med, high) w/ examples for each cat.
snooze hide a task for some amount of time (with defauts 1 hr, 1 day, 1 week, 1 month)
colors (task, list, tags)
easy nerd font/ emoji use slash command?
any number of key/value pairs

Adding a task:
click add task button
text boxes/dropdowns for all attributes
could optionally add new tag or list on the add a task screen, would be a popup
date pick for dates.... possible?
attachments bring up a system file selector box
AI box that takes tasks, gets any additional needed info from the user and breaks that task
into atomic tasks. then assigns attributes to each, with user approval gating along the way.

Main page has dropdown to select list and a searchbox, with task list below it. Tasks show
title and two lines of description, cration date, due date, list. paperclip icon to denote if
it has attachements. Clicking on a task expands it and you see all attributes and attachments.
You may order by alphabetical, due date, creation date, time estimate. there is a vertical
split with all the tags, ranked by frequency with a bit of variation thrown in so the list
doesn't look the same every time and allows for discovery. you may click on one or more to
filter the task list.

---

## Added after the original list

Recorded the same way as the list above — the operator's own words, dated, not paraphrased.

**2026-09-04, during issue #6:**

> I realized that I need this to be phone accessable, so it needs a web interface

Answered by SSH rather than a web interface, at the operator's choice once the two were put side by side: the phone runs an off-the-shelf SSH client against `todo serve` and gets the identical TUI, so there is no second surface to build and no second toolchain. Reach is **LAN only**; Tailscale is backlogged to the separate `./homelab` effort. The phone **reads and writes**.
