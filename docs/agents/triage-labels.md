# Triage Labels

The `triage` skill works in canonical role names. This file records the label string each role uses in this repository's issue tracker. Every label string below equals its role name, so a skill that mentions a role can apply it directly.

Every triaged issue carries **exactly one category role and exactly one state role**.

## Category roles

- `bug` — something is broken
- `enhancement` — new feature or improvement

## State roles

- `needs-triage` — maintainer needs to evaluate this issue
- `needs-info` — waiting on the reporter for more information
- `ready-for-agent` — fully specified, ready for an AFK agent
- `ready-for-human` — needs human implementation
- `wontfix` — will not be actioned

## A missing label is created, not substituted

GitHub ships `bug`, `enhancement`, and `wontfix` on a new repository. The other four do not exist until someone makes them:

```bash
gh label create needs-triage --description "Maintainer needs to evaluate this issue"
```

Create the label rather than reaching for the nearest one that already exists. GitHub's stock `question` is a category — its own description is "Further information is requested" — so applying it in place of `needs-info` puts a category label in a state slot and breaks the one-of-each rule above. The two read as synonyms and sit on different axes.

## Renaming

To run under a vocabulary the tracker already has, replace a role's label string above and say which role it stands for. The `triage` skill reads this file, so it applies the existing label instead of creating a duplicate.
