# apps/web

## Purpose

The person's surface: a TypeScript browser client, and the only thing a person
looks at, the TUI and `todo serve` having been deleted at the stage
`docs/plans/browser-client.md` set aside for it. It reads the tracker over the
JSON `todo api` serves and builds to static files that same process serves
beside the JSON, so there is no second process and no CORS.
`docs/adrs/0003-replace-the-tui-with-a-browser-client.md` records why the
surface moved off the terminal.

What it holds: the list over `GET /api/state` and the controls that narrow and
order it, the box that hands a dump to the Broker and opens the add sheet
filled in, the detail screen a tap on a Task opens, the Series screen and the
four things it does to a date, the breakdown that proposes Subtasks, the
collections screen over the Lists and Tags, and the activity screen over the
Change History. The six stages of
`docs/plans/browser-client.md` are all done; work since then is issue by issue
and adds to this list rather than to the plan.

## Ownership

- `src/api.ts` — the fetch plumbing every call shares: one JSON body out and
  one back, and the one place a failed response becomes an Error.
- `src/state.ts` — the wire shapes and the one call that reads them. It mirrors
  `apps/todo/src/api/state.go`, which is the side that decides them. It also
  holds the Narrowing and the one function that writes it as a query string,
  which both the list and the question ask under, and `Offered`: the served sets
  a surface picks from, which is the whole of the state bar the Tasks.
- `src/write.ts` — the wire shapes the write and Broker routes take, and the
  calls that reach them, and the client's one copy of the four lifecycle verbs
  and the three Occurrence marks, plus the fourth thing done to a date, which
  is a call of its own because it carries a whole Task. It mirrors
  `apps/todo/src/api/tasks.go`, `apps/todo/src/api/series.go`,
  `apps/todo/src/api/collections.go` and `apps/todo/src/api/broker.go`. It also turns
  a Task read back into the body that edits it, and takes the difference between
  the memberships a sheet opened on and the ones ticked when it was submitted.
- `src/read.ts` — the one read a screen makes for itself, and the guard around
  it: what came back, what went wrong, and the dropping of an answer that
  arrives after the screen has moved on.
- `src/log.ts` — what a Change History entry says, worked out without
  recognising anything by name: the Actor split on the first slash, and which
  kinds are the Lease bookkeeping rather than activity.
- `src/Box.tsx` — the box: a dump or a question, in the same field under the
  same thumb. Neither call writes.
- `src/Sheet.tsx` — the sheet: a Task open for correction, whether the Broker
  just read it or it already exists. Every attribute a Task has is on it, which
  is the title, description, why, deadline, estimate, priority, impact, color,
  snooze, the key/value pairs, and the Lists and Tags it is filed under. It
  makes no write of its own; whoever opens it says what submitting it does.
- `src/Narrow.tsx` — the controls over the list: the sort, the List, the Tag,
  the box searched in, and the one toggle that takes in the snoozed, completed,
  declined and deleted. It sets fields on the Narrowing and narrows nothing
  itself. The List and the Tag are one picker, because a Collection is the same
  shape either way.
- `src/Row.tsx` — one row of the list: the tap that opens the Task and the
  press held that puts the four verbs under it.
- `src/Series.tsx` — the Series screen: the rule, the dates it produces next,
  and the four things done to one of them. It works out no date of its own.
- `src/Breakdown.tsx` — the breakdown screen: the turn with the Broker, the
  questions it still has, and the proposals ticked by position.
- `src/Detail.tsx` — the detail screen: everything the Task carries, its
  Subtasks, its Series, its breakdown, the four lifecycle verbs, and its
  history.
- `src/Collections.tsx` — the collections screen: the Lists and the Tags
  created, renamed, recolored and deleted. Both sets are drawn by one component
  given the path segment, because a List and a Tag are the same three writes.
- `src/Activity.tsx` — the activity screen: the Change History across every
  Task, with the filter for Actors that name a harness and a model.
- `src/Log.tsx` — the entries drawn as who, what and when. Both screens draw
  their log through it.
- `src/App.tsx` — the box above the list, and which screen is open. It draws what the read returned and works nothing out for itself.
- `src/main.tsx` — the mount, and nothing else.
- `src/Sheet.test.tsx`, `src/Narrow.test.tsx`, `src/Collections.test.tsx` — the
  three components with a grammar: what a pick turns into on the wire, what a
  picker does with a value it cannot name, and which kind a collection write
  goes out under. The other components are drawn from what they are handed, so
  there is nothing in them a test would pin that reading them does not.
- `src/index.css` — the whole of the styling. There is no component-level
  stylesheet and no CSS-in-JS, so the 44px rule below is checkable by reading
  one file.
