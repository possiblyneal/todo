import { afterEach, expect, test, vi } from 'vitest'

import {
  addSubtask,
  addTask,
  ask,
  capture,
  draftOf,
  editTask,
  lifecycle,
  memberships,
} from './write'

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

test('the sheet submits the memberships that changed, both ways', () => {
  const before = { intoLists: ['house'], addTags: ['outdoors'] }
  const after = { intoLists: ['garage'], addTags: ['outdoors', 'spring'] }

  expect(memberships(before, after)).toEqual({
    intoLists: ['garage'],
    outOfLists: ['house'],
    addTags: ['spring'],
    dropTags: [],
  })
})

// A sheet opened on a draft carries nothing to leave, so every tick is a join.
test('a draft with no memberships joins whatever is ticked', () => {
  expect(memberships({}, { intoLists: ['house'] })).toEqual({
    intoLists: ['house'],
    outOfLists: [],
    addTags: [],
    dropTags: [],
  })
})

// The edit sheet opens on the Task as it is, and what it shows has to go back
// as something the API reads: a deadline the way it came, a duration as one.
test('a task reads back as the body that edits it', () => {
  const draft = draftOf({
    id: 'task_a',
    depth: 1,
    title: 'Paint the fence',
    createdAt: '2026-09-16T10:00:00Z',
    deadline: '2026-03-04T00:00:00Z',
    estimateSeconds: 5400,
    lists: ['house'],
    tags: ['outdoors'],
    marks: [],
  })

  expect(draft.title).toBe('Paint the fence')
  expect(draft.deadline).toBe('2026-03-04T00:00:00Z')
  expect(draft.estimate).toBe('90m')
  expect(draft.intoLists).toEqual(['house'])
  expect(draft.addTags).toEqual(['outdoors'])
})

test('a task with nothing to estimate has no estimate to send', () => {
  const draft = draftOf({
    id: 'task_a',
    depth: 1,
    title: 'Paint the fence',
    createdAt: '2026-09-16T10:00:00Z',
    marks: [],
  })

  expect(draft.estimate).toBeUndefined()
  expect(draft.deadline).toBeUndefined()
})

// The four are the API's list, so this sends the verb and reads the sentence
// back rather than keeping a second copy of which four there are.
test('a lifecycle verb is one post to the task', async () => {
  vi.stubGlobal('fetch', answering(200, { id: 'task_abc' }))

  await lifecycle('task_abc', 'complete')

  expect(sent?.url).toBe('/api/tasks/task_abc/complete')
  expect(sent?.init?.method).toBe('POST')
})

test('a subtask is written under the task in the path', async () => {
  vi.stubGlobal('fetch', answering(201, { id: 'task_child' }))

  const id = await addSubtask('task_parent', { title: 'Buy the paint' })

  expect(id).toBe('task_child')
  expect(sent?.url).toBe('/api/tasks/task_parent/subtasks')
  expect(body()).toEqual({ title: 'Buy the paint' })
})

test('an edit patches the task it is about', async () => {
  vi.stubGlobal('fetch', answering(200, { id: 'task_abc' }))

  await editTask('task_abc', { title: 'Paint the shed' })

  expect(sent?.url).toBe('/api/tasks/task_abc')
  expect(sent?.init?.method).toBe('PATCH')
})
