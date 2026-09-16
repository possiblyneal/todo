// The one read a screen makes for itself, and the guard around it.
//
// The list is polled once and handed down; the detail and activity screens
// each read something the poll does not carry, on the revision the poll hands
// them. Both do it the same way, so the way is written once: a read in flight
// when the screen moves on is a read whose answer must not be drawn.

import { useEffect, useState } from 'react'

import { sentence } from './api'

/**
 * Reads whenever `on` changes, and answers with what came back and what went
 * wrong. `empty` is what there is to draw before the first answer.
 *
 * A read that came back clears the message the one before it left, so a
 * sentence never sits over entries that arrived after it, and an answer that
 * lands after the screen has moved on is dropped rather than drawn over the
 * newer one.
 */
export function useRead<T>(
  read: () => Promise<T>,
  empty: T,
  on: unknown[],
): { value: T; error: string | null } {
  const [value, setValue] = useState<T>(empty)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let live = true
    read()
      .then((answer) => {
        if (!live) return
        setValue(answer)
        setError(null)
      })
      .catch((caught: unknown) => {
        if (live) setError(sentence(caught))
      })
    return () => {
      live = false
    }
    // The caller says what this read is of, because only it knows: `read` is a
    // closure made afresh every render and depending on it would read forever.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, on)

  return { value, error }
}
