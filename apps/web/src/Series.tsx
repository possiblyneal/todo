// The Series screen: the rule a Task repeats on, the dates it produces next,
// and the four things done to one of them.
//
// The dates are the API's, computed as they were answered. Nothing here works
// out when a rule falls due: a screen that did would be a second copy of the
// arithmetic the store already has, free to disagree with it.

import { useState } from 'react'

import { sentence } from './api'
import { useRead } from './read'
import { Sheet } from './Sheet'
import {
  fetchSeries,
  type Collection,
  type Series as Repeating,
  type Task,
} from './state'
import { MARKS, detachEdited, draftOf, mark, repeat, unrepeat } from './write'

const NONE: Repeating = { repeats: false, occurrences: [] }

export function Series({
  task,
  lists,
  tags,
  revision,
  onOpen,
  onBack,
}: {
  task: Task
  lists: Collection[]
  tags: Collection[]
  /**
   * The ETag of the read on the screen, which is what this reads again on: a
   * mark made here lands on the next poll and the dates redraw then, the same
   * way the list redraws on the read rather than on the write.
   */
  revision: string | null
  /** Where a detached date goes: it is an ordinary Task now, with a screen. */
  onOpen: (id: string) => void
  onBack: () => void
}) {
  const { value: series, error: unread } = useRead(
    () => fetchSeries(task.id),
    NONE,
    [task.id, revision],
  )
  // The rule is typed whole rather than edited in place. A Series is one value
  // and this field is what it is being set to, so it starts empty against the
  // rule drawn above it instead of being seeded from a read that moves under
  // it every poll.
  const [rule, setRule] = useState('')
  const [refused, setRefused] = useState<string | null>(null)
  const [working, setWorking] = useState(false)
  // Which date the sheet is open on, when it is. Nothing is written by opening
  // it: the date stays an Occurrence until the sheet is submitted, which is
  // what makes this different from detaching and then editing what came back.
  const [editing, setEditing] = useState<string | null>(null)
  // Every write here is the same shape: say nothing about what happens next,
  // because the poll is what says it. What comes back is either a sentence on
  // the screen or nothing at all.
  const write = async (make: () => Promise<void>) => {
    setWorking(true)
    setRefused(null)
    try {
      await make()
    } catch (caught) {
      setRefused(sentence(caught))
    }
    setWorking(false)
  }

  // Whether a read has ever come back, which is NONE's own identity and not a
  // second piece of state: `useRead` holds NONE until an answer replaces it,
  // and no answer is ever that object.
  const read = series !== NONE

  if (editing !== null) {
    return (
      <Sheet
        // What the date would become, which is the recurring Task with that
        // date as its deadline. `store.Detached` says the same thing on the
        // other side; this is the draft, and the store is what writes it.
        draft={{ ...draftOf(task), deadline: editing }}
        // Lifting a date out creates a Task, and `store.detach` writes the
        // Lists and the Tags it is handed rather than a change to what the
        // recurring Task carries, so the ticked sets go whole.
        against={{}}
        lists={lists}
        tags={tags}
        action="Lift out"
        onSubmit={async (body) => {
          // The lift and the corrections are one write, so backing out of this
          // sheet leaves the date an Occurrence and nothing in the record.
          const written = await detachEdited(task.id, editing, body)
          setEditing(null)
          onOpen(written)
        }}
        onCancel={() => setEditing(null)}
      />
    )
  }

  return (
    <div className="detail">
      <div className="buttons">
        <button type="button" onClick={onBack}>
          Back
        </button>
      </div>

      <h1 className="title">{task.title}</h1>
      {(refused ?? unread) && <p className="message">{refused ?? unread}</p>}

      {/*
        Nothing below is drawn until a read has come back. A failed first read
        knows nothing about the rule, and NONE drawn under the error would say
        the Task does not repeat rather than that nobody could find out.
      */}
      {read && (
        <>
          <h2 className="heading">Repeats</h2>
          <p className="marks">{series.rule ?? 'Not a repeating task.'}</p>

          <form
            className="field"
            onSubmit={(event) => {
              event.preventDefault()
              void write(async () => {
                await repeat(task.id, rule)
                setRule('')
              })
            }}
          >
            {/*
              Typed rather than picked, for the reason the sheet types a
              deadline: the rule is a sentence the parser reads, and a control
              offering the rules it could build would offer fewer than the
              parser accepts.
            */}
            <span>Rule</span>
            <input
              value={rule}
              onChange={(event) => setRule(event.target.value)}
              placeholder="every week on mon,thu"
            />
            <div className="buttons">
              <button type="submit" disabled={working || rule.trim() === ''}>
                {series.repeats ? 'Replace' : 'Repeat'}
              </button>
              {series.repeats && (
                <button
                  type="button"
                  disabled={working}
                  onClick={() => void write(() => unrepeat(task.id))}
                >
                  Stop
                </button>
              )}
            </div>
          </form>

          <h2 className="heading">Dates</h2>
          {!series.repeats && <p className="message">None.</p>}
          {series.repeats && series.occurrences.length === 0 && (
            <p className="message">The rule produces no more dates.</p>
          )}
          <ul className="list">
            {series.occurrences.map((on) => (
              <li key={on.date} className="row date">
                <span>{on.date}</span>
                {on.state && <span className="marks">{on.state}</span>}
                {/*
                  All four are offered on every date. Which of them the store
                  refuses on a date already marked is the store's to say, and
                  it says it in a sentence, which is the same rule the four
                  lifecycle verbs are drawn under.
                */}
                <div className="buttons">
                  {MARKS.map((which) => (
                    <button
                      key={which}
                      type="button"
                      disabled={working}
                      onClick={() =>
                        void write(async () => {
                          const written = await mark(task.id, which, on.date)
                          // Detaching answers a Task that did not exist
                          // before, and going to it is the only thing that
                          // names it.
                          if (written !== task.id) onOpen(written)
                        })
                      }
                    >
                      {which}
                    </button>
                  ))}
                  {/*
                    The fourth, and the only one that shows what the date would
                    become before anything is written: this week's is different,
                    corrected on the ordinary form, and lifted out on submit.
                  */}
                  <button
                    type="button"
                    disabled={working}
                    onClick={() => setEditing(on.date)}
                  >
                    edit
                  </button>
                </div>
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  )
}
