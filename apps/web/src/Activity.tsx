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
import {
  fetchHistory,
  HISTORY_CAP,
  type Entry,
  type Offered,
  type Task,
} from './state'
import { Was } from './Was'

/** The first page, and what each tap on Show more adds to it. */
const PAGE = 200

const NONE: Entry[] = []

export function Activity({
  tasks,
  offered,
  revision,
  onBack,
}: {
  /** What the read named, for putting a title to the Task an entry is about. */
  tasks: Task[]
  /** The Lists and Tags, for drawing a Task read at one of its positions. */
  offered: Offered
  revision: string | null
  onBack: () => void
}) {
  const [agentsOnly, setAgentsOnly] = useState(false)
  // What the log is searched for. It goes to the route rather than narrowing
  // what is already here: this screen reaches further back by asking for more,
  // so a match made on the page in hand could only find what was recent enough
  // to have arrived. A Task deleted a month ago is the thing somebody comes
  // here to find.
  const [search, setSearch] = useState('')
  // The entry being read out as the Task it was about, or nothing. It is kept
  // whole rather than as a seq: the screen draws what happened above the Task,
  // and the entry is what says that.
  const [opened, setOpened] = useState<Entry | null>(null)
  // How far back this screen is asking. The route answers the newest first, so
  // a bigger number is the same rows with older ones under them: reaching
  // further back is one read rather than a second one stitched onto the first,
  // which is what keeps a write that landed in between from being drawn twice.
  const [limit, setLimit] = useState(PAGE)
  const { value: entries, error } = useRead(
    () => fetchHistory(limit, search),
    NONE,
    [revision, limit, search],
  )

  // The Lease bookkeeping goes first and always: it brackets every guarded
  // write under the writer's own Actor, so leaving it in would make the screen
  // two thirds plumbing whichever way the filter is set.
  const shown = entries
    .filter(isWrite)
    .filter((entry) => !agentsOnly || isAgent(entry.actor))

  // A full page is the only thing that says there may be more. The count is of
  // what the route answered rather than of what is drawn, because the filters
  // above take rows out of a page that was already read. At the route's cap
  // there is no more to ask for, so the button goes rather than asking again
  // for the same thousand rows.
  const more = entries.length === limit && limit < HISTORY_CAP

  if (opened) {
    return (
      <Was entry={opened} offered={offered} onBack={() => setOpened(null)} />
    )
  }

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

      {/*
        Typing asks again, with no timer in between, which is the rule the
        search over the list already follows. The store is what matches, so
        this sends the text and nothing else.
      */}
      <input
        className="control"
        type="search"
        aria-label="Search the history"
        placeholder="Search the history"
        value={search}
        onChange={(event) => setSearch(event.target.value)}
      />

      {error && <p className="message">{error}</p>}
      <Log
        entries={shown}
        nameOf={(subject) =>
          tasks.find((task) => task.id === subject)?.title ?? subject
        }
        onOpen={setOpened}
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
