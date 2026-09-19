// The ranking's grammar: what the draw does to the order, and what it refuses
// to do to it on a second read.

import { expect, test } from 'vitest'

import { drawKey, ranked } from './rank'
import type { Collection } from './state'

function tag(id: string, count: number): Collection {
  return { id, name: id, color: '', count }
}

/** A random that answers the given numbers in turn, then repeats the last. */
function feeding(...values: number[]): () => number {
  let i = 0
  return () => values[Math.min(i++, values.length - 1)] ?? 0
}

// The weight is what makes the same uniform draw a bigger key for a Tag
// carried more often, which is the whole of why the expected order is the
// frequency order.
test('a Tag carried more often draws a higher key from the same uniform', () => {
  expect(drawKey(9, () => 0.5)).toBeGreaterThan(drawKey(0, () => 0.5))
})

// The variation is the point: a rare Tag drawing well beats a common one
// drawing badly, which is how something forgotten surfaces at all.
test('a rare Tag that draws well outranks a common one that draws badly', () => {
  const order = ranked(
    [tag('common', 50), tag('rare', 0)],
    new Map(),
    feeding(0.0001, 0.9),
  )
  expect(order.map((one) => one.id)).toEqual(['rare', 'common'])
})

// Once per Tag, not once per read. The poll runs every second, and a list that
// re-drew each time would move under whoever was reading it.
test('a second read does not draw again', () => {
  const tags = [tag('a', 1), tag('b', 1)]
  const keys = new Map<string, number>()
  const first = ranked(tags, keys, feeding(0.1, 0.9))
  // A random that throws would fail the test if anything drew a second time.
  const second = ranked(tags, keys, () => {
    throw new Error('the ranking drew again')
  })
  expect(second.map((one) => one.id)).toEqual(first.map((one) => one.id))
})

// A Tag made while the client is open has no key yet, and gets one then rather
// than being left unranked at whichever end the sort happens to put it. The
// draws are picked so the later Tag outranks the earlier one: a key the second
// read handed out but did not rank by would leave the order the other way
// round.
test('a Tag arriving later is keyed and ranked by that key', () => {
  const keys = new Map<string, number>()
  ranked([tag('old', 1)], keys, feeding(0.5))
  const order = ranked([tag('old', 1), tag('new', 1)], keys, feeding(0.7))
  expect(order.map((one) => one.id)).toEqual(['new', 'old'])
})

test('the ranking returns every Collection it was given', () => {
  const tags = [tag('a', 3), tag('b', 0), tag('c', 7)]
  const order = ranked(tags, new Map(), feeding(0.2, 0.4, 0.6))
  expect(order.map((one) => one.id).sort()).toEqual(['a', 'b', 'c'])
})
