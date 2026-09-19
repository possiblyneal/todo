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
 * The keys drawn so far, by Collection id, for as long as the page is loaded.
 *
 * It is here rather than inside a component because the lifetime is the page's
 * and not any one screen's. `Narrow` does not stay mounted across every screen,
 * so a draw held in its state would be thrown away and redrawn on the way back
 * — the reshuffle this exists to prevent, at the granularity somebody actually
 * navigates at.
 *
 * `ranked` takes a map as an argument all the same, so a test draws into one of
 * its own and this one stays out of it.
 */
export const drawnKeys = new Map<string, number>()

/**
 * The Collections in drawn order, keying any that have not been drawn yet.
 *
 * The draw is once per Collection rather than once per read: the poll runs
 * every second, and a list re-weighted that often would move between seeing a
 * Tag and reaching it. A Tag created while the client is open is keyed the
 * first time it arrives and holds that place afterwards.
 *
 * `keys` is written to rather than replaced, because what it holds is the
 * order somebody is already looking at.
 */
export function ranked(
  all: Collection[],
  keys: Map<string, number>,
  random: () => number,
): Collection[] {
  const keyed = all.map((one) => {
    let key = keys.get(one.id)
    if (key === undefined) {
      key = drawKey(one.count, random)
      keys.set(one.id, key)
    }
    return { one, key }
  })
  // Descending by key, and stably, so two drawing the same key keep the order
  // they were counted in rather than swapping about.
  keyed.sort((a, b) => b.key - a.key)
  return keyed.map((each) => each.one)
}
