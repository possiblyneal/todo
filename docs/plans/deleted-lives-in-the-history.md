# Plan: a deleted Task lives in the Change History

Carries out issue #85. A deleted Task stops appearing in any list read, and the
Change History becomes the place it is found — which only works if an entry can
show the Task it is about, and if the log can be searched past its first page.

## Why not a filter or a List

"Show everything" takes in the snoozed, the completed, the declined and the
deleted together, so a Task that should not have been there sits beside three
kinds of Task that were meant to be. `CONTEXT.md` already says a deletion is the
one thing there is no way back from; putting it in a view of the ended says the
opposite.

Offering **Deleted** as an entry in the List picker was considered and dropped.
`CONTEXT.md` defines a List as where the work sits and lets a Task belong to
more than one, so a deleted Task would still sit under its real Lists as well —
the picker would either double-file it or lose where it sat.

## Three parts

### 1. Deleted leaves every list read

`api/state.go:118-123` is the one place `all=true` fans out to four flags, and
it stops setting `IncludeDeleted`. `Query.IncludeDeleted` stays on the store:
`schedule.go:386` reads the whole tracker and still needs it.

The CLI's `-all` goes the same way, since a verb and a request make the same
call and the two surfaces must not disagree about what "everything" takes in.

### 2. Any entry opens the Task as it stood

Not only a deletion. Any entry in either log, so "what did this say before that
edit" has an answer, and a deletion is merely the case where there is no other
way to ask.

`apps/todo/CLAUDE.md` rules out replaying payloads in Go: the reopen cascade
leaves no entry per reopened ancestor, so the ancestors' state comes back "only
by re-running the same rule". Replaying in Go would be a second path through the
rules, which the same file forbids for writes.

So the replay goes through the rules that already exist. Entries with
`seq <= N` are inserted into a fresh in-memory database carrying the same
schema, the folds fire as they did the first time, and the Task is read out of
the replica by the ordinary read path. One rule set, and a kind added to the
store replays correctly the day it is appended without this code being touched.

The cost is a replay per view, proportional to the log. That is a personal
tracker's log and the view is opened deliberately rather than polled; if it ever
stops being cheap, the answer is a snapshot and not a second rule set.

### 3. The Change History is searchable, server-side

`apps/web/CLAUDE.md` has the activity screen reaching further back by asking for
more rather than stitching pages, so a filter applied client-side over the
entries already fetched could not find a Task deleted a month ago. The search is
a parameter on the route and the store is what matches, which is the rule the
list's own search already follows.

It matches an entry's payload and its subject, so the words somebody remembers
about a Task find the entries about it.

## Order

1. Store: the replay, and search on the history reads.
2. API: the `all` change, the as-of route, the search parameter.
3. CLI: `-all` stops taking in the deleted.
4. Client: the search box on the activity screen, and an entry that opens the
   Task as it stood.
5. Docs: `CONTEXT.md`'s **Deleted** term, and both child `CLAUDE.md` files.
