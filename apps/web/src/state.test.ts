import { afterEach, expect, test, vi } from 'vitest'

import { fetchSeries, fetchState } from './state'

const empty: string[] = []

// What the last call asked for, which is how the conditional request is
// checked: the ETag only earns its keep if it is actually sent back.
let asked: RequestInit | undefined

function answering(
  status: number,
  body: unknown,
  headers: Record<string, string> = {},
) {
  return (_url: string, init?: RequestInit) => {
    asked = init
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

  const snapshot = await fetchState(null)

  expect(snapshot?.etag).toBe('"abc"')
  expect(snapshot?.state.tasks[0]?.title).toBe('Ship it')
  expect(asked?.headers).toEqual({})
})

test('an unchanged store is nothing to redraw rather than an empty one', async () => {
  vi.stubGlobal('fetch', answering(304, null))

  expect(await fetchState('"abc"')).toBeNull()
  expect(asked?.headers).toEqual({ 'If-None-Match': '"abc"' })
})

test('a refusal surfaces the sentence the CLI would have printed', async () => {
  const sentence = 'usage: sort is one of [title deadline], not "nope"'
  vi.stubGlobal('fetch', answering(400, { error: sentence }))

  await expect(fetchState(null)).rejects.toThrow(sentence)
})

test('an error with no sentence in it still says what happened', async () => {
  vi.stubGlobal('fetch', () =>
    Promise.resolve(new Response('nope', { status: 500 })),
  )

  await expect(fetchState(null)).rejects.toThrow('the API answered 500')
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
