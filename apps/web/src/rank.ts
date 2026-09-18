// The order the Tags are offered in: by how often they are carried, with
// enough variation that the list does not read the same every time.
//
// The operator asked for discovery rather than a stable ranking. A Tag carried
// twice should usually sit above one carried once, and should sometimes sit
// below it, so a Tag filed against years ago surfaces where an ordering by
// count alone would bury it forever.
//
// This is the one thing the client works out that the store does not. The
// counts are the store's; what to do with them is a question about looking at
// a list, and nothing on the wire is asked to answer it.

import type { Collection } from './state'

/**
 * Efraimidis-Spirakis weighted sampling without replacement: each Collection
 * draws the key `u^(1/w)` for a uniform `u` and its weight `w`, and the keys
 * are ranked. The expected order is the frequency order, and every other order
 * has a chance proportional to how close the counts are.
 *
 * A Tag nothing carries still has a weight, so it can surface.
 */
export function drawKey(count: number, random: () => number): number {
  return Math.pow(random(), 1 / (count + 1))
}

/**
 * The Collections in drawn order, keying any that have not been drawn yet.
 *
 * `keys` is carried by the caller and outlives a poll on purpose. The draw is
 * once per Collection rather than once per read: a list re-weighted every
 * second would move between seeing a Tag and reaching it, which is the one way
 * discovery turns into an annoyance. A Tag created while the client is open is
 * keyed the first time it arrives and holds that place afterwards.
 *
 * `keys` is written to rather than replaced, because what it holds is the
 * order somebody is already looking at.
 */
export function ranked(
  all: Collection[],
  keys: Map<string, number>,
  random: () => number,
): Collection[] {
  for (const one of all) {
    if (!keys.has(one.id)) keys.set(one.id, drawKey(one.count, random))
  }
  // Descending by key, and stably, so two drawing the same key keep the order
  // they were counted in rather than swapping about.
  return [...all].sort((a, b) => (keys.get(b.id) ?? 0) - (keys.get(a.id) ?? 0))
}
