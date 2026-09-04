---
type: Architecture Decision Record
title: A Subtask Tree Is One Aggregate, Leased Whole
description: A top-level Task and every Subtask nested beneath it form a single aggregate with a single tree-scoped Lease, so that "a parent cannot complete with open children" becomes an invariant the store enforces rather than a convention callers keep.
scope: [global]
tags: [domain-model, aggregate-boundary, concurrency, invariants]
generated: { by: "agent/claude-opus-5", at: "2026-09-04T20:22:14Z" }
superseded_by:
status: accepted
---

# A Subtask Tree Is One Aggregate, Leased Whole

## Decision

A top-level Task and everything nested under it, to five levels, is one aggregate.
It is written as one whole and it is leased as one whole: a single tree-scoped Lease
covers every node in it, so an actor editing one Subtask five levels down holds the
claim to the entire tree for the life of that Lease.

This covers the aggregate boundary and the granularity of the Lease. It does not
cover how the Lease is represented, how long it lives, or how it is renewed. It does
not cover which deployable holds the code, and it would survive unchanged if the
single binary were ever split, which is why its scope is `global` rather than an app.

## Context

The tracker has two kinds of actor writing concurrently: a person at a keyboard and
AFK agents that add, edit, and delete while nobody is watching. Both hold the same
symmetric Lease before writing, and neither can tell which kind holds one.

Among the invariants the domain states, one carries the decision: **a parent cannot
complete with open children.** Checking it requires reading the parent and every
descendant, and keeping it true requires that nothing changes underneath that read
between the check and the write. An aggregate is the unit within which invariants
hold, so the invariant's span is what fixes the boundary: it spans the whole tree,
so the aggregate is the whole tree.

The second fact that makes the coarse boundary affordable is that **a Subtask is
fixed where it was created.** It never moves to a different parent and never leaves
the Task it sits under. Re-parenting is the operation a coarse boundary would make
expensive or impossible, because it is the one that spans two trees, and the domain
does not have it.

Established by reasoning against the domain model, not by measurement. Nothing has
been built and no contention has been observed. The trade-off below is argued from
the invariant, and the first real workload is what would test it.

## Alternatives Considered

**Every Task its own aggregate, with nesting held as a link, and fine-grained
per-Task Leases.** This was the recommended shape and it was rejected. It maximises
concurrency: two actors editing two Subtasks under the same parent never contend.
The cost is that *a parent cannot complete with open children* stops being
enforceable and becomes a convention. The check spans aggregates, so nothing holds
the descendants still while it runs, and an agent completing a parent while another
agent adds a child to it produces a state the domain says cannot exist. Rejected
because the invariant is the point; an invariant that callers are asked to respect
is a comment.

This becomes the right answer if the invariant is ever dropped, or if re-parenting
is ever added, since a Subtask that can move is a Subtask whose tree is not a stable
unit.

**Per-node Leases inside one aggregate.** Offered as the middle path: keep the
aggregate whole so the invariant still has a span, but claim only the node being
edited. Rejected because it pays the enforcement cost without getting the
enforcement. A write to a leaf can invalidate the parent's completion state, so a
lease that does not cover the parent does not protect what the aggregate boundary
was drawn to protect, and the machinery of tracking many claims per tree is spent
for a guarantee that is not delivered.

## Consequences

**The cost, stated plainly, because it is the reason this record exists.** An actor
holding a Lease on any node blocks every other write anywhere in that tree, for the
full life of the Lease. An agent appending one Subtask five levels down blocks the
person from editing the top-level Task's title. A reader who meets this without
context will assume it is a bug, and it is not; it is the price of the invariant.

The blast radius is bounded by tree size rather than by tracker size, so contention
is a property of how deep and how wide a single top-level Task grows. A tracker of
many small trees barely notices. One enormous tree that several agents work at once
is the shape that hurts, and it is the shape to watch for.

Reads are unaffected. Overdue, Lease expiry, and a due Occurrence are all conditions
computed when something looks, and reads never write, so nothing needs a Lease to
look at a tree someone else is holding.

**Subtasks never move**, which is what makes this affordable and is now a constraint this
boundary depends on. Adding re-parenting later is not a feature addition; it is a
cross-aggregate operation this boundary does not have, and it re-opens this ADR.
