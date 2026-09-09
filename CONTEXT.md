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
Said of a Task that will not be done. It ends the Task the way completing does, and a Task ends once — a Task that has ended is reopened before it ends the other way. It is not a deletion: a deleted Task is one that should not have been there, and a declined one was there, was looked at, and was refused. Nothing records why. Reopening undoes it.
_Avoid_: Rejected, cancelled, dropped, won't-do

**Subtask**:
A Task nested under another Task, to five levels. A Subtask is fixed where it was created: it never moves to a different parent and never leaves the Task it sits under. A top-level Task and everything nested beneath it are written and kept correct as one whole.
_Avoid_: Child task, step, checklist item

**List**:
A named collection a Task belongs to. It exists before any Task is in it and survives after the last one leaves, carries its own name and colour, and a Task may belong to more than one.
_Avoid_: Project, folder, category, bucket

**Tag**:
A named label a Task carries. It has an identity of its own rather than being the text typed on a Task — it can be renamed or recoloured once, and counted across the tracker.
_Avoid_: Keyword, label, topic

**Lease**:
An exclusive, expiring claim to edit one top-level Task and everything nested under it. Held by any actor before writing, and honoured by every other actor. Symmetric — a person's lease and an agent's lease are the same thing, and neither can tell which kind holds one.
_Avoid_: Lock, claim, reservation, checkout

**Attachment**:
A pointer held on a Task to something living outside the tracker — a file path or a web address. The tracker never holds a copy, so it cannot tell whether what is pointed at still exists.
_Avoid_: File, upload, document

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
_Avoid_: User, author, owner

**Agent**:
An actor that is invoked, acts, and exits. It has no schedule of its own and is not running between invocations.
_Avoid_: Bot, worker, daemon, service
