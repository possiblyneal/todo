// The activity screen: what agents did. The Change History across every Task,
// newest first, with a filter for the writes whose Actor names a harness and a
// model.
//
// It is a read of rows that already exist: it takes no Lease and appends
// nothing. The filter narrows and never hides, so the screen opens unfiltered
// and an Agent that named itself with no slash is in that view.

import { useState } from 'react'

import { isAgent, isWrite } from './log'
import { Log } from './Log'
import { useRead } from './read'
import { fetchHistory, type Entry, type Task } from './state'

/** The first page, and what each tap on Show more adds to it. */
const PAGE = 200

const NONE: Entry[] = []

export function Activity({
  tasks,
  revision,
  onBack,
}: {
  /** What the read named, for putting a title to the Task an entry is about. */
  tasks: Task[]
  revision: string | null
  onBack: () => void
}) {
  const [agentsOnly, setAgentsOnly] = useState(false)
  // How far back this screen is asking. The route answers the newest first, so
  // a bigger number is the same rows with older ones under them: reaching
  // further back is one read rather than a second one stitched onto the first,
  // which is what keeps a write that landed in between from being drawn twice.
  const [limit, setLimit] = useState(PAGE)
  const { value: entries, error } = useRead(() => fetchHistory(limit), NONE, [
    revision,
    limit,
  ])

  // The Lease bookkeeping goes first and always: it brackets every guarded
  // write under the writer's own Actor, so leaving it in would make the screen
  // two thirds plumbing whichever way the filter is set.
  const shown = entries
    .filter(isWrite)
    .filter((entry) => !agentsOnly || isAgent(entry.actor))

  // A full page is the only thing that says there may be more. The count is of
  // what the route answered rather than of what is drawn, because the filters
  // above take rows out of a page that was already read.
  const more = entries.length === limit

  return (
    <div className="detail">
      <div className="buttons">
        <button type="button" onClick={onBack}>
          Back
        </button>
        <button type="button" onClick={() => setAgentsOnly(!agentsOnly)}>
          {agentsOnly ? 'Everyone' : 'Agents'}
        </button>
      </div>

      {error && <p className="message">{error}</p>}
      <Log
        entries={shown}
        nameOf={(subject) =>
          tasks.find((task) => task.id === subject)?.title ?? subject
        }
      />
      {more && (
        <div className="buttons">
          <button type="button" onClick={() => setLimit(limit + PAGE)}>
            Show more
          </button>
        </div>
      )}
    </div>
  )
}
