// @vitest-environment jsdom

// The one thing the layout must not lose: selecting a Task and opening it are
// one state, drawn two ways. Which of the two ways is `index.css`'s to decide,
// so what this pins is that both are drawn from one tree — the list is still
// there under the selection rather than having been replaced by a screen.

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { App } from './App'
import { OFFERED_NOTHING, type State } from './state'
import * as state from './state'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const STATE: State = {
  ...OFFERED_NOTHING,
  tasks: [
    {
      id: 'one',
      depth: 1,
      title: 'Move house',
      createdAt: '2026-03-04T09:00:00Z',
      marks: [],
    },
  ],
}

beforeEach(() => {
  vi.spyOn(state, 'fetchState').mockResolvedValue({
    state: STATE,
    etag: 'w/1',
  })
  // The detail pane reads the Change History for itself. What it says about a
  // read that failed is its own screen's business; this is about the list.
  vi.spyOn(state, 'fetchTaskHistory').mockResolvedValue([])
  vi.spyOn(state, 'fetchSeries').mockResolvedValue({
    repeats: false,
    occurrences: [],
  })
})

test('a selected Task is drawn beside the list it was selected from', async () => {
  render(<App />)

  fireEvent.click(await screen.findByRole('button', { name: /Move house/ }))

  // The Task is on the screen twice: the row it was selected from and the pane
  // about it. At a desk those are two panes and on a phone the row is the one
  // CSS hides, and neither is a second piece of state.
  expect(
    await screen.findByRole('heading', { name: 'Move house' }),
  ).toBeDefined()
  expect(screen.getByRole('button', { name: /Move house/ })).toBeDefined()
})

test('typing in the search box asks the store for the narrower list', async () => {
  render(<App />)
  await screen.findByRole('button', { name: /Move house/ })

  fireEvent.change(screen.getByRole('searchbox', { name: 'Search' }), {
    target: { value: 'house' },
  })

  // The narrowing goes back to the store rather than sifting what is already
  // on the screen, which is what makes the box over the list a read and not a
  // filter this surface keeps.
  await vi.waitFor(() => {
    expect(
      vi
        .mocked(state.fetchState)
        .mock.calls.some(([narrowing]) => narrowing.search === 'house'),
    ).toBe(true)
  })
})
