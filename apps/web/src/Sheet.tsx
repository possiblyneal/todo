// The sheet: a Task open for correction, whether it is one the Broker just read
// or one that already exists. Nothing about the Task is written before submit,
// so a dump handed over and then thought better of leaves nothing behind.
//
// It makes one write of its own, which is the List or Tag its ticks offer to
// make: a Collection is an aggregate of its own rather than part of the Task,
// so making one is not the gate giving way. Every other write is whoever opens
// the sheet saying what submitting it does, which is what lets one sheet be the
// add form, the edit form and the subtask form without holding three
// descriptions of the same ten attributes.

import { useEffect, useState } from 'react'

import { sentence } from './api'
import {
  type Collection,
  fetchFiles,
  type Files,
  type Level,
  type Offered,
} from './state'
import { addCollection, type Kind, memberships, type TaskBody } from './write'

/** The attributes this sheet takes as text, which is every one it shows. */
type Said = 'title' | 'description' | 'why' | 'deadline' | 'estimate'

export function Sheet({
  draft,
  against,
  offered,
  action,
  onSubmit,
  onCancel,
}: {
  draft: TaskBody
  /**
   * What submitting this is a change against, where that is not the draft it
   * opened on. An edit is a change against the Task, which is the default. A
   * create takes the memberships whole, so a create prefilled from a Task
   * passes `{}` here: leaving the draft as the baseline would send an empty
   * difference and write a Task belonging to no List the sheet drew ticked.
   */
  against?: TaskBody
  /**
   * The Lists and Tags to file the Task under, the colors it may carry and the
   * snoozes on offer, as `GET /api/state` answered them. The client keeps no
   * list of any of them, so it cannot offer a value the store would refuse.
   * The sorts travel in the same type and are the controls' rather than this
   * form's.
   */
  offered: Offered
  /** The word on the button, which is what submitting it does. */
  action: string
  onSubmit: (body: TaskBody) => Promise<void>
  onCancel: () => void
}) {
  const [body, setBody] = useState<TaskBody>(draft)
  // What the Task carried when this opened, kept rather than read again. The
  // draft prop is recomputed from every poll, so a membership another Actor
  // changed while the sheet was open would move the baseline under it and an
  // untick would come out as no change at all.
  const [opened] = useState<TaskBody>(against ?? draft)
  const [error, setError] = useState<string | null>(null)
  const [writing, setWriting] = useState(false)

  const say = (name: Said, value: string) =>
    setBody((was) => ({ ...was, [name]: value }))

  const toggle = (name: 'intoLists' | 'addTags', id: string) =>
    setBody((was) => {
      const on = was[name] ?? []
      return {
        ...was,
        [name]: on.includes(id)
          ? on.filter((other) => other !== id)
          : [...on, id],
      }
    })

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    setWriting(true)
    setError(null)
    try {
      // The memberships are the difference between what the Task carried when
      // this opened and what is ticked now, because ticking and unticking are
      // different fields on the wire.
      await onSubmit({ ...body, ...memberships(opened, body) })
    } catch (caught) {
      // The API's sentence is the one the CLI would have printed, and a value
      // it could not read is still in the field it came back in, so whoever
      // is looking at the sheet can correct that value rather than retype the
      // whole Task.
      setError(sentence(caught))
      setWriting(false)
    }
  }

  return (
    <form className="sheet" onSubmit={(event) => void submit(event)}>
      {error && <p className="message">{error}</p>}

      <label className="field">
        <span>Title</span>
        <input
          value={body.title ?? ''}
          onChange={(event) => say('title', event.target.value)}
          required
          autoFocus
        />
      </label>

      <label className="field">
        <span>Description</span>
        <textarea
          value={body.description ?? ''}
          onChange={(event) => say('description', event.target.value)}
          rows={3}
        />
      </label>

      <label className="field">
        <span>Why</span>
        <input
          value={body.why ?? ''}
          onChange={(event) => say('why', event.target.value)}
        />
      </label>

      {/*
        A date is typed, and picked beside being typed. The box is the field:
        the Broker answers in prose and may say a day this program cannot read,
        and a control holding only what it can parse would blank the answer on
        the way to the screen, which is the one thing this sheet exists to
        prevent. What it could not read stays in the box and the API says so in
        its own words.

        The picker writes into the box and never reads it. "next Friday" is not
        a date it can show, and one that blanked or guessed at it would be the
        same failure a control later. So it is the platform's own, uncontrolled,
        and the box is still free text after a date lands in it.
      */}
      <fieldset className="field">
        <legend>Deadline</legend>
        <div className="pair">
          <input
            aria-label="Deadline"
            value={body.deadline ?? ''}
            onChange={(event) => say('deadline', event.target.value)}
            placeholder="2026-03-04"
          />
          <input
            type="date"
            aria-label="Pick a deadline"
            onChange={(event) => say('deadline', event.target.value)}
          />
        </div>
      </fieldset>

      <label className="field">
        <span>Estimate</span>
        <input
          value={body.estimate ?? ''}
          onChange={(event) => say('estimate', event.target.value)}
          placeholder="90m"
        />
      </label>

      <Choice
        name="Priority"
        options={offered.priorities.map((one) => one.name)}
        examples={offered.priorities}
        value={body.priority ?? ''}
        onPick={(value) => setBody((was) => ({ ...was, priority: value }))}
      />
      <Choice
        name="Impact"
        options={offered.impacts.map((one) => one.name)}
        examples={offered.impacts}
        value={body.impact ?? ''}
        onPick={(value) => setBody((was) => ({ ...was, impact: value }))}
      />
      <Choice
        name="Color"
        options={offered.colors}
        value={body.color ?? ''}
        onPick={(value) => setBody((was) => ({ ...was, color: value }))}
      />

      <Snooze
        options={offered.snoozes}
        value={body.snooze}
        onPick={(value) => setBody((was) => ({ ...was, snooze: value }))}
      />

      <Fields
        on={body.fields ?? {}}
        onChange={(fields) => setBody((was) => ({ ...was, fields }))}
      />

      <Pointers
        on={body.attachments ?? []}
        onChange={(attachments) => setBody((was) => ({ ...was, attachments }))}
      />

      <Ticks
        name="Lists"
        kind="lists"
        all={offered.lists}
        on={body.intoLists ?? []}
        onToggle={(id) => toggle('intoLists', id)}
        onFail={setError}
      />
      <Ticks
        name="Tags"
        kind="tags"
        all={offered.tags}
        on={body.addTags ?? []}
        onToggle={(id) => toggle('addTags', id)}
        onFail={setError}
      />

      <div className="buttons">
        <button type="button" onClick={onCancel} disabled={writing}>
          Cancel
        </button>
        <button type="submit" disabled={writing}>
          {writing ? '…' : action}
        </button>
      </div>
    </form>
  )
}

