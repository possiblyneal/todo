// The wire shapes the write routes take and the two Broker calls, and the
// calls that reach them. They mirror `apps/todo/src/api/tasks.go` and
// `apps/todo/src/api/broker.go`, which are the side that decides them.

import { send } from './api'

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
