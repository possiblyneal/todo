// A Task as one entry in the Change History left it.
//
// This is where a deleted Task is looked at. No list read offers one any more:
// a deletion is not an ending somebody puts aside, so it does not belong among
// the snoozed, the completed and the declined that "Show everything" takes in.
// The entry that deleted it is where what it said is still written down, and
// this is that entry read out.
//
// It answers about any entry and not only a deletion, so "what did this say
// before that edit" is a question with an answer too.
//
// Nothing here writes. A Task at a position is not a Task to act on: it is the
// record of one, and the only way back to acting on it is the Task itself,
// where it still exists.

import { what, who } from './log'
import { Attributes } from './Attributes'
import { useRead } from './read'
import { fetchTaskAsOf, type Entry, type Offered, type Task } from './state'

export function Was({
  entry,
  offered,
  onBack,
}: {
  entry: Entry
  offered: Offered
  onBack: () => void
}) {
  const { value: task, error } = useRead<Task | null>(
    () => fetchTaskAsOf(entry.subject, entry.seq),
    null,
    [entry.subject, entry.seq],
  )
  const said = who(entry.actor)

  return (
    <div className="detail">
      <div className="buttons">
        <button type="button" onClick={onBack}>
          Back
        </button>
      </div>

      {/*
        What happened, before what it happened to. The screen was opened from
        an entry rather than from a Task, and the entry is what the reader
        picked out: leading with the Task would leave them working out which of
        its positions they are looking at.
      */}
      <p className="marks">
        {what(entry.kind)} · {said.name}
        {said.harness && ` ${said.harness}`} ·{' '}
        <time dateTime={entry.at}>{local(entry.at)}</time>
      </p>

      {error && <p className="message">{error}</p>}
      {!task && !error && <p className="message">Reading the Task…</p>}
      {task && (
        <>
          <h1 className="title">{task.title}</h1>
          {task.marks.length > 0 && (
            <p className="marks">{task.marks.join(' · ')}</p>
          )}
          <Attributes task={task} offered={offered} />
        </>
      )}
    </div>
  )
}

/** The wire carries UTC, and a reader reads their own time zone. */
function local(at: string): string {
  const when = new Date(at)
  return isNaN(when.getTime()) ? at : when.toLocaleString()
}
