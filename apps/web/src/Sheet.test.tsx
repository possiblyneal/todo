// @vitest-environment jsdom

// The sheet's grammars, which are the part of it that cannot be read off the
// screen. Every other field is text in and text out; these three turn what was
// picked into a different thing on the wire, and getting one backwards is a
// write nobody sees go wrong.

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { Sheet } from './Sheet'
import * as state from './state'
import type { Offered } from './state'
import * as write from './write'
import type { TaskBody } from './write'

afterEach(cleanup)

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

// Making a List from here is a write of its own, and the point of it is that
// the Task lands in the List somebody just named. Ticking it separately would
// be the same two taps that leaving the sheet costs.
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
  expect((screen.getByLabelText('Deadline') as HTMLInputElement).value).toBe(
    '2026-03-04',
  )
  expect(sheet.submit().deadline).toBe('2026-03-04')
})

// The one thing the picker must not do. A phrase the store may yet read is the
// reason the box exists, and a picker reaching into it would blank or guess at
// that phrase, which is the failure this shape was chosen to avoid.
test('a phrase the picker cannot show is left in the box', () => {
  const sheet = opened({ title: 'Buy milk', deadline: 'next Friday' })
  expect((screen.getByLabelText('Deadline') as HTMLInputElement).value).toBe(
    'next Friday',
  )
  expect(sheet.submit().deadline).toBe('next Friday')
})

// The picker browses the machine that will resolve the path, because a
// pointer typed on a phone names a file on the host and the browser's own file
// input cannot answer with a directory at all. It fills the box and never reads
// it back, which is the rule the deadline's picker follows.
test('a file picked off the machine fills the attachment box', async () => {
  vi.spyOn(state, 'fetchFiles').mockImplementation((path?: string) =>
    Promise.resolve(
      path === undefined
        ? {
            path: '/home/neal',
            parent: '',
            entries: [
              { name: 'papers', dir: true },
              { name: 'note.txt', dir: false },
            ],
          }
        : {
            path,
            parent: '/home/neal',
            entries: [{ name: 'deed', dir: false }],
          },
    ),
  )
  const sheet = opened({ title: 'Move house' })

  fireEvent.click(screen.getByRole('button', { name: 'Browse' }))
  // A directory is somewhere to go and a file is something to point at, so the
  // first tap lists and the second fills.
  fireEvent.click(await screen.findByRole('button', { name: 'papers/' }))
  fireEvent.click(await screen.findByRole('button', { name: 'deed' }))

  const typed = screen.getByLabelText('New attachment')
  expect((typed as HTMLInputElement).value).toBe('/home/neal/papers/deed')
  // Filling the box is not attaching: the box is still the field, and Attach
  // is still what collects what is in it.
  expect(screen.queryByRole('button', { name: 'Remove' })).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'Attach' }))
  expect(sheet.submit().attachments).toEqual(['/home/neal/papers/deed'])
})

// The route refuses in its own words — a path outside the root it will look
// in, or one naming nothing — and the picker draws that rather than an empty
// directory, which would say the machine has nothing on it.
test('a refused listing is drawn in the API’s own words', async () => {
  vi.spyOn(state, 'fetchFiles').mockRejectedValue(
    new Error(
      '/etc is outside /home/neal, which is as far as this listener will look',
    ),
  )
  opened({ title: 'Move house' })

  fireEvent.click(screen.getByRole('button', { name: 'Browse' }))
  expect(
    await screen.findByText(
      '/etc is outside /home/neal, which is as far as this listener will look',
    ),
  ).toBeDefined()
})
