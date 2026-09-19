// The detail screen: what a tap on a Task opens. Everything the Task carries,
// its Subtasks, its Series if it has one, the breakdown that proposes more
// Subtasks, the four verbs that end or reopen it, and its history.
//
// Nothing here is worked out that the read already did: the marks are drawn as
// they arrived and the history is drawn as the API answered it.

import { useState } from 'react'

import { sentence } from './api'
import { Attributes } from './Attributes'
import { Breakdown } from './Breakdown'
import { Log } from './Log'
import { useRead } from './read'
import { Series } from './Series'
import { Sheet } from './Sheet'
import { Was } from './Was'
import { fetchTaskHistory, type Entry, type Offered, type Task } from './state'
import {
  addSubtask,
  attach,
  detach,
  draftOf,
  editTask,
  lifecycle,
  VERBS,
  type TaskBody,
} from './write'

const NONE: Entry[] = []

export function Detail({
  task,
  subtasks,
  offered,
  revision,
  onOpen,
  onBack,
}: {
  task: Task
  /** The Tasks directly under this one, which the list read already returned. */
  subtasks: Task[]
  /**
   * What the sheet this screen opens picks from. The Lists and Tags are read
   * here as well, to name the ones the Task carries; the rest goes straight
   * past to the sheet.
   */
  offered: Offered
  /**
   * The ETag of the read on the screen. The history is fetched again when it
   * changes, so a write made here or made by an Agent elsewhere shows up on the
   * log without this screen polling on a clock of its own.
   */
  revision: string | null
  onOpen: (id: string) => void
  onBack: () => void
}) {
  // Which screen this one is standing in front of. The Series and the
  // breakdown are screens rather than sections because a thumb reaching a mark
  // should not have scrolled past everything the Task carries to get there.
  const [open, setOpen] = useState<
    '' | 'edit' | 'subtask' | 'series' | 'breakdown'
  >('')
  const [working, setWorking] = useState(false)
  // The entry being read out as the Task it left behind, or nothing. A screen
  // belongs to the Task it was opened on, and this is that Task at one of its
  // own positions rather than a second screen about another one.
  const [opened, setOpened] = useState<Entry | null>(null)
  // What a verb was told, kept apart from what the read was told: a refusal is
  // about the write somebody just made and stays on the screen until they make
  // another, where a failed read is over as soon as one comes back.
  const [refused, setRefused] = useState<string | null>(null)
  const { value: entries, error: unread } = useRead(
    () => fetchTaskHistory(task.id),
    NONE,
    [task.id, revision],
  )

  const act = async (verb: string) => {
    setWorking(true)
    setRefused(null)
    try {
      await lifecycle(task.id, verb)
      // The verb landed, so this screen is drawing a Task the next read
      // describes differently, and three of the four take it out of the list
      // the poll asks for altogether. The list is where it says what happened
      // rather than this screen redrawing itself from what it still holds.
      onBack()
    } catch (caught) {
      setRefused(sentence(caught))
      setWorking(false)
    }
  }

  // A pointer added or taken off. Unlike the four verbs, the Task is still here
  // afterwards and this screen is still the one to be on, so it stays put and
  // the next poll is what redraws the list.
  //
  // It answers whether the write landed, which is what lets the field keep what
  // was typed through a refusal: nothing was written, so re-tapping is the
  // right thing to do and retyping the pointer is the whole cost of the feature
  // on a phone.
  const point = async (made: Promise<void>) => {
    setWorking(true)
    setRefused(null)
    try {
      await made
      return true
    } catch (caught) {
      setRefused(sentence(caught))
      return false
    } finally {
      setWorking(false)
    }
  }

  if (open === 'series') {
    return (
      <Series
        task={task}
        // The Series screen opens the same sheet this one does, for the date
        // being lifted out, so it picks from the same sets.
        offered={offered}
        revision={revision}
        // A detached date is an ordinary Task now, and opening it is the only
        // thing that names it: nothing else afterwards says where it went.
        onOpen={onOpen}
        onBack={() => setOpen('')}
      />
    )
  }

  if (open === 'breakdown') {
    return <Breakdown task={task} onBack={() => setOpen('')} />
  }

  if (open !== '') {
    return (
      <Sheet
        draft={open === 'edit' ? draftOf(task) : {}}
        offered={offered}
        action={open === 'edit' ? 'Save' : 'Add'}
        onSubmit={async (body: TaskBody) => {
          if (open === 'edit') await editTask(task.id, body)
          else await addSubtask(task.id, body)
          setOpen('')
        }}
        onCancel={() => setOpen('')}
      />
    )
  }

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
        <button type="button" onClick={() => setOpen('edit')}>
          Edit
        </button>
        <button type="button" onClick={() => setOpen('subtask')}>
          Subtask
        </button>
        <button type="button" onClick={() => setOpen('series')}>
          Series
        </button>
        <button type="button" onClick={() => setOpen('breakdown')}>
          Break down
        </button>
      </div>

      <h1 className="title">{task.title}</h1>
      {task.marks.length > 0 && (
        <p className="marks">{task.marks.join(' · ')}</p>
      )}
      {(refused ?? unread) && <p className="message">{refused ?? unread}</p>}

      <Attributes task={task} offered={offered} />

      <Attachments
        on={task.attachments ?? []}
        working={working}
        onAttach={(target) => point(attach(task.id, target))}
        onDetach={(target) => point(detach(task.id, target))}
      />

      {/*
        The four are buttons whatever state the Task is in. Which of them the
        store refuses is the store's to say, and it says it in a sentence: a
        screen that greyed out the wrong one would be the second copy of a rule
        that already exists.
      */}
      <div className="buttons">
        {VERBS.map((verb) => (
          <button
            key={verb}
            type="button"
            onClick={() => void act(verb)}
            disabled={working}
          >
            {verb}
          </button>
        ))}
      </div>

      <h2 className="heading">Subtasks</h2>
      {subtasks.length === 0 && <p className="message">None.</p>}
      {subtasks.length > 0 && (
        <ul className="list">
          {subtasks.map((one) => (
            <li key={one.id}>
              <button
                type="button"
                className="row"
                onClick={() => onOpen(one.id)}
              >
                <span>{one.title}</span>
                {one.marks.length > 0 && (
                  <span className="marks">{one.marks.join(' · ')}</span>
                )}
              </button>
            </li>
          ))}
        </ul>
      )}

      <h2 className="heading">History</h2>
      {/*
        Every entry about this Task opens it as that entry left it, so "what
        did this say before that edit" is a question the screen answers rather
        than one a reader works out from the kinds. Which kinds are about a
        Task at all is the store's to say, and it says it in `opens`.
      */}
      <Log entries={entries} opens={offered.opens} onOpen={setOpened} />
    </div>
  )
}

