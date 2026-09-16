import { useEffect, useState } from 'react'

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

    void poll()
    const timer = setInterval(() => void poll(), POLL_MS)
    return () => {
      controller.abort()
      clearInterval(timer)
    }
  }, [])

  if (error) return <p className="message">{error}</p>
  if (!state) return <p className="message">Reading the list…</p>
  if (state.tasks.length === 0)
    return <p className="message">Nothing here yet.</p>

  return (
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
  )
}
