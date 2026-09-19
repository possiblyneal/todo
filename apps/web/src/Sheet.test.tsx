// @vitest-environment jsdom

// The sheet's grammars, which are the part of it that cannot be read off the
// screen. Every other field is text in and text out; these three turn what was
// picked into a different thing on the wire, and getting one backwards is a
// write nobody sees go wrong.

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { Sheet } from './Sheet'
import type { Offered } from './state'
import * as write from './write'
import type { TaskBody } from './write'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const OFFERED: Offered = {
  lists: [{ id: 'l1', name: 'Home', color: 'blue', count: 2 }],
  tags: [{ id: 't1', name: 'errand', color: 'red', count: 1 }],
  sorts: ['title'],
  colors: ['red', 'blue'],
  snoozes: ['an hour', 'tomorrow'],
  priorities: [
    { name: 'low', example: 'It can wait a month.' },
    { name: 'high', example: 'Today.' },
  ],
  impacts: [{ name: 'med', example: 'One piece of work moves.' }],
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

// Five served sets arrive under one prop, so reading the wrong one off it is a
// mistake that can be made: the color and the snooze are both lists of strings
// and the compiler cannot tell them apart. This says which set feeds which.
test('the color and the snooze each offer their own served set', () => {
  opened({ title: 'Ship it' })
  const color = screen.getByLabelText('Color') as HTMLSelectElement
  expect([...color.options].map((o) => o.value)).toEqual(['', 'red', 'blue'])
  // Under its bare name: the levels are drawn beside what they mean and a
  // color has nothing to say for itself, so the same control draws both.
  expect([...color.options].map((o) => o.textContent)).toEqual([
    '—',
    'red',
    'blue',
  ])
  const snooze = screen.getByLabelText('Snooze') as HTMLSelectElement
  expect([...snooze.options].map((o) => o.value)).toEqual([
    'leave',
    'wake',
    'an hour',
    'tomorrow',
  ])
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

// Making a Tag from here is a write of its own, and the point of it is that
// the Task lands under the Tag somebody just named. Ticking it separately
// would be the same two taps that leaving the sheet costs.
test('a Tag made here is ticked and comes back as a membership', async () => {
  const made = vi.spyOn(write, 'addCollection').mockResolvedValue('t9')
  const sheet = opened({ title: 'Buy milk' })
  fireEvent.change(screen.getByLabelText('New Tag'), {
    target: { value: 'shopping' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Add Tag' }))
  // Under the word that was typed, not under the id: the poll that would name
  // it has not run, and an id on the screen is nothing anybody recognises.
  await screen.findByText('shopping')
  expect(made).toHaveBeenCalledWith('tags', { name: 'shopping' })
  expect(sheet.submit().addTags).toEqual(['t9'])
})

// A refusal is said in the sheet's own one place, and nothing is ticked: a
// membership to an id the store never minted would be submitted and refused
// again, with the first sentence gone by then.
test('a refused creation says so and ticks nothing', async () => {
  vi.spyOn(write, 'addCollection').mockRejectedValue(
    new Error('that name is taken'),
  )
  const sheet = opened({ title: 'Buy milk' })
  fireEvent.change(screen.getByLabelText('New List'), {
    target: { value: 'Home' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Add List' }))
  await screen.findByText('that name is taken')
  expect(sheet.submit().intoLists).toEqual([])
})

// The three words are the API's now, and the example beside each is why: it is
// what makes `high` mean the same thing to whoever is reading the form and to
// an Agent writing through the same call.
test('a level is offered under the example that says what it means', () => {
  opened({ title: 'Buy milk' })
  const priority = screen.getByLabelText('Priority') as HTMLSelectElement
  expect([...priority.options].map((one) => one.textContent)).toEqual([
    '—',
    'low — It can wait a month.',
    'high — Today.',
  ])
})

// A level the served set does not name is still offered, the rule every picker
// on this sheet follows. It has no example, because the route is what says what
// one means and it said nothing about this.
test('a level the route does not offer is kept, under its own name', () => {
  opened({ title: 'Buy milk', priority: 'urgent' })
  const priority = screen.getByLabelText('Priority') as HTMLSelectElement
  expect([...priority.options].map((one) => one.textContent)).toContain(
    'urgent',
  )
})

// An Attachment is collected and not written: the sheet is the gate, and a
// draft backed out of has to leave no pointer behind.
test('an attachment typed on the sheet comes back with the body', () => {
  const sheet = opened({ title: 'Buy milk' })
  fireEvent.change(screen.getByLabelText('New attachment'), {
    target: { value: '/home/neal/receipt.pdf' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Attach' }))
  expect(sheet.submit().attachments).toEqual(['/home/neal/receipt.pdf'])
})

// Collected means removable. Nothing was written, so taking one off the list
// is the list changing and not a detach.
test('a collected attachment is taken off before anything is written', () => {
  const sheet = opened({ title: 'Buy milk' })
  const typed = screen.getByLabelText('New attachment')
  fireEvent.change(typed, { target: { value: '/one' } })
  fireEvent.click(screen.getByRole('button', { name: 'Attach' }))
  fireEvent.change(typed, { target: { value: '/two' } })
  fireEvent.click(screen.getByRole('button', { name: 'Attach' }))
  fireEvent.click(screen.getAllByRole('button', { name: 'Remove' })[0]!)
  expect(sheet.submit().attachments).toEqual(['/two'])
})

// The picker writes into the box, which stays the field. A day picked is the
// same text somebody could have typed, and is still editable afterwards.
test('a picked date lands in the deadline box as text', () => {
  const sheet = opened({ title: 'Buy milk' })
  fireEvent.change(screen.getByLabelText('Pick a deadline'), {
    target: { value: '2026-03-04' },
  })
  expect(
    (screen.getByLabelText('Deadline as typed') as HTMLInputElement).value,
  ).toBe('2026-03-04')
  expect(sheet.submit().deadline).toBe('2026-03-04')
})

// The one thing the picker must not do. A phrase the API refuses is still the
// reason the box exists — the refusal comes back with the phrase still in the
// field — and a picker reaching into it would blank or guess at that phrase,
// which is the failure this shape was chosen to avoid.
test('a phrase the picker cannot show is left in the box', () => {
  const sheet = opened({ title: 'Buy milk', deadline: 'next Friday' })
  expect(
    (screen.getByLabelText('Deadline as typed') as HTMLInputElement).value,
  ).toBe('next Friday')
  expect(sheet.submit().deadline).toBe('next Friday')
})

// The same pointer twice is one pointer. It is also what keeps the rows keyed
// apart, since a row is keyed by the pointer it draws, and Remove filters by
// that same text: two rows of `/one` would be one key and one tap taking both.
test('the same attachment collected twice is collected once', () => {
  const sheet = opened({ title: 'Buy milk' })
  const typed = screen.getByLabelText('New attachment')
  fireEvent.change(typed, { target: { value: '/one' } })
  fireEvent.click(screen.getByRole('button', { name: 'Attach' }))
  fireEvent.change(typed, { target: { value: '/one' } })
  fireEvent.click(screen.getByRole('button', { name: 'Attach' }))
  expect(screen.getAllByRole('button', { name: 'Remove' })).toHaveLength(1)
  expect(sheet.submit().attachments).toEqual(['/one'])
})

// The picker blanking the box is the same failure from the other side: a
// native date input fires a change carrying the empty string when a keystroke
// clears it, and the phrase in the box is not the picker's to take away.
test('a cleared picker leaves the box alone', () => {
  const sheet = opened({ title: 'Buy milk' })
  const picker = screen.getByLabelText('Pick a deadline')
  fireEvent.change(picker, { target: { value: '2026-03-04' } })
  fireEvent.change(picker, { target: { value: '' } })
  expect(
    (screen.getByLabelText('Deadline as typed') as HTMLInputElement).value,
  ).toBe('2026-03-04')
  expect(sheet.submit().deadline).toBe('2026-03-04')
})

// The picker holds nothing of its own, which is the whole of "never reads the
// box". A control left holding the day it wrote would fire nothing when that
// same day is picked again, so somebody who typed over a picked date could not
// pick it back; empty after a pick is what makes the next one a change.
test('the picker holds nothing after it has written', () => {
  const sheet = opened({ title: 'Buy milk', deadline: 'next Friday' })
  const picker = screen.getByLabelText('Pick a deadline') as HTMLInputElement
  expect(picker.value).toBe('')

  fireEvent.change(picker, { target: { value: '2026-03-04' } })
  expect(picker.value).toBe('')
  fireEvent.change(screen.getByLabelText('Deadline as typed'), {
    target: { value: 'next Friday' },
  })
  fireEvent.change(picker, { target: { value: '2026-03-04' } })
  expect(sheet.submit().deadline).toBe('2026-03-04')
})

// A refusal belongs to the write somebody just made, and a Collection that was
// made is the write they just made. Left standing, the sentence says the wrong
// thing about the tick that appeared beside it.
test('a refusal is taken down by the creation that follows it', async () => {
  const made = vi.spyOn(write, 'addCollection')
  made.mockRejectedValueOnce(new Error('that name is taken'))
  opened({ title: 'Buy milk' })
  const box = screen.getByLabelText('New List')
  fireEvent.change(box, { target: { value: 'Home' } })
  fireEvent.click(screen.getByRole('button', { name: 'Add List' }))
  await screen.findByText('that name is taken')

  made.mockResolvedValueOnce('l9')
  fireEvent.change(box, { target: { value: 'Garden' } })
  fireEvent.click(screen.getByRole('button', { name: 'Add List' }))
  await screen.findByText('Garden')
  expect(screen.queryByText('that name is taken')).toBeNull()
})

// The box is cleared by the name going out, not by the answer coming back: a
// name typed while the request was in flight is the next one somebody means to
// make, and it is theirs rather than this screen's to throw away.
test('a name typed while the creation is in flight survives it', async () => {
  let land: (id: string) => void = () => {}
  vi.spyOn(write, 'addCollection').mockReturnValue(
    new Promise<string>((resolve) => {
      land = resolve
    }),
  )
  opened({ title: 'Buy milk' })
  const box = screen.getByLabelText<HTMLInputElement>('New Tag')
  fireEvent.change(box, { target: { value: 'shopping' } })
  fireEvent.click(screen.getByRole('button', { name: 'Add Tag' }))
  fireEvent.change(box, { target: { value: 'errands' } })
  land('t9')
  await screen.findByText('shopping')
  expect(box.value).toBe('errands')
})
