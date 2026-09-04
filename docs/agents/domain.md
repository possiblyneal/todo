# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`CONTEXT.md`** at the repo root, or
- **`CONTEXT-MAP.md`** at the repo root if it exists — it points at one `CONTEXT.md` per context. Read each one relevant to the topic.
- **`docs/adrs/`** — read ADRs that touch the area you're about to work in.

If any of these files don't exist, **proceed silently**. Don't flag their absence; don't suggest creating them upfront. The `/domain-modeling` skill (reached via `/grill-with-docs` and `/improve-codebase-architecture`) creates them lazily when terms or decisions actually get resolved.

A repository starts single-context, with one `CONTEXT.md` at the root, and that covers almost every repository. `CONTEXT-MAP.md` is what a repository grows into once `apps/` holds several units, or one unit is split into several domains, and a single glossary would have to define the same word twice. Nothing decides between the two shapes in advance: read whichever file is there.

## File structure

```
/
├── CONTEXT.md
├── apps/<unit>/src/
└── docs/adrs/
    ├── 0000-template.md
    └── 0001-<slug>.md
```

`0000-template.md` is the template to copy, not a record; `scripts/adr-index`
lists everything else in `docs/adrs/index.md` and never lists it.

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in `CONTEXT.md`. Don't drift to synonyms the glossary explicitly avoids.

If the concept you need isn't in the glossary yet, that's a signal — either you're inventing language the project doesn't use (reconsider) or there's a real gap (note it for `/domain-modeling`).

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding:

> *Contradicts ADR-0007 (the decision it records) — but worth reopening because…*