/**
 * The pointers a Task holds, each with the tap that takes it off, and the row
 * that adds one.
 *
 * An Attachment is text and nothing else: nothing is uploaded here and nothing
 * is fetched, so a pointer naming a file names it on the machine `todo api`
 * runs on rather than on the phone it was typed into.
 */
function Attachments({
  on,
  working,
  onAttach,
  onDetach,
}: {
  on: string[]
  working: boolean
  /** Answers whether the write landed, which is what clears the field. */
  onAttach: (target: string) => Promise<boolean>
  onDetach: (target: string) => void
}) {
  const [target, setTarget] = useState('')

  return (
    <>
      <h2 className="heading">Attachments</h2>
      {on.length === 0 && <p className="message">None.</p>}
      <ul className="list">
        {on.map((pointer) => (
          <li className="row" key={pointer}>
            {/*
              A web address is followable and a path is not, so the pointer is
              drawn as text either way rather than as a link this screen decides
              the shape of. The store never looked at what one names and neither
              does this.
            */}
            <span>{pointer}</span>
            <button
              type="button"
              disabled={working}
              onClick={() => onDetach(pointer)}
            >
              Remove
            </button>
          </li>
        ))}
      </ul>
      <div className="row">
        <input
          className="control"
          aria-label="New attachment"
          placeholder="https://… or /a/path"
          value={target}
          onChange={(event) => setTarget(event.target.value)}
        />
        <button
          type="button"
          disabled={working || target.trim() === ''}
          onClick={async () => {
            // Cleared on the write landing and not before. A refusal wrote
            // nothing, so the pointer has to still be here to tap again;
            // clearing it either way would make a refused attach cost the
            // whole target retyped, and disable the button that retries.
            if (await onAttach(target)) setTarget('')
          }}
        >
          Attach
        </button>
      </div>
    </>
  )
}
