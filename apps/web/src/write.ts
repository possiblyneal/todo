// The wire shapes the write routes take and the two Broker calls, and the
// calls that reach them. They mirror `apps/todo/src/api/tasks.go` and
// `apps/todo/src/api/broker.go`, which are the side that decides them.

import { send } from './api'
import { type Narrowing, queryString, type Task } from './state'

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
 *
 * "In view" is the narrowing the list is drawn under, sent as the same query
 * string the poll carries and read by the route the same way. A question asked
 * without it would be answered about every open Task while the screen shows a
 * sorted, narrowed few, and nothing on the screen would say so.
 */
export async function ask(
  question: string,
  narrowing: Narrowing,
): Promise<string> {
  const said = await send<{ answer: string }>(
    'POST',
    `/api/ask${queryString(narrowing)}`,
    { question },
  )
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
 * The four, and the client's one copy of which four there are. No route
 * answers the question, so this is the list every screen that draws them reads
 * rather than each keeping its own. A fifth added to `write.Lifecycle` is a
 * button missing here until it is added, never a sentence drawn wrongly: one
 * this sends that the API does not serve comes back refused in its own words.
 */
export const VERBS = ['complete', 'decline', 'reopen', 'delete']

/** The rest of a Task's life, by the verb in the path. */
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
 * in the first place. It is what the detail screen draws too, so an estimate
 * reads the same on the screen that shows it and in the field that edits it.
 */
export function estimate(seconds?: number): string | undefined {
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

/**
 * The rule a Task repeats on, set or replaced. A Series is one value edited as
 * one thing, which is why this sends the whole rule rather than part of one.
 * A rule the parser cannot read comes back refused in the parser's own words,
 * with what was typed still in the field.
 */
export async function repeat(id: string, rule: string): Promise<void> {
  await send<{ id: string }>(
    'PUT',
    `/api/tasks/${encodeURIComponent(id)}/series`,
    { rule },
  )
}

/**
 * The Task stops repeating. It erases nothing despite the method: the Series
 * and every mark on its dates stay in the record.
 */
export async function unrepeat(id: string): Promise<void> {
  await send<{ id: string }>(
    'DELETE',
    `/api/tasks/${encodeURIComponent(id)}/series`,
    {},
  )
}

/**
 * The three, and the client's copy of which three there are, for the same
 * reason `VERBS` is one: no route answers the question. A fourth added to
 * `write.Mark` is a button missing here until it is added, never a sentence
 * drawn wrongly.
 */
export const MARKS = ['tick', 'skip', 'detach']

/**
 * One date ticked, skipped or lifted out, and the id that names what happened:
 * detaching answers the Task the date became, and the other two answer the
 * Task the mark was against.
 */
export async function mark(
  id: string,
  which: string,
  on: string,
): Promise<string> {
  const written = await send<{ id: string }>(
    'POST',
    `/api/tasks/${encodeURIComponent(id)}/series/${which}`,
    { on },
  )
  return written.id
}

/** One question the Broker asked and the answer it was given back. */
export type QA = { question: string; answer: string }

/**
 * One turn coming back: what the Broker still needs to know, or what it
 * proposes. The two are alternatives.
 */
export type Step = {
  questions?: string[]
  proposals: TaskBody[]
}

/**
 * One turn of a breakdown. It writes nothing and holds no Lease: the Broker
 * keeps nothing between calls, so every turn carries everything already
 * answered, and an approved proposal is written afterwards by `addSubtask`
 * like any other Subtask.
 */
export function breakdown(task: string, answers: QA[]): Promise<Step> {
  return send<Step>('POST', '/api/breakdown', { task, answers })
}

/**
 * One date lifted out as the Task it was corrected into, which is the fourth
 * thing done to a date and one write rather than a detach and then an edit of
 * what it became. It answers the Task the date became.
 *
 * `POST .../series/edit` is its own route rather than a fourth `MARKS` name,
 * because it carries a whole Task where the three carry only the date.
 */
export async function detachEdited(
  id: string,
  on: string,
  body: TaskBody,
): Promise<string> {
  const written = await send<{ id: string }>(
    'POST',
    `/api/tasks/${encodeURIComponent(id)}/series/edit`,
    { ...body, on },
  )
  return written.id
}
