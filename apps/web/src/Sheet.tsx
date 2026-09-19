// The sheet: a Task open for correction, whether it is one the Broker just read
// or one that already exists. Submitting it is the only thing that writes, so a
// dump handed over and then thought better of leaves nothing behind.
//
// It makes no write of its own. Whoever opens it says what submitting it does,
// which is what lets one sheet be the add form, the edit form and the subtask
// form without holding three descriptions of the same ten attributes.

import { useState } from 'react'

import type { Collection } from './state'
import { memberships, type TaskBody } from './write'

/** The attributes this sheet takes as text, which is every one it shows. */
type Said = 'title' | 'description' | 'why' | 'deadline' | 'estimate'

/**
 * The three level names, and the one list the client still keeps a copy of:
 * nothing on the wire carries them, so a fourth added to `store.Levels` has to
 * be added here too. The colors and the snoozes used to be the same problem and
 * are not any more — `GET /api/state` carries both.
 */
const LEVELS = ['low', 'med', 'high']

export function Sheet({
  draft,
  against,
  lists,
  tags,
  colors,
  snoozes,
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
  lists: Collection[]
  tags: Collection[]
  /**
   * The colors a Task may carry and the snoozes on offer, as
   * `GET /api/state` answered them. The client keeps no list of either, so it
   * cannot offer a color the store would refuse.
   */
  colors: string[]
  snoozes: string[]
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
      setError(caught instanceof Error ? caught.message : String(caught))
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
        A date is typed rather than picked. The Broker answers in prose and may
        say a day this program cannot read, and a native picker holds only what
        it can parse: it would blank the answer on the way to the screen, which
        is the one thing this sheet exists to prevent. What it could not read
        stays in the field and the API says so in its own words.
      */}
      <label className="field">
        <span>Deadline</span>
        <input
          value={body.deadline ?? ''}
          onChange={(event) => say('deadline', event.target.value)}
          placeholder="2026-03-04"
        />
      </label>

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
        offered={LEVELS}
        value={body.priority ?? ''}
        onPick={(value) => setBody((was) => ({ ...was, priority: value }))}
      />
      <Choice
        name="Impact"
        offered={LEVELS}
        value={body.impact ?? ''}
        onPick={(value) => setBody((was) => ({ ...was, impact: value }))}
      />
      <Choice
        name="Color"
        offered={colors}
        value={body.color ?? ''}
        onPick={(value) => setBody((was) => ({ ...was, color: value }))}
      />

      <Snooze
        offered={snoozes}
        value={body.snooze}
        onPick={(value) => setBody((was) => ({ ...was, snooze: value }))}
      />

      <Fields
        on={body.fields ?? {}}
        onChange={(fields) => setBody((was) => ({ ...was, fields }))}
      />

      <Ticks
        name="Lists"
        all={lists}
        on={body.intoLists ?? []}
        onToggle={(id) => toggle('intoLists', id)}
      />
      <Ticks
        name="Tags"
        all={tags}
        on={body.addTags ?? []}
        onToggle={(id) => toggle('addTags', id)}
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
  offered,
  value,
  onPick,
}: {
  name: string
  offered: string[]
  value: string
  onPick: (value: string) => void
}) {
  const shown =
    value === '' || offered.includes(value) ? offered : [...offered, value]
  return (
    <label className="field">
      <span>{name}</span>
      <select value={value} onChange={(event) => onPick(event.target.value)}>
        <option value="">—</option>
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
  offered,
  value,
  onPick,
}: {
  offered: string[]
  value?: string
  onPick: (value: string | undefined) => void
}) {
  // A snooze this side was never offered is shown as one more, the way a Choice
  // shows a word it did not know: `write.Snooze` takes a plain duration as well
  // as the four labels, and `store.SnoozeNames` serves the offer rather than the
  // rule, so a Task snoozed by that duration from a terminal would otherwise
  // reach this screen with its snooze blanked.
  const shown =
    value === undefined || value === '' || offered.includes(value)
      ? offered
      : [...offered, value]
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
    // An empty value is how the wire says remove, so adding a pair with one
    // would draw a row that submitting deletes.
    if (key === '' || value === '') return
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
 * The Lists or Tags the Task is filed under, ticked by id.
 *
 * An id the draft carries that the client cannot name is shown anyway, under
 * the id itself. That happens in the window before the first poll lands, and
 * the alternative is a membership submitted without ever being on the screen,
 * which is not what the sheet being the gate means.
 */
function Ticks({
  name,
  all,
  on,
  onToggle,
}: {
  name: string
  all: Collection[]
  on: string[]
  onToggle: (id: string) => void
}) {
  const named = new Set(all.map((one) => one.id))
  const shown = [
    ...all,
    ...on.filter((id) => !named.has(id)).map((id) => ({ id, name: id })),
  ]
  if (shown.length === 0) return null
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
    </fieldset>
  )
}
