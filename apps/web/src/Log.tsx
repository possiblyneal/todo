// The Change History drawn as a log: who did it, what they did and when. It is
// the reason the log is append-only made visible, and it is where an Agent's
// unattended writes show up as somebody's writes rather than as changes that
// merely appeared.

import { what, who } from './log'
import type { Entry } from './state'

export function Log({
  entries,
  nameOf,
}: {
  entries: Entry[]
  /**
   * What to call the Task an entry is about, where the log spans more than one.
   * An id nothing can name is drawn as the id: a Task deleted last week is
   * still what the entry is about.
   */
  nameOf?: (subject: string) => string
}) {
  if (entries.length === 0) return <p className="message">Nothing yet.</p>
  return (
    <ul className="log">
      {entries.map((entry) => (
        <li key={entry.seq} className="entry">
          <span className="did">{what(entry.kind)}</span>
          {nameOf && <span className="about">{nameOf(entry.subject)}</span>}
          <Who actor={entry.actor} />
          <When at={entry.at} />
        </li>
      ))}
    </ul>
  )
}

/**
 * The Actor, split on the first slash the way `CONTEXT.md` writes one: the
 * harness it ran under and the model it was. A name with no slash is drawn as
 * it is, because that is a legal Actor and hiding it would make a badly named
 * Agent disappear from the one screen that exists to show it.
 */
function Who({ actor }: { actor: string }) {
  const said = who(actor)
  return (
    <span className="who">
      {said.name}
      {said.harness && <span className="harness"> {said.harness}</span>}
    </span>
  )
}

/**
 * When it happened, in the reader's own time zone. The wire carries UTC and
 * this is the one place that is turned into a local reading of it.
 */
function When({ at }: { at: string }) {
  const when = new Date(at)
  return (
    <time className="when" dateTime={at}>
      {isNaN(when.getTime()) ? at : when.toLocaleString()}
    </time>
  )
}
