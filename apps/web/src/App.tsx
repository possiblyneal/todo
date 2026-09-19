import { useEffect, useState } from 'react'

import { Activity } from './Activity'
import { Box } from './Box'
import { sentence } from './api'
import { Collections } from './Collections'
import { Detail } from './Detail'
import { Narrow, Search } from './Narrow'
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
        // Aborting does not reject a response that already arrived, so a poll
        // torn down between the response and its body draws the narrowing it
        // asked under over the one that replaced it. The catch guards for the
        // same reason; this is the other half of it.
        if (controller.signal.aborted) return
        // A null snapshot is 304: nothing changed, so nothing is redrawn.
        if (snapshot) {
          etag = snapshot.etag
          setState(snapshot.state)
          setDrawn(narrowing)
          // The tag is the revision the other screens fetch their own reads
          // again on. It changes when something was written and also when the
          // narrowing did, since the API hashes the query into it; a narrowing
          // cannot change while those screens are mounted, so what they see is
          // the first of the two.
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

  // Selected and open are one state, not two: a tap selects a Task, and which
  // of the two ways that is drawn is the layout's. `selected` is what somebody
  // tapped and `open` is the Task the last read still describes; they differ
  // only while a Task is going away under the selection.
  const selected = screen.name === 'task'
  const open = selected ? find(state, screen.id) : undefined

  if (screen.name === 'collections') {
    return (
      <Collections
        offered={state ?? OFFERED_NOTHING}
        // Whether a read has landed, which is not the same as having no Lists:
        // this screen is reachable inside the first second and a poll that has
        // been failing since load would otherwise tell somebody their Lists are
        // gone. The error is drawn over what is there for the reason the list
        // draws it that way.
        read={state !== null}
        error={error}
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

  return (
    <div className={selected ? 'panes showing' : 'panes'}>
      <div className="pane filters">
        {/*
          The controls are drawn before the first read lands, with nothing in
          the pickers but the everyday view. They are what asks for a list, so a
          screen that waited for a list before offering them would be waiting on
          itself.
        */}
        <Narrow
          narrowing={narrowing}
          offered={state ?? OFFERED_NOTHING}
          onChange={setNarrowing}
        />
      </div>

      <div className="pane middle">
        {/*
          The box is above the list rather than behind a tap, because a dump is
          the most frequent thing anybody does here and nothing should be
          stacked in front of it. On a phone that is `index.css` ordering this
          pane's children into the one column ahead of the filtering, since the
          panes as written would put the filtering first. The Lists and Tags it
          offers on the add sheet are the ones the last read named; before the
          first one there are none to offer and the sheet shows none.
        */}
        <Box
          offered={state ?? OFFERED_NOTHING}
          // A question is asked about the Tasks on the screen, which is the
          // narrowing they were read under and not the one the controls are
          // showing: a question asked between a control moving and its list
          // arriving would be answered about a list nobody is looking at yet.
          narrowing={drawn}
        />
        {/*
          The box the list is searched in sits over the Tasks and not among the
          filtering: it is about what is under it, which is the same thing on a
          phone and in the middle pane at a desk.
        */}
        <Search
          value={narrowing.search}
          onChange={(search) => setNarrowing({ ...narrowing, search })}
        />
        {/*
          An error sits over the list rather than replacing it. A poll that
          failed says nothing about the Tasks already on the screen, and a
          phone that walked out of range should not have its list taken away
          while it walks back.
        */}
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
            {!drawn.list && drawn.tags.length === 0 && !drawn.search
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
                lists={state.lists}
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
      </div>

      {/*
        The Task that is selected, which is the same selection whichever width
        this is drawn at: beside the list at a desk and instead of it on a
        phone, decided in `index.css` rather than by anything measured here.

        A Task the read no longer names is a Task that went out of the list
        while it was selected: deleted from another surface, or narrowed out
        by a filter changed on this one. The pane says so rather than
        disappearing, because the tap that selected it was somebody's, and the
        sentence says the list rather than the tracker because which of the two
        it was is not something this surface knows.
      */}
      {selected && state && (
        <div className="pane opened">
          {open ? (
            <Detail
              // Keyed on the Task, so opening a Subtask from here starts a
              // screen of its own rather than reusing this one: the history,
              // the error and the open sheet all belong to the Task they were
              // about.
              key={open.id}
              task={open}
              subtasks={state.tasks.filter((task) => task.parent === open.id)}
              // A State is an Offered with the Tasks on it, and the guard on
              // the pane has already said there is one, so this site does not
              // need the empty stand-in the three above do.
              offered={state}
              revision={etag}
              onOpen={(id) => setScreen({ name: 'task', id })}
              onBack={() => setScreen({ name: 'list' })}
            />
          ) : (
            <>
              <p className="message">That task is not in the list any more.</p>
              <button type="button" onClick={() => setScreen({ name: 'list' })}>
                Back
              </button>
            </>
          )}
        </div>
      )}
    </div>
  )
}

/** The Task a screen is open on, where the last read still names it. */
function find(state: State | null, id: string) {
  return state?.tasks.find((task) => task.id === id)
}
