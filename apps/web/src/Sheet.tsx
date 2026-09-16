// The add sheet: what the Broker read, filled in and open for correction.
// Submitting it is the only thing that writes, so a dump handed over and then
// thought better of leaves nothing behind.

import { useState } from 'react'

import type { Collection } from './state'
import { addTask, type TaskBody } from './write'

/** The attributes this sheet takes as text, which is every one it shows. */
type Said = 'title' | 'description' | 'why' | 'deadline' | 'estimate'

const LEVELS = ['low', 'med', 'high']

export function Sheet({
  draft,
  lists,
  tags,
  onWritten,
  onCancel,
}: {
  draft: TaskBody
  lists: Collection[]
  tags: Collection[]
  onWritten: () => void
  onCancel: () => void
}) {
  const [body, setBody] = useState<TaskBody>(draft)
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
      await addTask(body)
      onWritten()
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

      <Level
        name="Priority"
        value={body.priority ?? ''}
        onPick={(value) => setBody((was) => ({ ...was, priority: value }))}
      />
      <Level
        name="Impact"
        value={body.impact ?? ''}
        onPick={(value) => setBody((was) => ({ ...was, impact: value }))}
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
          {writing ? 'Adding…' : 'Add'}
        </button>
      </div>
    </form>
  )
}

/**
 * One of the three levels. A value that is none of them is offered as a fourth
 * rather than dropped: the Broker chose the word, and a list that silently
 * cannot hold it would lose what it said.
 */
function Level({
  name,
  value,
  onPick,
}: {
  name: string
  value: string
  onPick: (value: string) => void
}) {
  const offered =
    value === '' || LEVELS.includes(value) ? LEVELS : [...LEVELS, value]
  return (
    <label className="field">
      <span>{name}</span>
      <select value={value} onChange={(event) => onPick(event.target.value)}>
        <option value="">—</option>
        {offered.map((level) => (
          <option key={level} value={level}>
            {level}
          </option>
        ))}
      </select>
    </label>
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
