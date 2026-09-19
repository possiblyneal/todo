import { afterEach, expect, test, vi } from 'vitest'

import { fetchSeries, fetchState, queryString, WIDE } from './state'

const empty: string[] = []

// What the last call asked for, which is how the conditional request is
// checked: the ETag only earns its keep if it is actually sent back.
let asked: RequestInit | undefined
// Where it asked, which is how the narrowing is checked: the query string is
// the whole of what tells the API which Tasks the screen wants.
let at: string | undefined

function answering(
  status: number,
  body: unknown,
  headers: Record<string, string> = {},
) {
  return (url: string, init?: RequestInit) => {
    asked = init
    at = url
    return Promise.resolve(
      new Response(status === 304 ? null : JSON.stringify(body), {
        status,
        headers,
      }),
    )
  }
}

afterEach(() => {
  asked = undefined
  at = undefined
  vi.unstubAllGlobals()
})

test('a read returns the tree and the tag it came with', async () => {
  vi.stubGlobal(
    'fetch',
    answering(
      200,
      {
        tasks: [
          { id: 't1', depth: 1, title: 'Ship it', createdAt: '', marks: empty },
        ],
        lists: empty,
        tags: empty,
      },
      { ETag: '"abc"' },
    ),
  )

  const snapshot = await fetchState(WIDE, null)

  expect(snapshot?.etag).toBe('"abc"')
  expect(snapshot?.state.tasks[0]?.title).toBe('Ship it')
  expect(asked?.headers).toEqual({})
})

test('an unchanged store is nothing to redraw rather than an empty one', async () => {
  vi.stubGlobal('fetch', answering(304, null))

  expect(await fetchState(WIDE, '"abc"')).toBeNull()
  expect(asked?.headers).toEqual({ 'If-None-Match': '"abc"' })
})

test('a refusal surfaces the sentence the CLI would have printed', async () => {
  const sentence = 'usage: sort is one of [title deadline], not "nope"'
  vi.stubGlobal('fetch', answering(400, { error: sentence }))

  await expect(fetchState(WIDE, null)).rejects.toThrow(sentence)
})

test('an error with no sentence in it still says what happened', async () => {
  vi.stubGlobal('fetch', () =>
    Promise.resolve(new Response('nope', { status: 500 })),
  )

  await expect(fetchState(WIDE, null)).rejects.toThrow('the API answered 500')
})

test('a task that does not repeat is not an error', async () => {
  vi.stubGlobal(
    'fetch',
    answering(200, { repeats: false, occurrences: [] }, {}),
  )

  const series = await fetchSeries('task_abc')

  expect(series.repeats).toBe(false)
  expect(series.occurrences).toEqual([])
})

test('a series is the rule and the dates it produces', async () => {
  vi.stubGlobal(
    'fetch',
    answering(
      200,
      {
        repeats: true,
        rule: 'every week on mon,thu from 2026-09-16',
        occurrences: [
          { date: '2026-09-17', state: 'ticked' },
          { date: '2026-09-21' },
        ],
      },
      {},
    ),
  )

  const series = await fetchSeries('task_abc')

  // The store's own word for what was done to a date, drawn as it arrived: a
  // date nobody has touched carries none.
  expect(series.occurrences[0]?.state).toBe('ticked')
  expect(series.occurrences[1]?.state).toBeUndefined()
  expect(series.rule).toBe('every week on mon,thu from 2026-09-16')
})

// The everyday view is the bare path. A query string of empty values would be
// a second spelling of the same representation, and the ETag is hashed over
// the query, so the two would never share a cached answer.
test('narrowed by nothing is the path and no query at all', () => {
  expect(queryString(WIDE)).toBe('')
})

test('each narrowing is sent under the name the route reads it by', () => {
  expect(
    queryString({
      all: true,
      list: 'l1',
      tags: ['t1'],
      search: 'paint',
      sort: 'deadline',
    }),
  ).toBe('?all=true&list=l1&tag=t1&search=paint&sort=deadline')
})

// Repeated and not joined. A separator would be one this side invented, and
// the route reads `?tag=` the way a query string already means several values.
test('several tags are sent as the parameter repeated', () => {
  expect(queryString({ ...WIDE, tags: ['t1', 't2'] })).toBe('?tag=t1&tag=t2')
})

// `all` is one flag over four states, because that is what the route does with
// it. Sending `all=false` would be asking for something the route has no word
// for.
test('showing only the everyday tasks says nothing rather than false', () => {
  expect(queryString({ ...WIDE, all: false })).toBe('')
})

test('a list id that needs escaping is escaped', () => {
  expect(queryString({ ...WIDE, list: 'a b&c' })).toBe('?list=a+b%26c')
})

// The store is what matches the text, so whatever was typed goes out as typed.
// Trimming or splitting it here would be this side deciding what a search means
// and then disagreeing with `todo list -search`.
test('searched text is sent as it was typed', () => {
  expect(queryString({ ...WIDE, search: '50% off' })).toBe('?search=50%25+off')
})

test('the read asks under the narrowing it was given', async () => {
  vi.stubGlobal(
    'fetch',
    answering(200, { tasks: [], lists: [], tags: [], sorts: [] }),
  )

  await fetchState({ ...WIDE, all: true, list: 'l1', sort: 'title' }, null)

  expect(at).toBe('/api/state?all=true&list=l1&sort=title')
})

// The sorts are drawn as they arrive and never invented here, so an empty
// answer is an empty picker rather than a default set this side made up.
test('the sorts on offer are the ones the read named', async () => {
  vi.stubGlobal(
    'fetch',
    answering(200, {
      tasks: [],
      lists: [],
      tags: [],
      sorts: ['title', 'deadline', 'created', 'estimate'],
    }),
  )

  const snapshot = await fetchState(WIDE, null)

  expect(snapshot?.state.sorts).toEqual([
    'title',
    'deadline',
    'created',
    'estimate',
  ])
})
