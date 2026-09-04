---
type: Lessons Learned
title: Repository Lessons Learned
description: Repository-specific knowledge that prevents recurring mistakes or expensive rediscovery.
tags: []
generated: { by: "agent/claude-opus-5", at: "2026-09-04T20:22:14Z" } # update after each meaningful change
# verified: { by: "<actor>", at: "<ISO 8601 datetime>" } # uncomment after confirming this document against its cited sources
status: draft                 # draft | stable | deprecated
---
# Lessons learned

## How to add a lesson

Add only durable, repeatable, non-obvious constraints, conventions, or tool/provider quirks. Do not add one-off incident history, generic advice, or facts already clear from the code. Write concisely and concretely. Lead with the required action; name the exact artifact, command, or constraint; remove background and filler words. State why only when the consequence is not self-evident. Link to the authoritative source rather than duplicating it.

### Lesson format

```md
## <Short, imperative title>

<The non-obvious rule or constraint.>

**Do:** <The specific required action.>

**Why:** <The consequence avoided, if it is not self-evident.>

**Source:** [<Authoritative source title>](<URL or relative path>)
```

### Example

```md
## Put a language's manifest at the repository root

A package under `apps/` or `libs/` is checked only through a manifest at the
root. A manifest nested beside the package is invisible to every check.

**Do:** Add the root manifest for the language before adding a package that
needs one.

**Why:** Nothing reports the gap as a failure. `scripts/doctor` names the
orphan, but lint, type check, and test see no package at all and pass.

**Source:** [scripts/doctor](../scripts/doctor)
```

## Lessons

None recorded yet.
