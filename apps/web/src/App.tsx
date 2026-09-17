import { useEffect, useState } from 'react'

import { Activity } from './Activity'
import { Box } from './Box'
import { sentence } from './api'
import { Detail } from './Detail'
import { Row } from './Row'
import { fetchState, type State } from './state'

// The store's write-ahead log is what says a write happened, so this polls it
// once a second over HTTP, and the ETag is what keeps that to a 304 while
// nothing writes.
const POLL_MS = 1000

/**
 * Which of the three screens is open. There is no router: the client is three
 * screens and a box, and a screen is what is on the phone rather than an
 * address, so a dependency for it would be a decision and not a convenience.
 */
type Screen =
  { name: 'list' } | { name: 'task'; id: string } | { name: 'agents' }

export function App() {
  const [state, setState] = useState<State | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [etag, setEtag] = useState<string | null>(null)
  const [screen, setScreen] = useState<Screen>({ name: 'list' })

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
          // The tag is the revision the other screens fetch their own reads
          // again on: it changes exactly when something was written, which is
          // what keeps them current without a second clock.
          setEtag(snapshot.etag)
        }
        setError(null)
      } catch (caught) {
        if (controller.signal.aborted) return
        // The tag described a response this client may no longer hold, so the
        // next poll asks for the whole thing rather than risking a 304 against
        // a screen that failed to draw.
        etag = null
        setError(sentence(caught))
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
  const open = screen.name === 'task' ? find(state, screen.id) : undefined

  if (screen.name === 'agents') {
    return (
      <Activity
        tasks={state?.tasks ?? []}
        revision={etag}
        onBack={() => setScreen({ name: 'list' })}
      />
    )
  }

  // A Task the read no longer names is a Task that went away while it was
  // open, which is what deleting one from another surface looks like from
  // here. The list is what is left, rather than a screen about nothing.
  if (screen.name === 'task' && state && !open) {
    return (
      <>
        <p className="message">That task is not in the list any more.</p>
        <button type="button" onClick={() => setScreen({ name: 'list' })}>
          Back
        </button>
      </>
    )
  }

  if (open && state) {
    return (
      <Detail
        // Keyed on the Task, so opening a Subtask from here starts a screen of
        // its own rather than reusing this one: the history, the error and the
        // open sheet all belong to the Task they were about.
        key={open.id}
        task={open}
        subtasks={state.tasks.filter((task) => task.parent === open.id)}
        lists={state.lists}
        tags={state.tags}
        revision={etag}
        onOpen={(id) => setScreen({ name: 'task', id })}
        onBack={() => setScreen({ name: 'list' })}
      />
    )
  }

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
            <Row
              key={task.id}
              task={task}
              onOpen={(id) => setScreen({ name: 'task', id })}
            />
          ))}
        </ul>
      )}
      <div className="buttons">
        <button type="button" onClick={() => setScreen({ name: 'agents' })}>
          Activity
        </button>
      </div>
    </>
  )
}

/** The Task a screen is open on, where the last read still names it. */
function find(state: State | null, id: string) {
  return state?.tasks.find((task) => task.id === id)
}
