// The collections screen: the Lists and the Tags, created, renamed, recolored
// and deleted. It is the TUI's `ctrl+l` and `ctrl+t`, which were two screens
// there and are one here for the reason the API's routes are one pair of
// handlers: a List and a Tag are the same three writes against different
// aggregates.
//
// It draws what the poll already read rather than reading for itself. A write
// made here lands on the next poll and the rows redraw then, the same way the
// list redraws on the read rather than on the write.

import { useState } from 'react'

import { sentence } from './api'
import type { Collection, Offered } from './state'
import {
  addCollection,
  type CollectionBody,
  describeCollection,
  dropCollection,
  type Kind,
} from './write'

export function Collections({
  offered,
  read,
  error,
  onBack,
}: {
  /** The Lists, the Tags and the nine colors, as the last read named them. */
  offered: Offered
  /**
   * Whether a read has landed. Nothing to draw means one of three things and
   * this screen has to tell them apart: a read that has not come back yet, a
   * read that failed, and a store with no Lists in it. Saying the third when it
   * is one of the first two is telling somebody their Lists are gone.
   */
  read: boolean
  /** What the poll was told, drawn over the screen rather than in place of it. */
  error: string | null
  onBack: () => void
}) {
  // What a write was told. It is one message for the screen rather than one
  // per row, because a refusal is about the write somebody just made and there
  // is only ever one of those outstanding.
  const [refused, setRefused] = useState<string | null>(null)

  // A write's own promise is what the buttons wait on, so a double tap cannot
  // send the same create twice while the first is still in flight.
  const [working, setWorking] = useState(false)

  const write = async (made: Promise<unknown>) => {
    setWorking(true)
    setRefused(null)
    try {
      await made
    } catch (caught) {
      setRefused(sentence(caught))
    } finally {
      setWorking(false)
    }
  }

  return (
    <div className="detail">
      <div className="buttons">
        <button type="button" onClick={onBack}>
          Back
        </button>
      </div>

      {refused && <p className="message">{refused}</p>}
      {error && <p className="message">{error}</p>}

      <Kinds
        kind="lists"
        name="Lists"
        all={offered.lists}
        colors={offered.colors}
        read={read}
        working={working}
        onWrite={write}
      />
      <Kinds
        kind="tags"
        name="Tags"
        all={offered.tags}
        colors={offered.colors}
        read={read}
        working={working}
        onWrite={write}
      />
    </div>
  )
}

/** One of the two sets: its rows, and the blank one that creates another. */
function Kinds({
  kind,
  name,
  all,
  colors,
  read,
  working,
  onWrite,
}: {
  kind: Kind
  name: string
  all: Collection[]
  colors: string[]
  read: boolean
  working: boolean
  onWrite: (made: Promise<unknown>) => Promise<void>
}) {
  return (
    <>
      <h2 className="heading">{name}</h2>
      {all.length === 0 && (
        <p className="message">{read ? 'None.' : 'Reading…'}</p>
      )}
      <ul className="list">
        {all.map((one) => (
          <OneCollection
            // Keyed on what the row is drawn from and not on the id alone. The
            // id keeps a half typed name off whichever row takes its place in
            // the order; the name and the color put the row back on the read's
            // baseline the moment a poll carries a new one, so a Collection
            // another Actor renamed cannot leave this one offering to save the
            // name it opened on over the top of theirs.
            key={`${one.id}:${one.name}:${one.color}`}
            one={one}
            colors={colors}
            working={working}
            onDescribe={(body) =>
              onWrite(describeCollection(kind, one.id, body))
            }
            onDrop={() => onWrite(dropCollection(kind, one.id))}
          />
        ))}
      </ul>
      <Add kind={kind} colors={colors} working={working} onWrite={onWrite} />
    </>
  )
}

/**
 * One List or Tag. The name and the color are edited in place and submitted
 * together, because the store writes them as one entry and a screen that sent
 * two would put two rows in the Change History for one correction.
 *
 * The count is drawn and not edited: it is what the read worked out, and the
 * way to change it is to file a Task under this one.
 */
function OneCollection({
  one,
  colors,
  working,
  onDescribe,
  onDrop,
}: {
  one: Collection
  colors: string[]
  working: boolean
  onDescribe: (body: CollectionBody) => void
  onDrop: () => void
}) {
  const [name, setName] = useState(one.name)
  const [color, setColor] = useState(one.color)
  const renamed = name !== one.name
  const recolored = color !== one.color
  const changed = renamed || recolored

  return (
    <li className="row">
      <input
        className="control"
        aria-label={`${one.name} name`}
        value={name}
        onChange={(event) => setName(event.target.value)}
      />
      <Color
        name={`${one.name} color`}
        colors={colors}
        value={color}
        onPick={setColor}
      />
      <span className="marks">{one.count}</span>
      <button
        type="button"
        // Nothing to write is nothing to submit, which is also what keeps a row
        // nobody touched from appending an entry that changed nothing.
        disabled={working || !changed}
        onClick={() =>
          // Only what this row changed. The body's absent attribute is what
          // tells the store to leave the other one alone, so a rename that
          // carried the color it opened on would put back a recolor another
          // Actor made while the row sat here.
          onDescribe({
            ...(renamed && { name }),
            ...(recolored && { color }),
          })
        }
      >
        Save
      </button>
      {/*
        Deleting is one tap, the way the four verbs on a Task are. The Tasks
        that carried this one survive it and the Change History says it went,
        so there is nothing here a second tap would protect.
      */}
      <button type="button" disabled={working} onClick={onDrop}>
        Delete
      </button>
    </li>
  )
}

/** The blank row: a name, a color, and the write that mints the id. */
function Add({
  kind,
  colors,
  working,
  onWrite,
}: {
  kind: Kind
  colors: string[]
  working: boolean
  onWrite: (made: Promise<unknown>) => Promise<void>
}) {
  const [name, setName] = useState('')
  const [color, setColor] = useState('')

  const add = async () => {
    await onWrite(addCollection(kind, { name, color }))
    // The row is cleared whether or not the write landed, because a refusal is
    // said in its own words above and a name left sitting here would be added
    // twice by somebody who read the sentence and tapped again.
    setName('')
    setColor('')
  }

  return (
    <div className="row">
      <input
        className="control"
        aria-label={`New ${kind === 'lists' ? 'List' : 'Tag'}`}
        placeholder="name"
        value={name}
        onChange={(event) => setName(event.target.value)}
      />
      <Color
        name={`New ${kind === 'lists' ? 'List' : 'Tag'} color`}
        colors={colors}
        value={color}
        onPick={setColor}
      />
      <button
        type="button"
        disabled={working || name === ''}
        onClick={() => void add()}
      >
        Add
      </button>
    </div>
  )
}

/**
 * The nine, as the route serves them. A color the client cannot offer is kept
 * on the picker under its own name for the reason the narrowing's pickers keep
 * an unknown id: a control that rendered blank over a value that is really set
 * would clear it the moment anything else on the row was saved.
 */
function Color({
  name,
  colors,
  value,
  onPick,
}: {
  name: string
  colors: string[]
  value: string
  onPick: (value: string) => void
}) {
  const shown =
    value === '' || colors.includes(value) ? colors : [...colors, value]
  return (
    <select
      className="control"
      aria-label={name}
      value={value}
      onChange={(event) => onPick(event.target.value)}
    >
      <option value="">no color</option>
      {shown.map((one) => (
        <option key={one} value={one}>
          {one}
        </option>
      ))}
    </select>
  )
}
