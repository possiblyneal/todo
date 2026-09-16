// What a Change History entry says, worked out without recognising anything by
// name. The kinds are the store's words and the Actor is whatever was appended,
// so nothing here is a copy of a list that could go stale -- with the one
// exception below, which the screen's own rule needs.

import type { Entry } from './state'

/**
 * The Lease bookkeeping that brackets every guarded write. It is not activity:
 * it is the same Actor's own write said three times, so the activity screen
 * drops these or it is two thirds plumbing.
 *
 * This is the one place the client names a kind. `apps/todo/src/store/store.go`
 * is where they are defined, and a fourth added there is drawn here rather than
 * hidden, which is the safe direction for the copy to be stale in.
 */
const PLUMBING = ['lease_taken', 'lease_released', 'lease_broken']

/** Whether an entry is a write somebody made rather than the Lease around it. */
export function isWrite(entry: Entry): boolean {
  return !PLUMBING.includes(entry.kind)
}

/**
 * Who wrote it, on the terms `CONTEXT.md` sets out under Actor: an Agent names
 * itself `<harness>/<model>`, and a person is their bare login. The split is on
 * the first slash, and a name with none is drawn as it is.
 *
 * Nothing is recognised by name. A harness this has never heard of and a model
 * released tomorrow both read correctly, because neither is looked up.
 */
export function who(actor: string): { harness?: string; name: string } {
  const slash = actor.indexOf('/')
  if (slash === -1) return { name: actor }
  return { harness: actor.slice(0, slash), name: actor.slice(slash + 1) }
}

/** Whether the Actor named itself the way an Agent does. */
export function isAgent(actor: string): boolean {
  return actor.includes('/')
}

/**
 * What was done, in the store's own word for it. The kind is drawn rather than
 * translated: a kind added to the store appears here the day it is appended,
 * and this client cannot describe one of them wrongly because it describes
 * none of them.
 */
export function what(kind: string): string {
  return kind.replaceAll('_', ' ')
}