- `index.html`, `vite.config.ts`, `tsconfig.json`, `eslint.config.js` — the
  build. The dev server proxies `/api` so development has the one origin
  production has.
- `.unit.json` — `ships: none`, because what this unit produces is files
  somebody places rather than a program somebody starts. `npm run build` writes
  them to `dist/`, and on a host they are copied where `todo api -web` is
  pointed; `apps/todo/deploy/systemd/todo-api.service` is what points it.

## Local Contracts

- **Vite, React, and nothing else.** No router, no data-fetching library, no
  component kit. A dependency added here is a decision, not a convenience. The
  two under Verification below are the decision recorded in #49: the sheet's
  snooze and the pickers' unknown-value rule turn a pick into a different thing
  on the wire, and an inversion there is a wrong write nobody sees happen.
- **The client works nothing out that the store already did.** `Task.marks` is
  drawn as it arrives; a Task that read as snoozed from a keyboard cannot read
  as plain here.
- **`Depth` is 1 for a top-level Task.** `Tasks` comes back depth first, so the
  list indents by what a Task has beyond the top rather than by `depth` itself.
- **A 304 is nothing to redraw, not an empty state.** `fetchState` returns
  `null` for one and the view keeps what it has; the ETag is handed back on the
  next poll, which watches the store's write-ahead log over HTTP once a second.
- **One poll at a time.** The next is scheduled once the one before it has
  settled rather than on an interval, so two are never in flight together: the
  slower of an overlapping pair draws its older list over the newer one and
  leaves a stale ETag to be answered `304` against.
- **A failed poll takes nothing off the screen.** The error is drawn over the
  list it interrupted, because a read that failed says nothing about the Tasks
  already drawn, and a phone that walked out of range gets them back when it
  walks back rather than losing them on the way out.
- **An API error is shown in the API's own words.** The body's sentence is the
  one the CLI would have printed, so it is drawn rather than restated.
- **The sheet is the gate, and nothing before it writes.** Handing a dump over
  is a read: the Task comes back as a draft, and a draft abandoned leaves
  nothing behind. Only submitting the sheet calls `POST /api/tasks`.
- **Nothing the Broker said is dropped on the way to a field.** A deadline is
  typed rather than picked and a level that is none of the three is offered as
  a fourth, because a control that can hold only what it can parse would blank
  the Broker's answer before anybody saw it. What the API cannot read it says
  so about, in its own words, with the value still in the field.
- **A question is asked about the Tasks the read asked for.** `POST /api/ask`
  narrows by the same query string `GET /api/state` does, so whatever narrows
  the list narrows the question with it. `queryString` in `state.ts` is what
  makes that structural rather than a discipline: both calls build the string
  from the same Narrowing through the same function, and a narrowing added
  there is on the question the moment it is on the poll.
- **The sorts on offer are the API's, not a copy.** `GET /api/state` carries
  `sorts` from `store.Sorts`, so the picker cannot offer one the store would
  refuse and a fifth sort appears the day it lands. This is the pattern for a
  set the store owns; the level names, the verbs and the marks are copied only
  because no route answers what they are.
- **The served sets travel as one prop, never as several.** `Offered` bundles
  the Lists, the Tags, the sorts, the colors and the snoozes, and `State` is it
  plus the Tasks. Every component takes `offered` whole, including the three
  that only hand it on, so a sixth set the route serves is one field here rather
  than a prop threaded through them again. **What belongs in it is what the
  route serves, not what one screen reads.** A set used by both the controls and
  the sheet would have nowhere to live under a bundle shaped by its consumers,
  and the wire has one shape whatever reads it; the cost is that `Narrow` and
  `Sheet` each say which fields are not theirs. `Choice`, `Snooze` and `Picker`
  name their own list `options` or `all` rather than `offered`, so the served
  bundle and one control's values are never the same word in one file.
- **A component reading the wrong set off `Offered` is a mistake a test catches.**
  Five string lists behind one prop makes `offered.sorts` and `offered.colors`
  interchangeable to the compiler, so `Narrow.test.tsx` pins which set feeds the
  sort and `Sheet.test.tsx` pins the color and the snooze.
- **A changed narrowing restarts the poll from no ETag.** The tag and the list
  it describes have to be the same age, so `App` keys the polling effect on the
  query string. The API hashes the query into the tag as well, which means a
  stale one cannot be answered `304` against a different view even if it were
  handed back; the two together are belt and braces on the one mistake that
  would draw one narrowing's Tasks under another's controls.
