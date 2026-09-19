// @vitest-environment jsdom

// What the approval is over. A tick that writes six attributes and shows three
// is a gate over half of what it lets through, so this is about what reaches
// the screen rather than about what the Broker said.

import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { Breakdown } from './Breakdown'
import type { Task } from './state'
import * as write from './write'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const TASK: Task = {
  id: 'one',
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
  // Labelled, because a single word alone says neither which attribute it is
  // nor that anybody chose it: `30m` reads as much like a deadline as like an
  // estimate, and `high` says nothing at all.
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
