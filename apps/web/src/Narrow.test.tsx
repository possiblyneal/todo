// @vitest-environment jsdom

// The one thing the controls do that is not "set a field": a picker keeps a
// narrowing the client cannot name, rather than rendering blank over a list
// that is still narrowed to it.

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { Narrow } from './Narrow'
import { drawnKeys } from './rank'
import { OFFERED_NOTHING, WIDE, type Offered } from './state'

afterEach(cleanup)

const OFFERED: Offered = {
  ...OFFERED_NOTHING,
  lists: [{ id: 'l1', name: 'Home', color: 'blue', count: 2 }],
  tags: [{ id: 't1', name: 'errand', color: 'red', count: 1 }],
  sorts: ['title', 'deadline'],
  colors: ['blue', 'red'],
  snoozes: ['1h', '1d'],
}

function shown(narrowing = WIDE) {
  const onChange = vi.fn()
  render(<Narrow narrowing={narrowing} offered={OFFERED} onChange={onChange} />)
  return onChange
}

// Five served sets arrive under one prop now, so reading the wrong one off it
// is a mistake that can be made. This says which set feeds the sort.
test('the sort offers the orders the store served and nothing else', () => {
  shown()
  const picker = screen.getByLabelText('Sort') as HTMLSelectElement
  expect([...picker.options].map((o) => o.value)).toEqual([
    '',
    'title',
    'deadline',
  ])
})

test('a List deleted elsewhere stays on the picker under its own id', () => {
  shown({ ...WIDE, list: 'gone' })
  const picker = screen.getByLabelText('List') as HTMLSelectElement
  expect(picker.value).toBe('gone')
  // Under the id alone, with no count beside it: the store never said how many
  // Tasks are in it, and a zero here would be the client making one up.
  expect([...picker.options].map((o) => o.textContent)).toContain('gone')
})

test('a Tag deleted elsewhere stays on its picker too', () => {
  shown({ ...WIDE, tag: 'gone' })
  expect((screen.getByLabelText('Tag') as HTMLSelectElement).value).toBe('gone')
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
// the read.
test('each character typed is its own narrowing', () => {
  const onChange = shown()
  fireEvent.change(screen.getByLabelText('Search'), { target: { value: 'fe' } })
  expect(onChange).toHaveBeenCalledWith({ ...WIDE, search: 'fe' })
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

// The controls are unmounted whenever another screen is open, and the Tag order
// has to survive that: somebody who opens a Task and comes back is looking for
// the Tag where they last saw it. Thirty Tags drawn twice would agree by
// chance about once in 10^32 reads, so an order that matches is an order that
// was not redrawn.
test('the Tag order survives the controls being unmounted', () => {
  drawnKeys.clear()
  const many: Offered = {
    ...OFFERED,
    tags: Array.from({ length: 30 }, (_, at) => ({
      id: `t${at}`,
      name: `tag${at}`,
      color: 'red',
      count: at % 5,
    })),
  }
  const order = () => {
    render(<Narrow narrowing={WIDE} offered={many} onChange={vi.fn()} />)
    const picker = screen.getByLabelText('Tag') as HTMLSelectElement
    const values = [...picker.options].map((one) => one.value)
    cleanup()
    return values
  }
  expect(order()).toEqual(order())
})