- **Narrowing to a List narrows the tree, not just its roots.** The store
  applies the filter to Subtasks too, so a Task open from a List-narrowed read
  shows only the Subtasks in that List. That is `store.Tasks` behaving as
  `todo list -list` does, and the client draws what it returned rather than
  reassembling a tree the store did not describe.
- **Searching is a keystroke and a read, with no timer in between.** Each
  character is a new narrowing, so the poll restarts and the store answers off
  one query; a delay here to decide when typing stopped would be a list that
  lags the box it is searched from. The text goes out as typed, because the
  store is what matches it and `todo list -search` matches the same way.
- **The empty list says which of two things happened.** The List, the Tag and
  the search are what take Tasks out of a read this client asks for, so a read
  that came back with nothing under one of the three says the list is narrowed
  to nothing and otherwise says the store is empty. A sort cannot empty an
  answer and `all` widens rather than narrows, so neither is asked about. It branches on the narrowing the Tasks
  on the screen were read under, held in `App.tsx` as `drawn`, not on the one
  the controls show: the list is left standing through the round trip after a
  control is touched, so the sentence under an empty one has to name the
  narrowing that emptied it.
- **The three level names are the one thing the client keeps a copy of.**
  `Sheet.tsx` names them because `GET /api/state` does not carry them; a fourth
  added to `store.Levels` has to be added here too. Nothing is lost in the
  meantime: a level the client does not recognise is offered as an extra option
  rather than blanked, so the copy going stale costs a missing choice and never
  a dropped answer. The nine colors and the four snoozes were the same problem
  and are not any more: `colors` and `snoozes` arrive with the state, so the
  picker for each is the store's list and cannot offer a tenth or a fifth.
- **A rename and a recolor are one write, and each carries only what changed.**
  `Collections.tsx` edits a row's name and color in place and submits them
  together, because the store writes them as one entry and a screen that sent
  two would put two rows in the Change History for one correction. The body
  carries the attribute the row changed and leaves the other absent, which is
  what `api.collectionBody` reads as leave it alone: sending back the color the
  row opened on would undo a recolor another Actor made while it sat there.
  A row nobody touched cannot be saved at all, so no entry says nothing changed.
- **The collections screen says one thing about a refusal and clears the add
  row either way.** The message belongs to the write somebody just made and
  there is only ever one of those outstanding, so it sits above the screen
  rather than on a row. The blank row empties whether or not the write landed,
  because the refusal is already said in its own words and a name left sitting
  there is a name added twice by whoever read the sentence and tapped again.
  Deleting is one tap, the way the four verbs on a Task are: the Tasks that
  carried the Collection survive it and the Change History says it went.
- **A narrowing to something the client cannot name is kept on the screen.**
  `Picker` in `Narrow.tsx` draws an id it has no Collection for under the id
  itself. A List or a Tag deleted from another surface while the list is
  narrowed to it would otherwise match no option, so the control would render
  blank over a list that was still narrowed and nothing would say what
  happened. The rule has four sites: `Picker` here, `Choice` and `Snooze` in
  `Sheet.tsx` below, and `Color` in `Collections.tsx`, which keeps a color the
  served nine do not name so that saving a rename cannot clear it. The List and
  the Tag are one `Picker` because a Collection is the same shape either way,
  and that is the only merge the rule makes. `Color` and `Choice` are the
  nearest pair and stay apart: one is a bare control in a row and the other a
  labelled field in a form, so merging them would mean two props that configure
  chrome and one file's layout change having to consider the other's. What is
  duplicated across the four is the rule itself rather than the control, and
  lifting that one expression out is worth doing on its own rather than inside
  a feature branch. What `Picker`
  does share is `labelled`, which is what an option reads: a Collection the
  store named carries the count it worked out, and an id nothing named carries
  none, because a zero there would be this side answering a question the store
  never answered.
- **A picked value the client does not know is offered rather than dropped.**
  `Choice` in `Sheet.tsx` is one control for the levels and the Task's color
  alike — a Collection's color is `Color` in `Collections.tsx` — and a value that is none of the offered ones is added to the end of the list:
  the Broker chose the word, and a picker that silently could not hold it would
  lose what it said. The API refuses what it refuses, in the sentence the sheet
  shows.
- **Snooze is the one attribute the sheet cannot read back.** A Task carries the
  instant it wakes and the field takes the span to wait, so the control never
  opens knowing the answer: leaving it alone and waking the Task cannot be the
  same option, and they are two. Absent leaves a snoozed Task snoozed through an
  edit about something else, and waking it is the only way back from a snooze on
  this surface, since a snoozed Task is reached by showing everything the way an
  ended one is.