/**
 * One attribute whose values are a list somebody picks from: a level, or a
 * color. A value that is none of them is offered as one more rather than
 * dropped, because the Broker chose the word and a list that silently cannot
 * hold it would lose what it said. The API refuses the ones it refuses, and
 * says so in the sentence the sheet shows.
 *
 * Empty clears the attribute, which is the same rule an emptied text field
 * follows.
 */
function Choice({
  name,
  options,
  examples,
  value,
  onPick,
}: {
  name: string
  options: string[]
  /**
   * What each value means, where the route says. The levels carry one and the
   * colors do not: blue means blue. A value with none is offered under its own
   * name, which is what a value the client cannot recognise gets as well.
   */
  examples?: Level[]
  value: string
  onPick: (value: string) => void
}) {
  const shown =
    value === '' || options.includes(value) ? options : [...options, value]
  const said = (one: string) => examples?.find((each) => each.name === one)
  return (
    <label className="field">
      <span>{name}</span>
      {/*
        The example is in the option rather than under the picker, because the
        question it answers is asked while the three are side by side. Showing
        only the chosen one's would mean picking each in turn to read them,
        which is the choice being made to find out what the choice is.
      */}
      <select value={value} onChange={(event) => onPick(event.target.value)}>
        <option value="">—</option>
        {shown.map((one) => (
          <option key={one} value={one}>
            {said(one) ? `${one} — ${said(one)?.example}` : one}
          </option>
        ))}
      </select>
    </label>
  )
}

/**
 * How long to hide the Task for. It is not a Choice because a Task cannot be
 * read back into one: what it carries is the instant it wakes rather than the
 * span somebody asked for, so the sheet never opens knowing the answer and the
 * two things leaving it alone and waking it up cannot be the same option.
 *
 * Undefined is left alone, which is what an untouched sheet sends and what
 * keeps a snoozed Task snoozed through an edit about something else. The empty
 * string wakes it, which is the only way back from a snooze on this surface.
 */
function Snooze({
  options,
  value,
  onPick,
}: {
  options: string[]
  value?: string
  onPick: (value: string | undefined) => void
}) {
  // A snooze the Broker said is offered as one more, the way a Choice offers a
  // word it did not know: `write.Snooze` takes a plain duration as well as the
  // labels, so one is not a value to drop on the way to the screen.
  const shown =
    value === undefined || value === '' || options.includes(value)
      ? options
      : [...options, value]
  return (
    <label className="field">
      <span>Snooze</span>
      <select
        value={value === undefined ? 'leave' : value === '' ? 'wake' : value}
        onChange={(event) => {
          const picked = event.target.value
          onPick(
            picked === 'leave' ? undefined : picked === 'wake' ? '' : picked,
          )
        }}
      >
        <option value="leave">—</option>
        <option value="wake">wake it</option>
        {shown.map((one) => (
          <option key={one} value={one}>
            {one}
          </option>
        ))}
      </select>
    </label>
  )
}

