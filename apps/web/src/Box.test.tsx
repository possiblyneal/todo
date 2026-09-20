// @vitest-environment jsdom

// The box's one grammar: which route a dump goes out on. What the Broker read
// is the same body either way, so the only thing that says a sentence typed
// under a Task becomes a Subtask of it is the `parent` this was given -- and
// getting that backwards writes a top-level Task nobody asked for.

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { Box } from './Box'
import { OFFERED_NOTHING, WIDE } from './state'
import * as write from './write'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

/**
 * Renders the box it is given, hands it a dump, and submits the sheet that
 * comes back. Which box it is is spelled out at each call rather than switched
 * on here: the two are alternatives the type keeps apart, so there is no one
 * element that stands for both.
 */
async function dumped(box: React.ReactElement) {
  vi.spyOn(write, 'capture').mockResolvedValue({ title: 'Sand it down' })
  const addTask = vi.spyOn(write, 'addTask').mockResolvedValue('t2')
  const addSubtask = vi.spyOn(write, 'addSubtask').mockResolvedValue('t2')
  render(box)
  fireEvent.change(screen.getByRole('textbox'), {
    target: { value: 'sand the shed down' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Add' }))
  await vi.waitFor(() =>
    expect(screen.getByLabelText('Title')).toHaveProperty(
      'value',
      'Sand it down',
    ),
  )
  // The sheet's own Add is the one that submits it: the row that makes a List
  // and the row that adds a field say Add too, and only one of the three is a
  // submit.
  const submit = screen
    .getAllByRole('button', { name: 'Add' })
    .find((one) => (one as HTMLButtonElement).type === 'submit')
  if (!submit) throw new Error('the sheet has nothing to submit')
  fireEvent.click(submit)
  return { addTask, addSubtask }
}

test('a dump under a Task is submitted as a Subtask of it', async () => {
  const { addTask, addSubtask } = await dumped(
    <Box offered={OFFERED_NOTHING} parent="t1" />,
  )
  await vi.waitFor(() =>
    expect(addSubtask).toHaveBeenCalledWith(
      't1',
      expect.objectContaining({ title: 'Sand it down' }),
    ),
  )
  expect(addTask).not.toHaveBeenCalled()
})

test('a dump over the list is submitted as a top-level Task', async () => {
  const { addTask, addSubtask } = await dumped(
    <Box offered={OFFERED_NOTHING} narrowing={WIDE} />,
  )
  await vi.waitFor(() => expect(addTask).toHaveBeenCalled())
  expect(addSubtask).not.toHaveBeenCalled()
})

// A question is about a list, and a Task's own box has none: an Ask there
// would narrow by nothing and answer about every open Task in the tracker.
// What keeps the two apart is the type -- a box is given a Narrowing or a
// parent and never both -- so this pins what that comes to on the screen.
test('a Task of its own is not something the box asks about', () => {
  render(<Box offered={OFFERED_NOTHING} parent="t1" />)
  expect(screen.queryByRole('button', { name: 'Ask' })).toBeNull()
  expect(screen.getByRole('button', { name: 'Add' })).toBeTruthy()
})

test("the list's box is the one with a question under the same thumb", () => {
  render(<Box offered={OFFERED_NOTHING} narrowing={WIDE} />)
  expect(screen.getByRole('button', { name: 'Ask' })).toBeTruthy()
})
