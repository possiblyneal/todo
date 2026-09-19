// @vitest-environment jsdom

// The row's own grammar: what it does with a value nothing named and with an
// instant it cannot read. Everything else on it is drawn from what it was
// handed, but those two are decisions this file makes, and they are the ones
// a reader of the component cannot check by reading it.

import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { Row } from './Row'
import type { Collection, Task } from './state'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const TASK: Task = {
  id: 't1',
  depth: 1,
  title: 'Paint the shed',
  createdAt: '2026-01-01T00:00:00Z',
  marks: [],
}

const WORK: Collection = { id: 'list_1', name: 'Work', color: 'blue', count: 3 }

function drawn(task: Partial<Task>, lists: Collection[] = [WORK]) {
  render(<Row task={{ ...TASK, ...task }} lists={lists} onOpen={() => {}} />)
}

test('a List is drawn by name, and one nothing named is drawn as its id', () => {
  drawn({ lists: ['list_1', 'list_gone'] })
  expect(screen.getByText('Work')).toBeDefined()
  // Not dropped: a filing nobody can see is one nobody thinks to change.
  expect(screen.getByText('list_gone')).toBeDefined()
})

test('two Lists sharing a name do not collide', () => {
  // Nothing in the store makes a List name unique, and `nameOf` falls back to
  // the id, so a name is not unique twice over. React draws both either way
  // and only complains, so the complaint is what this asserts on: the ids are
  // what is unique and what the spans are keyed on.
  const complaints: unknown[][] = []
  vi.spyOn(console, 'error').mockImplementation((...said) =>
    complaints.push(said),
  )
  const other: Collection = { ...WORK, id: 'list_2' }
  drawn({ lists: ['list_1', 'list_2'] }, [WORK, other])
  expect(screen.getAllByText('Work')).toHaveLength(2)
  expect(complaints.flat().join(' ')).not.toContain('same key')
})

test('an instant no date can be read out of is drawn as it came', () => {
  drawn({ createdAt: 'whenever' })
  expect(screen.getByText('Created whenever')).toBeDefined()
})

test('a Deadline is drawn only when there is one', () => {
  drawn({})
  expect(screen.queryByText(/^Deadline/)).toBeNull()
})

test('the paperclip is named, and is there only when a pointer is', () => {
  drawn({})
  expect(screen.queryByLabelText('Has attachments')).toBeNull()
  cleanup()
  drawn({ attachments: ['https://example.com/x'] })
  // `role="img"` is what lets the name be exposed at all: ARIA does not name a
  // bare span, so without it a reader hears the emoji or nothing.
  expect(screen.getByLabelText('Has attachments').getAttribute('role')).toBe(
    'img',
  )
})
