// @vitest-environment jsdom

// The collections screen's one rule that cannot be read off the screen: a
// rename and a recolor are one write, and a row nobody touched is no write at
// all. The rest of it is text in and text out.

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { Collections } from './Collections'
import { OFFERED_NOTHING, type Offered } from './state'
import * as write from './write'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const OFFERED: Offered = {
  ...OFFERED_NOTHING,
  lists: [{ id: 'l1', name: 'Home', color: 'blue', count: 2 }],
  tags: [{ id: 't1', name: 'errand', color: 'red', count: 1 }],
  colors: ['red', 'blue'],
}

/** Renders the screen with every write stubbed, and answers with the stubs. */
function opened() {
  const wrote = {
    add: vi.spyOn(write, 'addCollection').mockResolvedValue('new'),
    describe: vi.spyOn(write, 'describeCollection').mockResolvedValue(),
    drop: vi.spyOn(write, 'dropCollection').mockResolvedValue(),
  }
  render(<Collections offered={OFFERED} onBack={() => {}} />)
  return wrote
}

// Two attributes and one entry: a screen that sent the rename and the recolor
// separately would put two rows in the Change History for one correction.
test('a rename and a recolor go out as one write', () => {
  const wrote = opened()
  fireEvent.change(screen.getByLabelText('Home name'), {
    target: { value: 'House' },
  })
  fireEvent.change(screen.getByLabelText('Home color'), {
    target: { value: 'red' },
  })
  fireEvent.click(screen.getAllByRole('button', { name: 'Save' })[0]!)
  expect(wrote.describe).toHaveBeenCalledWith('lists', 'l1', {
    name: 'House',
    color: 'red',
  })
})

// Absent is what tells the store to leave an attribute alone, so a rename
// carries no color and a recolor carries no name. A body that always sent both
// would put back whatever another Actor changed while the row sat open.
test('a rename carries the name alone', () => {
  const wrote = opened()
  fireEvent.change(screen.getByLabelText('Home name'), {
    target: { value: 'House' },
  })
  fireEvent.click(screen.getAllByRole('button', { name: 'Save' })[0]!)
  expect(wrote.describe).toHaveBeenCalledWith('lists', 'l1', { name: 'House' })
})

test('a recolor carries the color alone', () => {
  const wrote = opened()
  fireEvent.change(screen.getByLabelText('Home color'), {
    target: { value: 'red' },
  })
  fireEvent.click(screen.getAllByRole('button', { name: 'Save' })[0]!)
  expect(wrote.describe).toHaveBeenCalledWith('lists', 'l1', { color: 'red' })
})

test('a row nobody touched cannot be saved', () => {
  opened()
  const save = screen.getAllByRole('button', { name: 'Save' })[0]!
  expect((save as HTMLButtonElement).disabled).toBe(true)
})

// The two sets are the same three writes against different aggregates, so the
// only thing that tells them apart on the wire is the segment.
test('a Tag is written under its own kind', () => {
  const wrote = opened()
  fireEvent.click(screen.getAllByRole('button', { name: 'Delete' })[1]!)
  expect(wrote.drop).toHaveBeenCalledWith('tags', 't1')
})

test('the blank row creates one and then clears itself', async () => {
  const wrote = opened()
  const box = screen.getByLabelText('New List') as HTMLInputElement
  fireEvent.change(box, { target: { value: 'Work' } })
  fireEvent.click(screen.getAllByRole('button', { name: 'Add' })[0]!)
  expect(wrote.add).toHaveBeenCalledWith('lists', { name: 'Work', color: '' })
  await vi.waitFor(() => expect(box.value).toBe(''))
})

test('an unnamed create cannot be submitted', () => {
  opened()
  const add = screen.getAllByRole('button', { name: 'Add' })[0]!
  expect((add as HTMLButtonElement).disabled).toBe(true)
})

// A color the read does not offer is still the color the Collection carries,
// so the picker holds it rather than rendering blank over it.
test('a color the client does not offer stays on the picker', () => {
  render(
    <Collections
      offered={{
        ...OFFERED,
        lists: [{ id: 'l1', name: 'Home', color: 'chartreuse', count: 0 }],
      }}
      onBack={() => {}}
    />,
  )
  expect((screen.getByLabelText('Home color') as HTMLSelectElement).value).toBe(
    'chartreuse',
  )
})