- **A key/value pair is removed by emptying it, and a key is never renamed.**
  The wire names a pair by its key, so what looks like a rename is a removal and
  an addition; offering it as one edit would be the sheet describing a write the
  API does not make. Emptying a value is the store's own rule for removing a
  pair rather than a delete this side invents.
- **A membership in the draft is on the screen before it is written.** An id
  the client cannot yet put a name to, which is the window before the first
  poll lands, is ticked under the id itself rather than hidden, because a
  membership nobody could untick is a write the sheet did not gate.
- **A dump survives backing out of the sheet.** Somebody who changed their mind
  about the Task has not changed their mind about having typed the sentence.
- **The list redraws on the next poll, not on the write.** A write answers with
  an id and nothing else; the poll a second later is what puts the Task on the
  screen, so there is one description of the list and it is the read's.
- **One sheet, and whoever opens it says what submitting it does.** Adding,
  editing and adding a Subtask are the same ten attributes, so they are one
  component taking an `onSubmit` rather than three copies of the form. The
  memberships it submits are the difference between what it opened on and what
  is ticked now, because ticking and unticking are different fields on the wire
  and an untick that sent nothing would leave the membership on. `against` is
  what the sheet takes that difference from where the draft is not it: a route
  that creates takes the memberships whole, so a create prefilled from a Task
  passes `{}` and sends the ticked sets rather than an empty difference.
- **The other screens re-read on the ETag, not on a clock.** `App` hands the
  poll's tag down as a revision; the detail, Series and activity screens fetch
  their own read again when it changes. A second poll of their own would be a
  second clock disagreeing with the first. The tag changes when something was
  written and also when the narrowing changes, because the API hashes the query
  into it. The second cannot be observed: the only screen the controls are on is
  the list, and the three are unmounted while it is. Do not go looking for the
  cost of it. A store whose write-ahead log cannot be stat'd carries no ETag at
  all, and then those three read once and never again while the list stays live.
  That is the one state where they are behind, and it is the same state the API
  describes as not knowing whether anything changed.
- **A verb is offered whatever state the Task is in.** Which of the four the
  store refuses is the store's to say, and it says it in a sentence. Reopen is
  offered on a deleted Task and refused there, in the store's own sentence,
  which is the rule working rather than failing: a screen that greyed the verb
  out would be a second copy of a rule that already exists, and would be wrong
  the day the store's answer changed. Reopen is reached through show everything: the everyday poll asks for the
  open Tasks, so an ended one is in the list to be tapped only under
  `?all=true`.
- **The four verbs are the second thing the client keeps a copy of.**
  `write.VERBS` names them because no route answers what they are, and both the
  row and the detail screen read that one list. A fifth added to
  `write.Lifecycle` has to be added here too, and until it is, the button is
  missing rather than a wrong sentence being drawn.
- **A press held on a row is what hover and a right click would have been.**
  Half a second, cancelled by a thumb that travels, and the click the browser
  sends afterwards is swallowed so one press does one thing. The refusal is
  drawn under the row it is about, because the row is where it was asked for.
- **The activity screen reaches further back by asking for more, not by
  stitching.** Show more raises the limit and re-reads, so a write that landed
  in between cannot appear twice. The button is there while the route filled
  the page it asked for, which is the only thing that says there may be more:
  what is drawn is smaller, since the Lease bookkeeping comes out client-side.
  The route's own cap is the client's third copy of something, in `state.ts`:
  nothing on the wire says where the route stops, and a button that asked past
  it would offer more and then produce none.
- **A verb that landed goes back to the list.** Three of the four take the Task
  out of the read the poll asks for, so the screen would be drawing a Task the
  next read does not describe. The list is where what happened is said, which
  is the same rule as a write redrawing on the poll rather than on itself.
- **A screen belongs to the Task it was opened on.** `Detail` is keyed on the
  id, so opening a Subtask from inside one starts a screen of its own: a log,
  an error and an open sheet are about one Task, and carrying them across would
  draw one Task's refusal over another's title.
- **The sheet diffs against what the Task carried when it opened.** The draft
  it is handed is recomputed from every poll, so the baseline is kept once at
  mount; diffing against a moving one would let a membership another Actor
  changed turn an untick into no change at all.
- **Lease bookkeeping is the one kind the client names.** `log.ts` lists
  `lease_taken`, `lease_released` and `lease_broken`, because the activity
  screen is two thirds plumbing without dropping them. Everything else about an
  entry is drawn in the store's own words: a kind is its name with the
  underscores taken out, so a kind added to the store appears the day it is
  appended and this client cannot describe one wrongly.
