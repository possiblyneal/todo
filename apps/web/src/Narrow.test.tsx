// @vitest-environment jsdom

// The one thing the controls do that is not "set a field": a picker keeps a
// narrowing the client cannot name, rather than rendering blank over a list
// that is still narrowed to it.

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { Narrow, Search } from './Narrow'
import { OFFERED_NOTHING, WIDE, type Offered } from './state'

afterEach(cleanup)

const OFFERED: Offered = {
  ...OFFERED_NOTHING,
  lists: [{ id: 'l1', name: 'Home', color: 'blue', count: 2 }],
  tags: [{ id: 't1', name: 'errand', color: 'red', count: 1 }],
  sorts: ['title', 'deadline'],
}

function shown(narrowing = WIDE) {
  const onChange = vi.fn()
  render(<Narrow narrowing={narrowing} offered={OFFERED} onChange={onChange} />)
  return onChange
}

test('a List deleted elsewhere stays on the picker under its own id', () => {
  shown({ ...WIDE, list: 'gone' })
  const picker = screen.getByLabelText('List') as HTMLSelectElement
  expect(picker.value).toBe('gone')
  // Under the id alone, with no count beside it: the store never said how many
  // Tasks are in it, and a zero here would be the client making one up.
  expect([...picker.options].map((o) => o.textContent)).toContain('gone')
})

// The Tags are switches rather than a picker, and the rule is the same one: a
// Tag that went away while the list was narrowed to it is still the thing to
// turn off, so it is drawn under its id rather than leaving the list narrowed
// by something with nothing on the screen to undo it.
test('a Tag deleted elsewhere stays on as a switch under its own id', () => {
  shown({ ...WIDE, tags: ['gone'] })
  const gone = screen.getByRole('button', { name: 'gone' })
  expect(gone.getAttribute('aria-pressed')).toBe('true')
})

// Any of them, not another narrowing on top: a Tag switched on is added to the
// set the route is asked under rather than replacing what is already there.
test('a Tag switched on is added to the set rather than replacing it', () => {
  const onChange = shown({ ...WIDE, tags: ['other'] })
  fireEvent.click(screen.getByRole('button', { name: 'errand (1)' }))
  expect(onChange).toHaveBeenCalledWith({ ...WIDE, tags: ['other', 't1'] })
})

test('a Tag switched off leaves the others on', () => {
  const onChange = shown({ ...WIDE, tags: ['other', 't1'] })
  fireEvent.click(screen.getByRole('button', { name: 'errand (1)' }))
  expect(onChange).toHaveBeenCalledWith({ ...WIDE, tags: ['other'] })
})

test('a picker offers each Collection once and no more', () => {
  shown({ ...WIDE, list: 'l1' })
  const picker = screen.getByLabelText('List') as HTMLSelectElement
  expect([...picker.options].map((o) => o.value)).toEqual(['', 'l1'])
})

test('picking nothing widens the narrowing back out', () => {
  const onChange = shown({ ...WIDE, list: 'l1' })
  fireEvent.change(screen.getByLabelText('List'), { target: { value: '' } })
  expect(onChange).toHaveBeenCalledWith({ ...WIDE, list: '' })
})

// Each keystroke is a new narrowing, so there is no timer between the box and
// the read. The box is drawn apart from the rest of the controls, because it
// belongs over the list rather than among the filtering.
test('each character typed is its own narrowing', () => {
  const onChange = vi.fn()
  render(<Search value="" onChange={onChange} />)
  fireEvent.change(screen.getByLabelText('Search'), { target: { value: 'fe' } })
  expect(onChange).toHaveBeenCalledWith('fe')
})

test('showing everything is one toggle and not four', () => {
  const onChange = shown()
  fireEvent.click(screen.getByRole('button', { name: 'Show everything' }))
  expect(onChange).toHaveBeenCalledWith({ ...WIDE, all: true })
})

// The same button is the way back, so it reads the narrowing it is given
// rather than only ever asking for everything.
test('showing everything again narrows back to the everyday view', () => {
  const onChange = shown({ ...WIDE, all: true })
  fireEvent.click(screen.getByRole('button', { name: 'Everything shown' }))
  expect(onChange).toHaveBeenCalledWith({ ...WIDE, all: false })
})
