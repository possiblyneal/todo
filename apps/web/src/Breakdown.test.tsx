// @vitest-environment jsdom

// What the approval is over. A tick that writes six attributes and shows three
// is a gate over half of what it lets through, so this is about what reaches
// the screen rather than about what the Broker said.

import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { Breakdown } from './Breakdown'
import type { Task } from './state'
import * as write from './write'

afterEach(cleanup)

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
  expect(screen.getByText('30m')).toBeDefined()
  // Labelled, because `high` alone says neither which attribute it is nor that
  // anybody chose it.
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
