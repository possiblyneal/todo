// A row in the list, and the verbs a press held on it opens.
//
// A tap opens the Task. A press held puts the four lifecycle verbs where the
// thumb already is, which is what the TUI's verb keys were: the same writes,
// reached without opening the Task first. There is no hover and no right
// click to hang them off, so holding is what a phone has.

import { useEffect, useRef, useState } from 'react'

import { sentence } from './api'
import type { Task } from './state'
import { lifecycle, VERBS } from './write'

/** How long a press is held before it is a press rather than a tap. */
const HELD_MS = 500

/** How far a thumb may travel and still be holding still rather than scrolling. */
const STILL_PX = 10

export function Row({
  task,
  onOpen,
}: {
  task: Task
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
        <span>{task.title}</span>
        {task.marks.length > 0 && (
          <span className="marks">{task.marks.join(' · ')}</span>
        )}
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
