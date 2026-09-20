// The box is the front door. A Task said the way somebody thinks of it, one
// tap from the list with nothing stacked in front of it, and the question
// under the same thumb: a dump and a question are both "say a sentence about
// the list".
//
// It is the front door to a Subtask as well, under a Task rather than over the
// list: the same sentence read the same way, submitted as a Subtask of the
// Task it was typed under. A question is about a list and there is no list
// there, so that box asks nothing and draws no Ask.
//
// Neither call writes. Handing a dump over opens the add sheet filled in, and
// submitting that sheet is the only thing that writes; a question is answered
// as prose over the Tasks in view and appends nothing.

import { useState } from 'react'

import { Sheet } from './Sheet'
import type { Narrowing, Offered } from './state'
import { addSubtask, addTask, ask, capture, type TaskBody } from './write'

/**
 * Which box this is, and the two are alternatives rather than two switches
 * over one fact. A `Narrowing` makes it the list's: the dump becomes a
 * top-level Task and a question is asked about the Tasks in view. A `parent`
 * makes it that Task's: the dump becomes a Subtask of it and there is no list
 * to ask about. Both at once and neither are the two states the code would
 * have had to describe and nobody could have reached, so the type refuses
 * them.
 */
type Where =
  | { narrowing: Narrowing; parent?: never }
  | { parent: string; narrowing?: never }

export function Box({
  offered,
  narrowing,
  parent,
}: {
  /** What the add sheet picks from. Nothing here is read on the way past. */
  offered: Offered
} & Where) {
  const [text, setText] = useState('')
  const [working, setWorking] = useState<'' | 'reading' | 'asking'>('')
  const [error, setError] = useState<string | null>(null)
  const [answer, setAnswer] = useState<string | null>(null)
  const [draft, setDraft] = useState<TaskBody | null>(null)

  // The Broker holds nothing between calls, so each of these is one turn and
  // the whole of it. Both take minutes at worst, which is why the box says
  // which one it is waiting on rather than only that it is busy.
  //
  // What the turn is is handed in rather than branched on here: the question is
  // only ever asked from the button that knows what it is about, so there is no
  // arm of this that has to wonder whether there was a list.
  const hand = async (
    what: 'reading' | 'asking',
    turn: (said: string) => Promise<void>,
  ) => {
    const said = text.trim()
    if (said === '' || working !== '') return
    setWorking(what)
    setError(null)
    setAnswer(null)
    try {
      await turn(said)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught))
    } finally {
      setWorking('')
    }
  }

  if (draft) {
    return (
      <Sheet
        draft={draft}
        // Adding is a create, and `POST /api/tasks` files the Task under the
        // Lists and Tags it is handed rather than a change to them. The Broker
        // fills both in, so they are the draft as well as what is ticked: left
        // as the baseline they would cancel out and the dump would land filed
        // under nothing.
        against={{}}
        offered={offered}
        // A Task being written now is hidden from nothing, so the sheet does
        // not offer to snooze it.
        existing={false}
        action="Add"
        // The dump is done with once the Task is written. Backing out of the
        // sheet keeps it, because somebody who changed their mind about the
        // Task has not changed their mind about having typed the sentence.
        onSubmit={async (body) => {
          // A dump under a Task is a Subtask of it; the sentence was read the
          // same way either side of that, and only the route differs.
          if (parent) await addSubtask(parent, body)
          else await addTask(body)
          setDraft(null)
          setText('')
        }}
        onCancel={() => setDraft(null)}
      />
    )
  }

  return (
    <div className="box">
      <textarea
        className="dump"
        value={text}
        onChange={(event) => setText(event.target.value)}
        placeholder={
          parent ? 'Say the subtask' : 'Say the task, or ask about the list'
        }
        rows={2}
      />
      <div className="buttons">
        <button
          type="button"
          onClick={() =>
            void hand('reading', async (said) => setDraft(await capture(said)))
          }
          disabled={working !== '' || text.trim() === ''}
        >
          {working === 'reading' ? 'Reading…' : 'Add'}
        </button>
        {/* A question is about a list, and a Task's own box has none. */}
        {narrowing && (
          <button
            type="button"
            onClick={() =>
              void hand('asking', async (said) =>
                setAnswer(await ask(said, narrowing)),
              )
            }
            disabled={working !== '' || text.trim() === ''}
          >
            {working === 'asking' ? 'Asking…' : 'Ask'}
          </button>
        )}
      </div>
      {error && <p className="message">{error}</p>}
      {answer && <p className="answer">{answer}</p>}
    </div>
  )
}
