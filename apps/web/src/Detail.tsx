// The detail screen: what a tap on a Task opens. Everything the Task carries,
// its Subtasks, its Series if it has one, the breakdown that proposes more
// Subtasks, the four verbs that end or reopen it, and its history.
//
// Nothing here is worked out that the read already did: the marks are drawn as
// they arrived and the history is drawn as the API answered it.

import { useState } from 'react'

import { sentence } from './api'
import { Breakdown } from './Breakdown'
import { Log } from './Log'
import { useRead } from './read'
import { Series } from './Series'
import { Sheet } from './Sheet'
import {
  fetchTaskHistory,
  type Collection,
  type Entry,
  type Task,
} from './state'
import {
  addSubtask,
  draftOf,
  editTask,
  estimate,
  lifecycle,
  VERBS,
  type TaskBody,
} from './write'

const NONE: Entry[] = []

export function Detail({
  task,
  subtasks,
  lists,
  tags,
  colors,
  snoozes,
  revision,
  onOpen,
  onBack,
}: {
  task: Task
  /** The Tasks directly under this one, which the list read already returned. */
  subtasks: Task[]
  lists: Collection[]
  tags: Collection[]
  colors: string[]
  snoozes: string[]
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

  if (open === 'series') {
    return (
      <Series
        task={task}
        // The Series screen opens the same sheet this one does, for the date
        // being lifted out, so it needs the same two to tick memberships with.
        lists={lists}
        tags={tags}
        colors={colors}
        snoozes={snoozes}
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
        lists={lists}
        tags={tags}
        colors={colors}
        snoozes={snoozes}
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

      <dl className="carried">
        <Carried name="Description" value={task.description} />
        <Carried name="Why" value={task.why} />
        <Carried name="Deadline" value={task.deadline} />
        <Carried name="Estimate" value={estimate(task.estimateSeconds)} />
        <Carried name="Priority" value={task.priority} />
        <Carried name="Impact" value={task.impact} />
        <Carried name="Color" value={task.color} />
        <Carried name="Lists" value={named(task.lists, lists)} />
        <Carried name="Tags" value={named(task.tags, tags)} />
        <Carried name="Series" value={task.series} />
        <Carried name="Created" value={task.createdAt} />
        {Object.entries(task.fields ?? {}).map(([name, value]) => (
          <Carried key={name} name={name} value={value} />
        ))}
        {(task.attachments ?? []).map((pointer) => (
          <Carried key={pointer} name="Attachment" value={pointer} />
        ))}
      </dl>

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
      <Log entries={entries} />
    </div>
  )
}

/** One attribute, drawn only where the Task carries one. */
function Carried({ name, value }: { name: string; value?: string }) {
  if (!value) return null
  return (
    <>
      <dt>{name}</dt>
      <dd>{value}</dd>
    </>
  )
}

/**
 * The Lists or Tags a Task carries, by name where the read named them. An id
 * the read did not name is drawn as the id rather than dropped, for the same
 * reason the sheet ticks one: a membership nobody can see is one nobody can
 * take off.
 */
function named(ids: string[] | undefined, all: Collection[]): string {
  return (ids ?? [])
    .map((id) => all.find((one) => one.id === id)?.name ?? id)
    .join(', ')
}