/**
 * The key/value pairs, edited in place with one blank row to add another.
 *
 * A value emptied removes its pair, which is the store's own rule rather than a
 * delete this side invents. A key is not renamed: the wire names a pair by its
 * key, so a rename is a removal and an addition, and offering it as one edit
 * would be the sheet describing a write the API does not make.
 */
function Fields({
  on,
  onChange,
}: {
  on: Record<string, string>
  onChange: (fields: Record<string, string>) => void
}) {
  const [key, setKey] = useState('')
  const [value, setValue] = useState('')

  const add = () => {
    if (key === '') return
    onChange({ ...on, [key]: value })
    setKey('')
    setValue('')
  }

  return (
    <fieldset className="field">
      <legend>Fields</legend>
      {Object.entries(on).map(([name, held]) => (
        <label key={name} className="pair">
          <span>{name}</span>
          <input
            value={held}
            placeholder="empty removes it"
            onChange={(event) =>
              onChange({ ...on, [name]: event.target.value })
            }
          />
        </label>
      ))}
      <div className="pair">
        <input
          value={key}
          placeholder="name"
          onChange={(event) => setKey(event.target.value)}
        />
        <input
          value={value}
          placeholder="value"
          onChange={(event) => setValue(event.target.value)}
        />
        <button type="button" onClick={add} disabled={key === ''}>
          Add
        </button>
      </div>
    </fieldset>
  )
}

/**
 * The Attachments to add, collected and not written. Each is its own guarded
 * write against a Task that may not exist yet, so submitting is what sends
 * them: a draft backed out of leaves no pointer behind because none was sent.
 *
 * It adds and never removes. An edit opens with this empty rather than with
 * what the Task carries, because taking one off is the detail screen's, which
 * is the screen that can show what is there.
 *
 * A pointer is text and nothing else. Nothing is uploaded and nothing fetched,
 * so one naming a file names it on the machine `todo api` runs on rather than
 * on the phone it was typed into.
 */
function Pointers({
  on,
  onChange,
}: {
  on: string[]
  onChange: (on: string[]) => void
}) {
  const [target, setTarget] = useState('')
  const [browsing, setBrowsing] = useState(false)

  const add = () => {
    const pointer = target.trim()
    if (pointer === '' || on.includes(pointer)) return
    onChange([...on, pointer])
    setTarget('')
  }

  return (
    <fieldset className="field">
      <legend>Attachments</legend>
      {on.map((pointer) => (
        <div className="pair pointer" key={pointer}>
          <span>{pointer}</span>
          <button
            type="button"
            onClick={() => onChange(on.filter((other) => other !== pointer))}
          >
            Remove
          </button>
        </div>
      ))}
      <div className="pair">
        <input
          aria-label="New attachment"
          placeholder="https://… or /a/path"
          value={target}
          onChange={(event) => setTarget(event.target.value)}
        />
        <button type="button" onClick={() => setBrowsing(!browsing)}>
          {browsing ? 'Close' : 'Browse'}
        </button>
        <button type="button" onClick={add} disabled={target.trim() === ''}>
          Attach
        </button>
      </div>
      {browsing && (
        <Machine
          onPick={(path) => {
            setTarget(path)
            setBrowsing(false)
          }}
        />
      )}
    </fieldset>
  )
}

/**
 * The machine `todo api` runs on, one directory at a time. It is here because
 * a pointer naming a file names it on that machine: the browser's own file
 * input answers with a bare filename and no directory, so a file chosen on a
 * phone would be a path the host cannot resolve.
 *
 * It fills the box and never reads it back, which is the rule the deadline's
 * picker follows: a pointer half typed is not a path this could show, and the
 * box stays the field that is submitted.
 *
 * It lists and never opens. Nothing is fetched and nothing is copied in, so
 * what a name points at is as unknown here as it is to the store.
 */