- **The activity filter narrows and never hides.** The screen opens unfiltered
  and the filter is a toggle, so an Agent that named itself with no slash, one
  run with no `TODO_ACTOR`, is in the view it opens on.
- **The Series is set as one value and the screen keeps no draft of it.** The
  rule is typed whole, for the reason a deadline is: a control offering the
  rules it could build would offer fewer than the parser accepts. The field
  starts empty against the rule drawn beside it rather than seeded from a read
  that moves under it on every poll, so `Replace` is a rule stated in full and
  never half of two. Stopping is its own button, because `DELETE` is what takes
  a rule off and an empty field is a refusal.
- **The three marks are the client's fourth copy of something.**
  `write.MARKS` names tick, skip and detach because no route answers what they
  are, the same as `write.VERBS`. All three are offered on every date; which of
  them the store refuses on a date already marked is the store's to say, and it
  says it in a sentence.
- **Lifting a date out corrected is the fourth button and not a fourth mark.**
  It opens the same sheet everything else does, on what the date would become,
  and writes nothing until it is submitted: a date backed out of is still an
  Occurrence, which is what makes it different from detaching and then editing
  what came back. The draft is the Task with that date as its deadline, which
  restates `store.Detached` and is the one thing this screen works out; the
  store is what writes it, and one refusal leaves no half-detached date behind.
  The sheet needs the Lists and Tags, which is why the detail screen hands them
  down to this screen as well as opening its own.
- **A failed first read draws no Series at all.** `useRead` holds `NONE` until
  an answer replaces it, so drawing `NONE` under the error sentence would say
  the Task does not repeat when all that happened is that nobody could find
  out. The screen tells the two apart by `NONE`'s own identity rather than by a
  second piece of state, because no answer is ever that object.
- **A detached date is followed, because nothing else afterwards names it.** The
  mark answers the id of the Task the date became and the screen opens it. Tick
  and skip answer the Task they were against, which is the screen already open,
  and nothing moves.
- **The breakdown holds its own conversation and writes nothing until it is
  approved.** Every turn carries everything already asked and answered, because
  the Broker keeps nothing between calls. Approving is one `addSubtask` per
  ticked proposal: there is no Lease over a browser's think time, so a tree that
  moved while the proposals were being read is what an add sheet left open
  already risks.
- **Proposals are ticked by position and never by title.** Two can come back
  saying the same thing and only the order tells them apart, so `approved` holds
  indices; everything starts ticked and unticking one is how it is declined,
  because a turn that proposed nothing worth keeping is the rarer answer. A
  breakdown backed out of leaves nothing behind, the same gate the sheet is for
  a dump. Each proposal that
  lands is unticked before the next is tried, so a refusal partway through
  leaves the button offering only what did not land: nothing stops the store
  writing a Subtask that says what another one says, so a second press on the
  whole list would write the landed ones twice.
- **Proposals win over questions when a turn carries both.**
  `POST /api/breakdown` carries both whole and ranks neither, so the order is
  this screen's to choose: the two are alternatives, and a turn carrying both is
  the Broker having answered oddly rather than having asked something. Taking
  the questions first would throw the proposals away and ask again for what was
  already proposed.
- **What the Broker proposed is drawn unparsed.** A proposal arrives in the same
  body the sheet submits and is written as it came, so an estimate this side
  cannot read is the API's to refuse in its own words rather than this screen's
  to drop.
- **Touch targets no smaller than 44px, one thumb, no hover.**

## Work Guidance

- Formatting is Prettier's alone. `eslint-config-prettier` sits last in
  `eslint.config.js`, so `npm run lint` and `npm run format:check` cannot
  disagree about a line.
- The npm scripts here are the interface `scripts/libs/detect.sh` dispatches
  node on, through the root workspace manifest. A script renamed here goes
  unrun and reports `unavailable`, which fails the gate.
- No `public/`: `scripts/structure` reads a folder under a unit that is neither
  `src/` nor a scoped folder as a domain and requires a `src/` inside it.

## Verification

`npm run lint`, `npm run typecheck`, `npm run test` and `npm run build` from
the repository root, or `scripts/check` for the gate CI runs.

`environment: 'node'` is the default, and the modules that talk to the API are
tested under it. A component test opts into a DOM with a
`// @vitest-environment jsdom` docblock at the top of its file, so the default
stays the cheaper one and a pure-function test cannot reach a DOM by accident.

`jsdom` and `@testing-library/react` are the only two devDependencies here that
exist for the tests. `@testing-library/user-event` and `jest-dom` are
deliberately absent: `fireEvent` and plain `expect` cover what these tests
assert, and the two-dependency floor above is what keeps that a decision.

## Child Index

None.
