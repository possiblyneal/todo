// @vitest-environment jsdom

// What the approval is over. A tick that writes six attributes and shows three
// is a gate over half of what it lets through, so this is about what reaches
// the screen rather than about what the Broker said.

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { Breakdown } from './Breakdown'
import type { Task } from './state'
import * as write from './write'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const TASK: Task = {
  id: 't1',
  title: 'Move house',
  depth: 0,
  createdAt: '2026-03-04T09:00:00Z',
  marks: [],
}

test('every attribute a proposal would write is drawn beside its tick', async () => {
  vi.spyOn(write, 'breakdown').mockResolvedValue({
    questions: [],
    proposals: [
      {
        title: 'Book the van',
        description: 'The big one, not the transit.',
        why: 'Nothing else moves until it is booked.',
        estimate: '30m',
        priority: 'high',
        impact: 'med',
      },
    ],
  })
  render(<Breakdown task={TASK} onBack={() => {}} />)

  await screen.findByText('Book the van')
  expect(screen.getByText('The big one, not the transit.')).toBeDefined()
  expect(
    screen.getByText('Nothing else moves until it is booked.'),
  ).toBeDefined()
  // The labels are what is pinned; `Breakdown.tsx` is where they are argued.
  expect(screen.getByText('Estimate 30m')).toBeDefined()
  expect(screen.getByText('Priority high')).toBeDefined()
  expect(screen.getByText('Impact med')).toBeDefined()
})

// The screen draws what the Broker said and parses none of it. A level that is
// none of the three is the store's to refuse, and blanking it here would let it
// be approved without ever having been seen.
test('a level the store would refuse is drawn as the word the Broker used', async () => {
  vi.spyOn(write, 'breakdown').mockResolvedValue({
    questions: [],
    proposals: [{ title: 'Book the van', priority: 'urgent' }],
  })
  render(<Breakdown task={TASK} onBack={() => {}} />)

  await screen.findByText('Book the van')
  expect(screen.getByText('Priority urgent')).toBeDefined()
})

// The line under a proposal holds the estimate and the two levels, and the
// Broker answers none of them on a Subtask it has nothing to say about. Drawn
// anyway it is an empty strip of nothing under the title, which reads as an
// attribute that failed to load rather than one nobody set.
test('the line of single words is not drawn when there are none', async () => {
  vi.spyOn(write, 'breakdown').mockResolvedValue({
    questions: [],
    proposals: [{ title: 'Book the van' }],
  })
  const drawn = render(<Breakdown task={TASK} onBack={() => {}} />)

  await screen.findByText('Book the van')
  expect(drawn.container.querySelector('.facts')).toBeNull()
})

// The gate is that the write carries nothing the tick did not show. The type
// says so on this side and `write.AsProposed` says so on the other, but the
// thing in between is this screen handing the proposal to `addSubtask`
// untouched, and that is what would quietly stop being true.
test('approving writes the proposal and nothing added to it', async () => {
  vi.spyOn(write, 'breakdown').mockResolvedValue({
    questions: [],
    proposals: [{ title: 'Book the van', estimate: '30m' }],
  })
  const wrote = vi.spyOn(write, 'addSubtask').mockResolvedValue('t2')
  render(<Breakdown task={TASK} onBack={() => {}} />)

  await screen.findByText('Book the van')
  fireEvent.click(screen.getByRole('button', { name: 'Add 1' }))

  await vi.waitFor(() => expect(wrote).toHaveBeenCalledTimes(1))
  expect(wrote.mock.calls[0]?.[0]).toBe('t1')
  expect(wrote.mock.calls[0]?.[1]).toEqual({
    title: 'Book the van',
    estimate: '30m',
  })
})
