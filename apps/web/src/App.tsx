import { useEffect, useState } from 'react'

import { Box } from './Box'
import { fetchState, type State } from './state'

// The write-ahead log was polled once a second by the TUI; this is the same
// poll over HTTP, and the ETag is what keeps it to a 304 while nothing writes.
const POLL_MS = 1000

export function App() {
  const [state, setState] = useState<State | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    let etag: string | null = null

    const poll = async () => {
      try {
        const snapshot = await fetchState(etag, controller.signal)
        // A null snapshot is 304: nothing changed, so nothing is redrawn.
        if (snapshot) {
          etag = snapshot.etag
          setState(snapshot.state)
        }
        setError(null)
      } catch (caught) {
        if (controller.signal.aborted) return
        // The tag described a response this client may no longer hold, so the
        // next poll asks for the whole thing rather than risking a 304 against
        // a screen that failed to draw.
        etag = null
        setError(caught instanceof Error ? caught.message : String(caught))
      }
    }

    // Each poll is scheduled once the one before it has settled rather than
    // on a fixed interval, so two can never be in flight together. Overlapping
    // reads come back in whatever order they come back in, and the slower one
    // would draw its older list over the newer one and leave a stale ETag
    // behind to be answered 304 against.
    let timer: ReturnType<typeof setTimeout> | undefined
    const tick = async () => {
      await poll()
      if (!controller.signal.aborted) {
        timer = setTimeout(() => void tick(), POLL_MS)
      }
    }

    void tick()
    return () => {
      controller.abort()
      clearTimeout(timer)
    }
  }, [])

  // An error sits over the list rather than replacing it. A poll that failed
  // says nothing about the Tasks already on the screen, and a phone that walked
  // out of range should not have its list taken away while it walks back.
  return (
    <>
      {/*
        The box is above the list rather than behind a tap, because a dump is
        the most frequent thing anybody does here and nothing should be stacked
        in front of it. The Lists and Tags it offers on the add sheet are the
        ones the last read named; before the first one there are none to offer
        and the sheet shows none.
      */}
      <Box lists={state?.lists ?? []} tags={state?.tags ?? []} />
      {error && <p className="message">{error}</p>}
      {!state && !error && <p className="message">Reading the list…</p>}
      {state && state.tasks.length === 0 && (
        <p className="message">Nothing here yet.</p>
      )}
      {state && state.tasks.length > 0 && (
        <ul className="list">
          {state.tasks.map((task) => (
            <li
              key={task.id}
              className="row"
              // Depth is 1 for a top-level Task, so the indent is what it has
              // beyond the top rather than the depth itself.
              style={{ paddingLeft: `${1 + (task.depth - 1) * 1.25}rem` }}
            >
              <span>{task.title}</span>
              {task.marks.length > 0 && (
                <span className="marks">{task.marks.join(' · ')}</span>
              )}
            </li>
          ))}
        </ul>
      )}
    </>
  )
}
