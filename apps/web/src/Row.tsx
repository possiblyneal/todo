// A row in the list, and the verbs a press held on it opens.
//
// Enough of the Task to tell it from the one under it without opening either:
// what it is called, the start of what it says, when it arrived, when it is
// due, what it is filed under, and whether it points anywhere. All of it came
// with the read, so nothing here fetches and nothing here is worked out that
// the store already did.
//
// A tap opens the Task. A press held puts the four lifecycle verbs where the
// thumb already is: the same writes, reached without opening the Task first.
// There is no hover and no right click to hang them off, so holding is what a
// phone has.

import { useEffect, useRef, useState } from 'react'

import { sentence } from './api'
import type { Collection, Task } from './state'
import { lifecycle, VERBS } from './write'

/** How long a press is held before it is a press rather than a tap. */
const HELD_MS = 500

/** How far a thumb may travel and still be holding still rather than scrolling. */
const STILL_PX = 10

/**
 * An instant the API sent, as the day it falls on where the reader is. A string
 * no date can be read out of is drawn as it came: the same rule the pickers
 * follow for a value they cannot name, and the API is what refuses a bad one.
 */
function day(instant: string): string {
  const at = new Date(instant)
  return Number.isNaN(at.getTime()) ? instant : at.toLocaleDateString()
}

/**
 * The Lists a Task is filed under, by name. An id the read did not name is
 * drawn as the id, for the reason the detail screen does it: a filing nobody
 * can see is one nobody thinks to change.
 */
function filed(ids: string[] | undefined, all: Collection[]): string[] {
  return (ids ?? []).map((id) => all.find((one) => one.id === id)?.name ?? id)
}

export function Row({
  task,
  lists,
  onOpen,
}: {
  task: Task
  /** The Lists the read named, to draw the ones this Task is filed under. */
  lists: Collection[]
  onOpen: (id: string) => void
}) {
  const [verbs, setVerbs] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [working, setWorking] = useState(false)
  // Whether the press that is ending was held. The browser sends a click after
  // one, and opening the Task under the verbs somebody just asked for would be
  // the row doing two things to one press.
  const held = useRef(false)
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const from = useRef({ x: 0, y: 0 })

  const stop = () => clearTimeout(timer.current)

  // A row goes when the next poll no longer names its Task, which a verb from
  // this very row is one way to cause. A press still counting down then has
  // nothing left to open.
  useEffect(() => () => clearTimeout(timer.current), [])

  const press = (event: React.PointerEvent) => {
    held.current = false
    from.current = { x: event.clientX, y: event.clientY }
    timer.current = setTimeout(() => {
      held.current = true
      setVerbs(true)
    }, HELD_MS)
  }

  // A thumb on its way down the list is scrolling, not holding: the press ends
  // as soon as it travels, or the verbs open under somebody reading.
  const travel = (event: React.PointerEvent) => {
    const { x, y } = from.current
    if (Math.abs(event.clientX - x) > STILL_PX) stop()
    if (Math.abs(event.clientY - y) > STILL_PX) stop()
  }

  const act = async (verb: string) => {
    setWorking(true)
    setError(null)
    try {
      await lifecycle(task.id, verb)
      // The write landed, so the next poll is what says what the list is now.
      setVerbs(false)
    } catch (caught) {
      // The API's own sentence, under the row it is about: a Task that ends
      // once says so here rather than on a screen somewhere else.
      setError(sentence(caught))
    }
    setWorking(false)
  }

  return (
    <li>
      <button
        type="button"
        className="row"
        // Depth is 1 for a top-level Task, so the indent is what it has beyond
        // the top rather than the depth itself.
        style={{ paddingLeft: `${1 + (task.depth - 1) * 1.25}rem` }}
        onPointerDown={press}
        onPointerMove={travel}
        onPointerUp={stop}
        onPointerCancel={stop}
        onPointerLeave={stop}
        // The platform's own callout on a long press would cover the verbs.
        onContextMenu={(event) => event.preventDefault()}
        onClick={() => {
          if (held.current) {
            held.current = false
            return
          }
          onOpen(task.id)
        }}
      >
        <span className="said">
          <span className="titled">{task.title}</span>
          {task.marks.length > 0 && (
            <span className="marks">{task.marks.join(' · ')}</span>
          )}
        </span>
        {/*
          Two lines and then clipped, in CSS. A count of characters this side
          would be guessing at a width it cannot see, and would guess wrong on
          every phone but the one it was written against.
        */}
        {task.description && <span className="lines">{task.description}</span>}
        <span className="facts">
          <span>Added {day(task.createdAt)}</span>
          {task.deadline && <span>Due {day(task.deadline)}</span>}
          {filed(task.lists, lists).map((name) => (
            <span key={name}>{name}</span>
          ))}
          {/*
            An attachment is not one of `store.Task.Marks`: the marks are the
            store's vocabulary for what a Task is, and holding a pointer is not
            a state it is in. So the row counts them rather than the store
            growing a mark nothing else would read.
          */}
          {(task.attachments?.length ?? 0) > 0 && (
            <span aria-label="Has attachments">📎</span>
          )}
        </span>
      </button>

      {verbs && (
        <>
          <div className="verbs">
            {VERBS.map((verb) => (
              <button
                key={verb}
                type="button"
                onClick={() => void act(verb)}
                disabled={working}
              >
                {verb}
              </button>
            ))}
            <button
              type="button"
              onClick={() => {
                setVerbs(false)
                setError(null)
              }}
            >
              cancel
            </button>
          </div>
          {error && <p className="message">{error}</p>}
        </>
      )}
    </li>
  )
}
