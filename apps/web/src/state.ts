// The wire shapes `GET /api/state` answers with, and the one call that reads
// it. They mirror src/api/state.go rather than store.Task: the API decides what
// a Task looks like on the wire, and this is the other side of that decision.

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
}

export type Snapshot = {
  etag: string | null
  state: State
}

/**
 * Reads the whole screen in one request. The ETag is handed back on the next
 * call so an unchanged store answers 304 with no body, which is what makes
 * polling once a second cheap -- the same thing the TUI did against the
 * write-ahead log.
 *
 * A `null` return is "nothing changed", which is not the same as an empty
 * state and must not redraw as one.
 */
export async function fetchState(
  etag: string | null,
  signal?: AbortSignal,
): Promise<Snapshot | null> {
  const response = await fetch('/api/state', {
    headers: etag ? { 'If-None-Match': etag } : {},
    signal,
  })

  if (response.status === 304) return null

  if (!response.ok) {
    // The API's error body is the sentence the CLI would have printed, so it
    // is shown as it is rather than restated in the client's own words.
    const body: unknown = await response.json().catch(() => null)
    const sentence =
      body &&
      typeof body === 'object' &&
      'error' in body &&
      typeof body.error === 'string'
        ? body.error
        : `the API answered ${response.status}`
    throw new Error(sentence)
  }

  return {
    etag: response.headers.get('ETag'),
    state: (await response.json()) as State,
  }
}
