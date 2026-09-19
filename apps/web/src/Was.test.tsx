// @vitest-environment jsdom

// A deleted Task is reachable at the entry that deleted it and nowhere else,
// so the two halves of that path are what these check: an entry is something
// to press, and pressing it reads the Task out at that position.

import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { Log } from './Log'
import { OFFERED_NOTHING, type Entry, type Task } from './state'
import { Was } from './Was'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

const GONE: Entry = {
  seq: 12,
  at: '2026-09-19T10:00:00Z',
  actor: 'claude-code/claude-opus-5',
  kind: 'task_deleted',
  subject: 'abc',
}

const WAS: Task = {
  id: 'abc',
  depth: 1,
  title: 'Buy cerulean paint',
  why: 'the hallway',
  createdAt: '2026-09-01T09:00:00Z',
  deletedAt: '2026-09-19T10:00:00Z',
  marks: ['deleted'],
}

test('an entry is something to press when there is somewhere to go', () => {
  const onOpen = vi.fn()
  render(<Log entries={[GONE]} onOpen={onOpen} />)
  screen.getByRole('button').click()
  expect(onOpen).toHaveBeenCalledWith(GONE)
})

// Without onOpen the log is a log. The detail screen drew it that way before
// there was anywhere to go, and a row that looked pressable and did nothing
// would be worse than a row that does not.
test('an entry is not a button where nothing opens', () => {
  render(<Log entries={[GONE]} />)
  expect(screen.queryByRole('button')).toBeNull()
})

test('the entry reads the Task out as it stood at that position', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify(WAS), { status: 200 })),
  )
  render(<Was entry={GONE} offered={OFFERED_NOTHING} onBack={() => {}} />)

  await waitFor(() =>
    expect(screen.getByText('Buy cerulean paint')).toBeDefined(),
  )
  // What happened is drawn above the Task, because the screen was opened from
  // the entry rather than from the Task.
  expect(screen.getByText(/task deleted/i)).toBeDefined()
  expect(screen.getByText('the hallway')).toBeDefined()
  expect(globalThis.fetch).toHaveBeenCalledWith('/api/tasks/abc/at/12')
})
