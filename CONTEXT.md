# Todo

A task tracker with two kinds of consumer: a person at a keyboard, and agents that add, edit, and delete while nobody is watching. The language below exists to keep those two from meaning different things by the same word.

## Contexts

Three boundaries divide the language. A context is an area of the business, not a program and not a folder: all three ship as one artifact, and that is settled rather than coincidental.

**Tracking**:
The area concerned with what work exists and how it is filed. It changes when the way work is organised changes.
_Avoid_: Tasks, core, domain

**Scheduling**:
The area concerned with repetition. It holds rules and dates and never task content — no title, tag, priority, or attachment lives here. Nothing in it runs on a schedule.
_Avoid_: Recurrence engine, scheduler, cron

**Change History**:
The area concerned with who did what and in what order, and also the name of the record at its centre. It changes when the arrangement of agents changes, not when the way work is organised does. It stores entries without reading inside them, so a new Task attribute changes Tracking alone.
_Avoid_: Audit, provenance, logging

## Language

### Tracking

**Task**:
A single piece of work the tracker holds, with its own lifecycle — added, described, completed, declined, reopened, deleted. There is one kind of Task: what a person works on and what an agent works on are the same thing, seen two ways.
_Avoid_: Todo, item, entry, ticket

**Declined**:
Said of a Task that will not be done. It ends the Task the way completing does, and a Task ends once — a Task that has ended is reopened before it ends the other way. It is not a deletion: a deleted Task is one that should not have been there, and a declined one was there, was looked at, and was refused. Nothing records why. Reopening undoes it, which is what makes declining the way to put a Task aside and keep it.
_Avoid_: Rejected, cancelled, dropped, won't-do

**Deleted**:
Said of a Task that should not have been there. It is the one thing there is no way back from: reopening a deleted Task is refused, so a Task somebody may want again is declined rather than deleted. What the Change History says about it stays, because a deletion is an appended entry and never an erasure; the Task itself does not come back.
_Avoid_: Removed, archived, trashed

**Subtask**:
A Task nested under another Task, to five levels. A Subtask is fixed where it was created: it never moves to a different parent and never leaves the Task it sits under. A top-level Task and everything nested beneath it are written and kept correct as one whole.
_Avoid_: Child task, step, checklist item

**Collection**:
Either of the two things a Task is filed under: a List or a Tag. Each is a thing in its own right rather than text on a Task — it is made, named, colored and deleted on its own, it outlives every Task that was filed under it, and deleting one takes nothing off a Task but the filing. The word exists because the two behave identically everywhere except in what somebody means by filing something under one.
_Avoid_: Group, bucket, folder, category

**List**:
A Collection a Task belongs to, standing for where the work sits: a house move, a job, a house. A Task may belong to more than one.
_Avoid_: Project, folder, category, bucket

**Tag**:
A named label a Task carries. It has an identity of its own rather than being the text typed on a Task — it can be renamed or recolored once, and counted across the tracker.
_Avoid_: Keyword, label, topic

**Color**:
One of nine offered colors a Task, a List or a Tag can carry: red, orange, yellow, green, cyan, blue, violet, magenta, brown. It is named rather than coded, and it is one of the nine or it is none — nothing else is stored, so every surface knows how to paint whatever it reads back.
_Avoid_: Colour, hex, swatch, highlight, theme

**Lease**:
An exclusive, expiring claim to edit one top-level Task and everything nested under it. Held by any actor before writing, and honoured by every other actor. Symmetric — a person's lease and an agent's lease are the same thing, and neither can tell which kind holds one.
_Avoid_: Lock, claim, reservation, checkout

**Attachment**:
A pointer held on a Task to something living outside the tracker — a file path or a web address. The tracker never holds a copy, so it cannot tell whether what is pointed at still exists.
_Avoid_: File, upload, document

**Narrowing**:
What one read of the Tasks asks for: which List, which Tags, what text to match, what order to come back in, and whether the snoozed and the ended are in. It describes a question and never a result, nothing stores one, and every Task that comes back came back because the store answered it — no surface sifts a list it was given. Naming a second Tag widens it rather than narrowing twice: a Task carrying any one of the named Tags is in the read.
_Avoid_: Filter, query, view, search

**Overdue**:
A condition true of a Task whose due date has passed, evaluated whenever something reads it. Nothing records the moment it becomes true.
_Avoid_: Late, expired

### Scheduling

**Series**:
A recurrence rule that produces Occurrences. It is edited as one thing, and editing it does not reach a date already Detached.
_Avoid_: Repeat, schedule, template, recurring task

**Occurrence**:
One date produced by a Series. It is worked out from the rule whenever something looks, and nothing is stored ahead of time. Stored state exists only for a date someone has acted on — ticked, skipped, or Detached.
_Avoid_: Instance, event, repetition

**Detached**:
Said of a date lifted out of its Series by being edited, becoming an ordinary Task that the rule no longer produces. Later edits to the Series do not reach it.
_Avoid_: Override, exception, modified instance

### Change History

**Change History**:
The single ordered sequence of every write in the tracker, kept rather than collapsed. It is the record, not a derived audit trail — current state is what you get by folding it. An actor interested in part of the tracker reads the sequence narrowed to that part.
_Avoid_: Log, audit log, journal, event stream

**Actor**:
Whoever performed a write — a person or an agent. Every write is attributed; reads are not. Only the Change History knows an Actor as an identity: elsewhere it is an opaque id, so the holder of a Lease can be recognised as the same actor but never named, and never told apart by kind.

An Agent names itself `<harness>/<model>` — `claude-code/claude-opus-5`, `orca/fable-5-1` — and a person is their bare login, so inside the Change History the slash is what says which wrote a thing and the two halves are what a history log reads. Nowhere else looks at it: an Actor is still opaque everywhere the entries are not. The model half is whatever served the call and is not a list anything here holds: OmniRoute fronts many providers and the set turns over. Nothing validates the shape. An Actor is a string the store writes down and reads back, one that names itself badly or not at all is still an Actor, and the day something refuses a write over its own name is the day attribution has started deciding what may be written.
_Avoid_: User, author, owner

**Agent**:
An actor that is invoked, acts, and exits. It has no schedule of its own and is not running between invocations. It is named for what it does here, which is write to the tracker under attribution, and not for whether it infers. The Broker infers and is not an Agent.
_Avoid_: Bot, worker, daemon, service

### Outside the contexts

**Broker**:
The thing that infers, reached over the network and owned by none of the three contexts. It is shown Tasks and answers with questions, proposals, or one Task read out of a dump; it holds nothing between calls, is never an Actor, and never writes. A proposal becomes a Task only when a person approves it, and the write is attributed to that person. A Task read out of a dump is not a proposal: it is what somebody already said they wanted, so running `todo capture` over their own words is the approval, and the write is attributed to them the same way.
_Avoid_: The box, the agent, the AI, the model, the assistant

**Dump**:
What somebody says a Task is, in their own words and in one box, before any field is filled in. The Broker reads one and answers with the Task it describes, so what comes back is that person's own words sorted into attributes rather than a suggestion of work nobody asked for. A dump amending a Task carries that Task as it stands and comes back whole.
_Avoid_: Prompt, note, request
