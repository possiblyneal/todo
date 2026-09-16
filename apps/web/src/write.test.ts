import { afterEach, expect, test, vi } from 'vitest'

import { addTask, ask, capture } from './write'

// What the last call sent, which is how the body is checked: the routes take
// JSON and a client that posted something else would still get a Response.
let sent: { url: string; init?: RequestInit } | undefined

function answering(status: number, body: unknown) {
  return (url: string, init?: RequestInit) => {
    sent = { url, init }
    return Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
  }
}

function body() {
  return JSON.parse(String(sent?.init?.body)) as Record<string, unknown>
}

afterEach(() => {
  sent = undefined
  vi.unstubAllGlobals()
})

test('a dump goes to the broker and comes back as the body the sheet submits', async () => {
  vi.stubGlobal(
    'fetch',
    answering(200, {
      title: 'Paint the fence',
      estimate: '90m',
      intoLists: ['list_house'],
    }),
  )

  const draft = await capture('paint the fence before march')

  expect(sent?.url).toBe('/api/capture')
  expect(sent?.init?.method).toBe('POST')
  expect(body()).toEqual({ text: 'paint the fence before march' })
  expect(draft.title).toBe('Paint the fence')
  expect(draft.intoLists).toEqual(['list_house'])
})

test('a question comes back as prose', async () => {
  vi.stubGlobal('fetch', answering(200, { answer: 'Start with the fence.' }))

  expect(await ask('what first?')).toBe('Start with the fence.')
  expect(sent?.url).toBe('/api/ask')
  expect(body()).toEqual({ question: 'what first?' })
})

test('a written task is named by the id the api answers with', async () => {
  vi.stubGlobal('fetch', answering(201, { id: 'task_abc' }))

  const id = await addTask({ title: 'Paint the fence', intoLists: ['list_a'] })

  expect(id).toBe('task_abc')
  expect(body()).toEqual({ title: 'Paint the fence', intoLists: ['list_a'] })
})

test('a refusal surfaces the sentence the CLI would have printed', async () => {
  const sentence =
    'cannot read "next tuesday" as a date: want 2006-01-02, 2006-01-02 15:04, or RFC 3339'
  vi.stubGlobal('fetch', answering(400, { error: sentence }))

  await expect(addTask({ deadline: 'next tuesday' })).rejects.toThrow(sentence)
})

test('an error with no sentence in it still says what happened', async () => {
  vi.stubGlobal('fetch', () =>
    Promise.resolve(new Response('nope', { status: 500 })),
  )

  await expect(capture('paint the fence')).rejects.toThrow(
    'the API answered 500',
  )
})
