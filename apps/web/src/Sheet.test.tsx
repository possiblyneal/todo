// @vitest-environment jsdom

// The sheet's grammars, which are the part of it that cannot be read off the
// screen. Every other field is text in and text out; these three turn what was
// picked into a different thing on the wire, and getting one backwards is a
// write nobody sees go wrong.

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'

import { Sheet } from './Sheet'
import type { Offered } from './state'
import type { TaskBody } from './write'

afterEach(cleanup)

const OFFERED: Offered = {
  lists: [{ id: 'l1', name: 'Home', color: 'blue', count: 2 }],
  tags: [{ id: 't1', name: 'errand', color: 'red', count: 1 }],
  sorts: ['title'],
  colors: ['red', 'blue'],
  snoozes: ['an hour', 'tomorrow'],
}

/** Renders the sheet and answers with what submitting it sent. */
function opened(draft: TaskBody, against?: TaskBody) {
  const sent: TaskBody[] = []
  render(
    <Sheet
      draft={draft}
      against={against}
      offered={OFFERED}
      action="Save"
      onSubmit={(body) => {
        sent.push(body)
        return Promise.resolve()
      }}
      onCancel={() => {}}
    />,
  )
  return {
    /** What submitting sent, which is what every assertion below is about. */
    submit: () => {
      fireEvent.click(screen.getByRole('button', { name: 'Save' }))
      const body = sent.at(-1)
      if (!body) throw new Error('the sheet submitted nothing')
      return body
    },
    pick: (label: string, value: string) =>
      fireEvent.change(screen.getByLabelText(label), { target: { value } }),
  }
}

// Undefined, empty and a label are three different writes, and the control has
// to keep them three: the first leaves a snoozed Task snoozed through an edit
// about something else, and the second is the only way back from a snooze on
// this surface.
test('an untouched sheet sends no snooze at all', () => {
  const sheet = opened({ title: 'Buy milk' })
  const body = sheet.submit()
  expect(body.snooze).toBeUndefined()
})

test('waking a Task sends an empty snooze rather than none', () => {
  const sheet = opened({ title: 'Buy milk' })
  sheet.pick('Snooze', 'wake')
  const body = sheet.submit()
  expect(body.snooze).toBe('')
})

test('an offered snooze is sent under its own label', () => {
  const sheet = opened({ title: 'Buy milk' })
  sheet.pick('Snooze', 'tomorrow')
  const body = sheet.submit()
  expect(body.snooze).toBe('tomorrow')
})

// The third carrier of the unknown-value rule, and the one whose fallback has
// two extra arms: `undefined` and `''` are the sentinels the grammar above
// reads, so neither may be drawn as a snooze the Broker said. A duration
// `write.Snooze` takes but this side was never offered has to survive the trip
// to the screen, or the Broker's word is blanked on the way.
test('a snooze the client was not offered is kept and sent as it came', () => {
  const sheet = opened({ title: 'Buy milk', snooze: '90m' })
  expect(screen.getByLabelText('Snooze')).toHaveProperty('value', '90m')
  expect(sheet.submit().snooze).toBe('90m')
})

test('a snooze picked and then put back is absent again', () => {
  const sheet = opened({ title: 'Buy milk' })
  sheet.pick('Snooze', 'tomorrow')
  sheet.pick('Snooze', 'leave')
  const body = sheet.submit()
  expect(body.snooze).toBeUndefined()
})

// The Broker chose the word, so a value none of the offered ones is offered as
// one more. A picker that silently could not hold it would blank the answer on
// the way to the screen, which is the one thing the sheet exists to prevent.
test('a level the client does not offer is on the picker and submitted', () => {
  const sheet = opened({ title: 'Ship it', priority: 'urgent' })
  const picker = screen.getByLabelText('Priority') as HTMLSelectElement

  expect([...picker.options].map((o) => o.value)).toContain('urgent')
  expect(picker.value).toBe('urgent')

  const body = sheet.submit()
  expect(body.priority).toBe('urgent')
})

test('a color the client does not offer is on the picker too', () => {
  opened({ title: 'Ship it', color: 'chartreuse' })
  const picker = screen.getByLabelText('Color') as HTMLSelectElement
  expect(picker.value).toBe('chartreuse')
})

test('emptying a picker clears the attribute rather than leaving it alone', () => {
  const sheet = opened({ title: 'Ship it', priority: 'high' })
  sheet.pick('Priority', '')
  const body = sheet.submit()
  expect(body.priority).toBe('')
})

// A membership nobody can untick is a write the sheet did not gate, so an id
// the client cannot yet name is ticked under the id itself. That is the window
// before the first poll lands.
test('a List the client cannot name is ticked under its own id', () => {
  opened({ title: 'Buy milk', intoLists: ['l1', 'unknown-id'] })
  const tick = screen.getByRole('checkbox', {
    name: 'unknown-id',
  }) as HTMLInputElement
  expect(tick.checked).toBe(true)
})

// Ticking and unticking are different fields on the wire, so an untick that
// sent nothing would leave the membership on.
test('unticking a List sends it as a removal', () => {
  const sheet = opened({ title: 'Buy milk', intoLists: ['l1'] })
  fireEvent.click(screen.getByRole('checkbox', { name: 'Home' }))
  const body = sheet.submit()
  expect(body.outOfLists).toEqual(['l1'])
  expect(body.intoLists).toEqual([])
})

// A create takes the memberships whole, so a create prefilled from a Task
// passes `{}` as the baseline: diffing against the draft would send an empty
// difference and write a Task belonging to no List the sheet drew ticked.
test('a create prefilled from a Task sends the ticked Lists whole', () => {
  const draft: TaskBody = { title: 'Buy milk', intoLists: ['l1'] }
  const sheet = opened(draft, {})
  const body = sheet.submit()
  expect(body.intoLists).toEqual(['l1'])
})

// The wire names a pair by its key, so emptying a value is the store's own way
// of removing the pair rather than a delete this side invents.
test('emptying a value keeps the key, mapped to empty', () => {
  const sheet = opened({ title: 'Buy milk', fields: { url: 'example.com' } })
  fireEvent.change(screen.getByLabelText('url'), { target: { value: '' } })
  const body = sheet.submit()
  expect(body.fields).toEqual({ url: '' })
})

test('a pair added on the blank row is sent with the rest', () => {
  const sheet = opened({ title: 'Buy milk', fields: { url: 'example.com' } })
  fireEvent.change(screen.getByPlaceholderText('name'), {
    target: { value: 'aisle' },
  })
  fireEvent.change(screen.getByPlaceholderText('value'), {
    target: { value: '7' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Add' }))
  const body = sheet.submit()
  expect(body.fields).toEqual({ url: 'example.com', aisle: '7' })
})
