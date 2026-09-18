// The wire shapes `GET /api/state` answers with, and the one call that reads
// it. They mirror src/api/state.go rather than store.Task: the API decides what
// a Task looks like on the wire, and this is the other side of that decision.

import { refused } from './api'

export type Collection = {
  id: string
  name: string
  color: string
  count: number
}

export type Task = {
  id: string
  parent?: string
  depth: number
  title: string
  description?: string
  why?: string
  color?: string
  createdAt: string
  deadline?: string
  snoozedUntil?: string
  completedAt?: string
  declinedAt?: string
  deletedAt?: string
  estimateSeconds?: number
  priority?: string
  impact?: string
  lists?: string[]
  tags?: string[]
  attachments?: string[]
  fields?: Record<string, string>
  series?: string
  // What the read worked out, in the store's own words. The client draws them
  // and does not recompute them: a Task that reads as snoozed here and plain in
  // a verb would be the same Task described two ways.
  marks: string[]
}

export type State = {
  tasks: Task[]
  lists: Collection[]
  tags: Collection[]
  // What `?sort=` accepts, in the order the store lists them. It is drawn as
  // it arrives: a sort added to `store.Sorts` is in the picker the day it
  // lands, and this client cannot offer one the store would refuse.
  sorts: string[]
}

/**
 * The three narrowings `GET /api/state` accepts, as the screen holds them.
 * `all` is the store's own word for it: one flag that takes in the snoozed,
 * the completed, the declined and the deleted together, because that is what
 * the route does with it rather than four switches this side pretends to.
 */
export type Narrowing = {
  all: boolean
  list: string
  sort: string
}

/** Narrowed by nothing, which is the everyday view and what the list opens on. */
export const WIDE: Narrowing = { all: false, list: '', sort: '' }

/**
 * A Narrowing as the query string both the list and the question carry.
 *
 * It is one function because it has to be one string: `POST /api/ask` reads
 * the query the same way `GET /api/state` does, so whatever narrows the list
 * narrows the question with it. Building the two separately is how they come
 * to disagree, and a question answered about a list nobody is looking at is
 * wrong in a way nothing on the screen would show.
 *
 * An empty value is left out rather than sent empty, so the everyday view is
 * the bare path and the ETag it is cached under does not change shape.
 */
export function queryString({ all, list, sort }: Narrowing): string {
  const query = new URLSearchParams()
  if (all) query.set('all', 'true')
  if (list) query.set('list', list)
  if (sort) query.set('sort', sort)
  const written = query.toString()
  return written ? `?${written}` : ''
}

export type Snapshot = {
  etag: string | null
  state: State
}

/**
 * Reads the whole screen in one request. The ETag is handed back on the next
 * call so an unchanged store answers 304 with no body, which is what makes
 * polling once a second cheap.
 *
 * A `null` return is "nothing changed", which is not the same as an empty
 * state and must not redraw as one.
 *
 * The ETag the API answers with is hashed over the query as well as the
 * store's log, so a narrowing changed with no write in between is a different
 * representation and is answered in full rather than 304.
 */
export async function fetchState(
  narrowing: Narrowing,
  etag: string | null,
  signal?: AbortSignal,
): Promise<Snapshot | null> {
  const response = await fetch(`/api/state${queryString(narrowing)}`, {
    headers: etag ? { 'If-None-Match': etag } : {},
    signal,
  })

  if (response.status === 304) return null

  // The API's error body is the sentence the CLI would have printed, so it is
  // shown as it is rather than restated in the client's own words.
  if (!response.ok) throw await refused(response)

  return {
    etag: response.headers.get('ETag'),
    state: (await response.json()) as State,
  }
}

/**
 * One appended fact, as `GET /api/history` and `GET /api/tasks/{id}/history`
 * answer it. The Actor is verbatim: the API recognises no model by name and
 * neither does this, which is why the splitting is `log.ts`'s and not a shape
 * the wire carries.
 */
export type Entry = {
  seq: number
  at: string
  actor: string
  kind: string
  subject: string
  payload?: unknown
}

/** What the Change History holds about one Task, newest first. */
export async function fetchTaskHistory(id: string): Promise<Entry[]> {
  return await read(`/api/tasks/${encodeURIComponent(id)}/history`)
}

/**
 * The most entries `GET /api/history` will answer, whatever is asked for. It is
 * `api.historyLimit`, and this is the client's copy of it: nothing on the wire
 * says where the route stops, so asking past it would be a screen offering more
 * and then not producing any. Raising it there means raising it here.
 */
export const HISTORY_CAP = 1000

/**
 * The same rows across every Task, newest first and a page at a time. The page
 * is generous because the screen drops the Lease bookkeeping out of it, and a
 * page counted before that happens is mostly plumbing.
 */
export async function fetchHistory(limit = 200): Promise<Entry[]> {
  return await read(`/api/history?limit=${Math.min(limit, HISTORY_CAP)}`)
}

async function read(path: string): Promise<Entry[]> {
  const response = await fetch(path)
  if (!response.ok) throw await refused(response)
  const body = (await response.json()) as { entries: Entry[] }
  return body.entries
}

/**
 * A Task's Series: the rule as the store wrote it back, and the dates it
 * produces next with what anybody has done to each. `apps/todo/src/api/series.go`
 * is the side that decides the shape.
 *
 * A Task that does not repeat answers `repeats: false` and no dates, which is
 * most of them and is not an error.
 */
export type Series = {
  repeats: boolean
  rule?: string
  occurrences: Occurrence[]
}

/** One date of a Series, and the store's own word for what was done to it. */
export type Occurrence = {
  date: string
  state?: string
}

/**
 * Reads one Task's Series. The dates are computed as they are answered, so
 * asking again on every revision costs the log nothing.
 */
export async function fetchSeries(id: string): Promise<Series> {
  const response = await fetch(`/api/tasks/${encodeURIComponent(id)}/series`)
  if (!response.ok) throw await refused(response)
  return (await response.json()) as Series
}
