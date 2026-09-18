// @vitest-environment jsdom

// The one thing on the detail screen that cannot be read off it: whether the
// field holding a pointer is cleared. A refusal writes nothing, so what was
// typed has to survive it -- and on a phone, retyping the pointer is the whole
// cost of the feature.

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { Detail } from './Detail'
import { OFFERED_NOTHING, type Task } from './state'
import * as state from './state'
import * as write from './write'

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

/** Renders the screen with the history read and the attach write stubbed. */
function opened(attaching: Promise<void>) {
  vi.spyOn(state, 'fetchTaskHistory').mockResolvedValue([])
  const attach = vi.spyOn(write, 'attach').mockReturnValue(attaching)
  render(
    <Detail
      task={TASK}
      subtasks={[]}
      offered={OFFERED_NOTHING}
      revision={null}
      onOpen={() => {}}
      onBack={() => {}}
    />,
  )
  const box = screen.getByLabelText('New attachment') as HTMLInputElement
  fireEvent.change(box, { target: { value: '/home/neal/plans/shed.pdf' } })
  fireEvent.click(screen.getByRole('button', { name: 'Attach' }))
  return { attach, box }
}

test('a refused attach keeps the pointer for another tap', async () => {
  const { attach, box } = opened(Promise.reject(new Error('the Lease is held')))

  expect(attach).toHaveBeenCalledWith('t1', '/home/neal/plans/shed.pdf')
  await vi.waitFor(() =>
    expect(screen.getByText('the Lease is held')).toBeTruthy(),
  )
  // Still here, and the button that retries is still reachable: clearing the
  // field would disable it, since an empty target cannot be submitted.
  expect(box.value).toBe('/home/neal/plans/shed.pdf')
  expect(
    (screen.getByRole('button', { name: 'Attach' }) as HTMLButtonElement)
      .disabled,
  ).toBe(false)
})

test('an attach that landed clears the field', async () => {
  const { box } = opened(Promise.resolve())

  await vi.waitFor(() => expect(box.value).toBe(''))
})
