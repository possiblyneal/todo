import { expect, test } from 'vitest'

import { isAgent, isWrite, what, who } from './log'
import type { Entry } from './state'

function entry(kind: string, actor = 'neal'): Entry {
  return { seq: 1, at: '2026-09-16T10:00:00Z', actor, kind, subject: 'task_a' }
}

// CONTEXT.md: an Agent names itself <harness>/<model>, a person is their bare
// login. The split is on the first slash, and nothing is recognised by name.
test('an actor is split on the first slash and nothing else', () => {
  expect(who('claude-code/opus-5')).toEqual({
    harness: 'claude-code',
    name: 'opus-5',
  })
  expect(who('neal')).toEqual({ name: 'neal' })

  // A model named with a slash of its own keeps it: the harness is what comes
  // before the first one, and the rest is the name whatever is in it.
  expect(who('harness/vendor/model')).toEqual({
    harness: 'harness',
    name: 'vendor/model',
  })
})

test('an actor with no slash is still an actor', () => {
  expect(isAgent('claude-code/opus-5')).toBe(true)
  // One run with no TODO_ACTOR, or anything appended before the convention
  // existed. The filter narrows and never hides, so this is not an agent and
  // is still in the unfiltered view.
  expect(isAgent('some-agent')).toBe(false)
})

// The Lease bookkeeping brackets every guarded write under the writer's own
// Actor, so the activity screen is two thirds plumbing without this.
test('lease bookkeeping is not activity', () => {
  expect(isWrite(entry('lease_taken'))).toBe(false)
  expect(isWrite(entry('lease_released'))).toBe(false)
  expect(isWrite(entry('lease_broken'))).toBe(false)
  expect(isWrite(entry('task_completed'))).toBe(true)
})

// A kind added to the store is drawn the day it is appended: this client keeps
// no table of what each one means, so it cannot describe one wrongly.
test('a kind is drawn in the store own words', () => {
  expect(what('task_completed')).toBe('task completed')
  expect(what('something_nobody_has_written_yet')).toBe(
    'something nobody has written yet',
  )
})
