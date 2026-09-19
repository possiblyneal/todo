// @vitest-environment jsdom

// The one thing the controls do that is not "set a field": a picker keeps a
// narrowing the client cannot name, rather than rendering blank over a list
// that is still narrowed to it.

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { Narrow } from './Narrow'
import { WIDE } from './state'

afterEach(cleanup)

const LISTS = [{ id: 'l1', name: 'Home', color: 'blue', count: 2 }]
const TAGS = [{ id: 't1', name: 'errand', color: 'red', count: 1 }]

function shown(narrowing = WIDE) {
  const onChange = vi.fn()
  render(
    <Narrow
      narrowing={narrowing}
      lists={LISTS}
      tags={TAGS}
      sorts={['title', 'deadline']}
      onChange={onChange}
    />,
  )
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