function Machine({ onPick }: { onPick: (path: string) => void }) {
  const [at, setAt] = useState<string | undefined>(undefined)
  const [files, setFiles] = useState<Files | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let live = true
    fetchFiles(at)
      .then((answer) => {
        if (!live) return
        setFiles(answer)
        setError('')
      })
      .catch((caught: unknown) => {
        if (live) setError(sentence(caught))
      })
    return () => {
      live = false
    }
  }, [at])

  if (error !== '') return <p className="aside">{error}</p>
  if (!files) return <p className="aside">…</p>
  return (
    <div role="group" aria-label="Files">
      <p className="aside">{files.path}</p>
      {files.parent !== '' && (
        <button
          type="button"
          className="row"
          onClick={() => setAt(files.parent)}
        >
          <span>Up a directory</span>
        </button>
      )}
      {files.entries.map((one) => {
        // The separator the host uses is the one in the path it answered, and
        // every path it answers is absolute, so this is a join rather than a
        // guess: `todo api` is a Unix service and the route is the only thing
        // that names a directory here.
        const path = `${files.path}/${one.name}`
        return (
          <button
            key={one.name}
            type="button"
            className="row"
            onClick={() => (one.dir ? setAt(path) : onPick(path))}
          >
            <span>{one.dir ? `${one.name}/` : one.name}</span>
          </button>
        )
      })}
      {files.entries.length === 0 && <p className="aside">Nothing here.</p>}
    </div>
  )
}

/**
 * The Lists or Tags the Task is filed under, ticked by id, with the row that
 * makes one more.
 *
 * An id the draft carries that the client cannot name is shown anyway, under
 * the id itself. That happens in the window before the first poll lands, and
 * the alternative is a membership submitted without ever being on the screen,
 * which is not what the sheet being the gate means.
 *
 * Making one is the sheet's one exception to nothing here writing, and it is
 * not really an exception: a List is an aggregate of its own, and the one made
 * here exists on the same terms as one made on the collections screen. It
 * outlives a sheet backed out of, so the row says so rather than letting
 * somebody discover it later. A blank set still draws, because a tracker with
 * no Lists is exactly where somebody needs to make the first one.
 *
 * The made one is held here until the poll names it, so it ticks under the word
 * that was typed rather than under its id for the second it takes to come back.
 * It is ticked on arrival: making a List from the form that files a Task under
 * one is somebody saying which List, not adding to a catalogue.
 */
function Ticks({
  name,
  kind,
  all,
  on,
  onToggle,
  onFail,
}: {
  name: string
  kind: Kind
  all: Collection[]
  on: string[]
  onToggle: (id: string) => void
  /** Where a refused creation is said, which is the sheet's own one place. */
  onFail: (said: string) => void
}) {
  // Only the id and the name, because those are the two this side knows. A
  // color and a count filled in here would be this side answering questions
  // the store never answered, which is the rule the unnamed ids below follow.
  const [made, setMade] = useState<{ id: string; name: string }[]>([])
  const [naming, setNaming] = useState('')
  const [making, setMaking] = useState(false)

  const named = new Set(all.map((one) => one.id))
  const held = made.filter((one) => !named.has(one.id))
  const shown = [
    ...all,
    ...held,
    ...on
      .filter((id) => !named.has(id) && !held.some((one) => one.id === id))
      .map((id) => ({ id, name: id })),
  ]

  // The singular, because the row is about making one. The plural is the
  // legend above the ticks and says what the set is.
  const singular = kind === 'lists' ? 'List' : 'Tag'

  const make = async () => {
    // The name as the store will hold it, since it trims one on the way in.
    // Sending it untrimmed would draw the typed spacing until the poll took
    // them away, which is this side describing a write it did not make.
    const name = naming.trim()
    setMaking(true)
    try {
      const id = await addCollection(kind, { name })
      setMade((was) => [...was, { id, name }])
      onToggle(id)
      // Only if the box still holds what went out: a name typed while the
      // request was in flight is the next one somebody means to make, and
      // blanking it would throw away what they had just typed.
      setNaming((now) => (now === naming ? '' : now))
      // The sentence belonged to a write that has now been followed by one
      // that landed, and a refusal left standing over a Collection that was
      // made says the wrong thing about the tick beside it.
      onFail('')
    } catch (caught) {
      onFail(sentence(caught))
    } finally {
      setMaking(false)
    }
  }

  return (
    <fieldset className="field">
      <legend>{name}</legend>
      {shown.map((one) => (
        <label key={one.id} className="tick">
          <input
            type="checkbox"
            checked={on.includes(one.id)}
            onChange={() => onToggle(one.id)}
          />
          <span>{one.name}</span>
        </label>
      ))}
      <div className="pair">
        <input
          aria-label={`New ${singular}`}
          placeholder={`new ${singular.toLowerCase()}`}
          value={naming}
          onChange={(event) => setNaming(event.target.value)}
        />
        <button
          type="button"
          aria-label={`Add ${singular}`}
          onClick={() => void make()}
          disabled={making || naming.trim() === ''}
        >
          Add
        </button>
      </div>
      <span className="aside">
        Made when you tap Add, and kept even if this sheet is cancelled.
      </span>
    </fieldset>
  )
}
