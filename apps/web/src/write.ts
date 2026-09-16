// The wire shapes the write routes take and the two Broker calls, and the
// calls that reach them. They mirror `apps/todo/src/api/tasks.go` and
// `apps/todo/src/api/broker.go`, which are the side that decides them.

import { send } from './api'
import type { Task } from './state'

/**
 * A Task's attributes and its memberships as they are sent, and the same shape
 * `POST /api/capture` answers in: what the Broker read comes back as the body
 * this submits once somebody has corrected it.
 *
 * An absent attribute is left alone and an empty one is cleared, which is the
 * same rule a typed `-title ""` follows. Lists and Tags are the ids
 * `GET /api/state` already named.
 */
export type TaskBody = {
  title?: string
  description?: string
  why?: string
  deadline?: string
  estimate?: string
  priority?: string
  impact?: string
  parent?: string
  intoLists?: string[]
  outOfLists?: string[]
  addTags?: string[]
  dropTags?: string[]
}

/**
 * Hands a dump to the Broker and gets back the Task it read, filled into the
 * body the add sheet submits. It writes nothing: a dump read and then
 * abandoned leaves nothing behind, so the sheet is the gate.
 */
export function capture(text: string): Promise<TaskBody> {
  return send<TaskBody>('POST', '/api/capture', { text })
}

/**
 * Asks about the Tasks in view and gets prose back. It is a read like the list
 * it is about: nothing is appended and nothing is kept between calls.
 */
export async function ask(question: string): Promise<string> {
  const said = await send<{ answer: string }>('POST', '/api/ask', { question })
  return said.answer
}

/** Writes one Task and files it, and names the Task it wrote. */
export async function addTask(body: TaskBody): Promise<string> {
  const written = await send<{ id: string }>('POST', '/api/tasks', body)
  return written.id
}

/** Changes a Task's attributes and its memberships together, as one write. */
export async function editTask(id: string, body: TaskBody): Promise<void> {
  await send<{ id: string }>(
    'PATCH',
    `/api/tasks/${encodeURIComponent(id)}`,
    body,
  )
}

/** Writes one Task under another, which is the same write with a parent. */
export async function addSubtask(
  parent: string,
  body: TaskBody,
): Promise<string> {
  const written = await send<{ id: string }>(
    'POST',
    `/api/tasks/${encodeURIComponent(parent)}/subtasks`,
    body,
  )
  return written.id
}

/**
 * The rest of a Task's life: complete, decline, reopen, delete. The four are
 * one route and the API holds the list, so a verb this sends that a Task does
 * not do comes back refused in the API's own words rather than being checked
 * twice.
 */
export async function lifecycle(id: string, verb: string): Promise<void> {
  await send<{ id: string }>(
    'POST',
    `/api/tasks/${encodeURIComponent(id)}/${verb}`,
    {},
  )
}

/**
 * A Task read back as the body that edits it. What the sheet shows is text, so
 * a deadline goes back the way it came -- RFC 3339, which `write.Deadline`
 * reads -- rather than being reformatted into something this side decided on.
 *
 * The memberships are what the Task carries now, which is what makes unticking
 * one mean something: see `memberships` below.
 */
export function draftOf(task: Task): TaskBody {
  return {
    title: task.title,
    description: task.description,
    why: task.why,
    deadline: task.deadline,
    estimate: estimate(task.estimateSeconds),
    priority: task.priority,
    impact: task.impact,
    intoLists: task.lists ?? [],
    addTags: task.tags ?? [],
  }
}

/**
 * A duration the API can read back, and the shortest of them: the wire carries
 * seconds because JavaScript has no duration, and `90m` is what somebody typed
 * in the first place.
 */
function estimate(seconds?: number): string | undefined {
  if (!seconds) return undefined
  if (seconds % 3600 === 0) return `${seconds / 3600}h`
  if (seconds % 60 === 0) return `${seconds / 60}m`
  return `${seconds}s`
}

/**
 * The Lists and Tags one write joins and leaves, worked out from what the
 * Task carried when the sheet opened and what is ticked on it now.
 *
 * Ticking and unticking are different fields on the wire, so the difference has
 * to be taken somewhere: taking it here means a sheet opened on a Task and a
 * sheet opened on a draft submit the same body, and an untick is a membership
 * dropped rather than one silently left on.
 */
export function memberships(before: TaskBody, after: TaskBody): TaskBody {
  const on = (body: TaskBody, name: 'intoLists' | 'addTags') => body[name] ?? []
  const missing = (from: string[], against: string[]) =>
    from.filter((id) => !against.includes(id))
  return {
    intoLists: missing(on(after, 'intoLists'), on(before, 'intoLists')),
    outOfLists: missing(on(before, 'intoLists'), on(after, 'intoLists')),
    addTags: missing(on(after, 'addTags'), on(before, 'addTags')),
    dropTags: missing(on(before, 'addTags'), on(after, 'addTags')),
  }
}
