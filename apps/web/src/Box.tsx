// The box is the front door. A Task said the way somebody thinks of it, one
// tap from the list with nothing stacked in front of it, and the question
// under the same thumb: a dump and a question are both "say a sentence about
// the list".
//
// Neither call writes. Handing a dump over opens the add sheet filled in, and
// submitting that sheet is the only thing that writes; a question is answered
// as prose over the Tasks in view and appends nothing.

import { useState } from 'react'

import { Sheet } from './Sheet'
import type { Collection, Narrowing } from './state'
import { addTask, ask, capture, type TaskBody } from './write'

export function Box({
  lists,
  tags,
  colors,
  snoozes,
  narrowing,
}: {
  lists: Collection[]
  tags: Collection[]
  colors: string[]
  snoozes: string[]
  // What the list is narrowed to, so a question is asked about the Tasks on
  // the screen rather than about every open one.
  narrowing: Narrowing
}) {
  const [text, setText] = useState('')
  const [working, setWorking] = useState<'' | 'reading' | 'asking'>('')
  const [error, setError] = useState<string | null>(null)
  const [answer, setAnswer] = useState<string | null>(null)
  const [draft, setDraft] = useState<TaskBody | null>(null)

  // The Broker holds nothing between calls, so each of these is one turn and
  // the whole of it. Both take minutes at worst, which is why the box says
  // which one it is waiting on rather than only that it is busy.
  const hand = async (what: 'reading' | 'asking') => {
    const said = text.trim()
    if (said === '' || working !== '') return
    setWorking(what)
    setError(null)
    setAnswer(null)
    try {
      if (what === 'reading') setDraft(await capture(said))
      else setAnswer(await ask(said, narrowing))
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
        lists={lists}
        tags={tags}
        colors={colors}
        snoozes={snoozes}
        action="Add"
        // The dump is done with once the Task is written. Backing out of the
        // sheet keeps it, because somebody who changed their mind about the
        // Task has not changed their mind about having typed the sentence.
        onSubmit={async (body) => {
          await addTask(body)
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
        placeholder="Say the task, or ask about the list"
        rows={2}
      />
      <div className="buttons">
        <button
          type="button"
          onClick={() => void hand('reading')}
          disabled={working !== '' || text.trim() === ''}
        >
          {working === 'reading' ? 'Reading…' : 'Add'}
        </button>
        <button
          type="button"
          onClick={() => void hand('asking')}
          disabled={working !== '' || text.trim() === ''}
        >
          {working === 'asking' ? 'Asking…' : 'Ask'}
        </button>
      </div>
      {error && <p className="message">{error}</p>}
      {answer && <p className="answer">{answer}</p>}
    </div>
  )
}
