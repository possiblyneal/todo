// The activity screen: what agents did. The Change History across every Task,
// newest first, with a filter for the writes whose Actor names a harness and a
// model.
//
// It is a read of rows that already exist: it takes no Lease and appends
// nothing. The filter narrows and never hides, so the screen opens unfiltered
// and an Agent that named itself with no slash is in that view.

import { useEffect, useState } from 'react'

import { isAgent, isWrite } from './log'
import { Log } from './Log'
import { fetchHistory, type Entry, type Task } from './state'

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
  const [entries, setEntries] = useState<Entry[]>([])
  const [error, setError] = useState<string | null>(null)
  const [agentsOnly, setAgentsOnly] = useState(false)

  useEffect(() => {
    let live = true
    fetchHistory()
      .then((read) => {
        if (live) setEntries(read)
      })
      .catch((caught: unknown) => {
        if (live)
          setError(caught instanceof Error ? caught.message : String(caught))
      })
    return () => {
      live = false
    }
  }, [revision])

  // The Lease bookkeeping goes first and always: it brackets every guarded
  // write under the writer's own Actor, so leaving it in would make the screen
  // two thirds plumbing whichever way the filter is set.
  const shown = entries
    .filter(isWrite)
    .filter((entry) => !agentsOnly || isAgent(entry.actor))

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
    </div>
  )
}
