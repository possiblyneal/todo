import { useEffect, useState } from 'react'

import { Activity } from './Activity'
import { Box } from './Box'
import { sentence } from './api'
import { Collections } from './Collections'
import { Detail } from './Detail'
import { Narrow } from './Narrow'
import { Row } from './Row'
import {
  fetchState,
  type Narrowing,
  OFFERED_NOTHING,
  queryString,
  type State,
  WIDE,
} from './state'

// The store's write-ahead log is what says a write happened, so this polls it
// once a second over HTTP, and the ETag is what keeps that to a 304 while
// nothing writes.
const POLL_MS = 1000

/**
 * Which screen is open. There is no router: the client is a handful of screens
 * and a box, and a screen is what is on the phone rather than an
 * address, so a dependency for it would be a decision and not a convenience.
 */
type Screen =
  | { name: 'list' }
  | { name: 'task'; id: string }
  | { name: 'agents' }
  | { name: 'collections' }

export function App() {
  const [state, setState] = useState<State | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [etag, setEtag] = useState<string | null>(null)
  const [screen, setScreen] = useState<Screen>({ name: 'list' })
  // The list opens on the everyday view, the same one `todo list` prints with
  // no flags. It is held here rather than in the controls because the poll and
  // the question both ask under it.
  const [narrowing, setNarrowing] = useState<Narrowing>(WIDE)
  // The narrowing the Tasks on the screen were read under, which is not the
  // one the controls show for the round trip after a control is touched. The
  // list is left standing meanwhile rather than blanked, so the sentence under
  // an empty one has to name the narrowing that emptied it and not the one
  // being asked for.
  const [drawn, setDrawn] = useState<Narrowing>(WIDE)

  // The query is what the effect depends on rather than the object holding it:
  // a Narrowing is a new object on every render and depending on one would
  // restart the poll forever.
  const query = queryString(narrowing)

  useEffect(() => {
    const controller = new AbortController()
    let etag: string | null = null

    const poll = async () => {
      try {
        const snapshot = await fetchState(narrowing, etag, controller.signal)
        // A null snapshot is 304: nothing changed, so nothing is redrawn.
        if (snapshot) {
          etag = snapshot.etag
          setState(snapshot.state)
          setDrawn(narrowing)
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
    // A changed narrowing starts the poll again from no ETag, which is what
    // keeps the tag and the list it describes the same age. The API hashes the
    // query into the tag as well, so an old one cannot be answered 304 against
    // a different view even if one were handed back.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query])

  // An error sits over the list rather than replacing it. A poll that failed
  // says nothing about the Tasks already on the screen, and a phone that walked
  // out of range should not have its list taken away while it walks back.
  const open = screen.name === 'task' ? find(state, screen.id) : undefined

  if (screen.name === 'collections') {
    return (
      <Collections
        offered={state ?? OFFERED_NOTHING}
        onBack={() => setScreen({ name: 'list' })}
      />
    )
  }

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
        offered={state}
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
      <Box
        offered={state ?? OFFERED_NOTHING}
        // A question is asked about the Tasks the list asked for, under the
        // same query string. The two go together or the box starts answering
        // about a list nobody is looking at.
        narrowing={narrowing}
      />
      {/*
        The controls are drawn before the first read lands, with nothing in the
        pickers but the everyday view. They are what asks for a list, so a
        screen that waited for a list before offering them would be waiting on
        itself.
      */}
      <Narrow
        narrowing={narrowing}
        offered={state ?? OFFERED_NOTHING}
        onChange={setNarrowing}
      />
      {error && <p className="message">{error}</p>}
      {!state && !error && <p className="message">Reading the list…</p>}
      {state && state.tasks.length === 0 && (
        <p className="message">
          {/*
            The List, the Tag and the search are what take Tasks out of a read
            this client asks for, so one of them set is a list narrowed to
            nothing. A sort reorders what came back and cannot empty it, and
            `all` widens rather than narrows, so neither is asked about here: a
            store with nothing in it says so under every sort and under the
            toggle both ways.

            That leaves the everyday view over a store holding only ended Tasks
            saying nothing is here yet. Telling that from an empty store would
            take a second read, and for a fresh store this is the right
            sentence.
          */}
          {!drawn.list && !drawn.tag && !drawn.search
            ? 'Nothing here yet.'
            : 'Nothing matches what the list is narrowed to.'}
        </p>
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
        <button
          type="button"
          onClick={() => setScreen({ name: 'collections' })}
        >
          Lists and Tags
        </button>
      </div>
    </>
  )
}

/** The Task a screen is open on, where the last read still names it. */
function find(state: State | null, id: string) {
  return state?.tasks.find((task) => task.id === id)
}
