// The controls over the list: which Tasks it asks for and what order they come
// back in. On a phone they sit above the list, where a thumb reaches them; at a
// desk they are the pane to its left. Which of the two is CSS's to decide, and
// this file draws the same controls either way.
//
// The box the list is searched in is here too, as `Search`, and it is drawn
// apart from the rest: it belongs over the Tasks rather than among the
// filtering, which is what puts it in the middle pane at a desk.
//
// Nothing here filters anything. Each control sets one field of the Narrowing
// and the next poll asks the API again, so the list on the screen is always a
// list the store described rather than one this side sifted.

import { drawnKeys, ranked } from './rank'
import type { Collection, Narrowing, Offered } from './state'

export function Narrow({
  narrowing,
  offered,
  onChange,
}: {
  narrowing: Narrowing
  /**
   * The Lists, the Tags and the sorts to narrow and order by, as
   * `GET /api/state` answered them. The client keeps no list of its own, so a
   * sort added to the store is offered here the day it lands and one this side
   * invented cannot be offered at all. The colors and the snoozes travel in the
   * same type and are the sheet's rather than these controls'.
   */
  offered: Offered
  onChange: (narrowing: Narrowing) => void
}) {
  // The Tags in drawn order. The keys outlive this component on purpose: it is
  // unmounted whenever another screen is open, and `rank.ts` is where the
  // lifetime and the reason for it are argued.
  const tags = ranked(offered.tags, drawnKeys, Math.random)

  return (
    <div className="narrow">
      {/*
        Native selects, because a phone already knows how to open one under a
        thumb and a written-out menu would be a control kit nobody has decided
        on. Each empty option says what leaving it does rather than naming
        itself: there is no List called nothing.

        Sending no sort is the store's own default, which is `created`
        ascending, so the empty option and `By created` order the list the same
        way. Hiding that would mean knowing here which of the served sorts the
        store falls back to, which is the copy this side is not allowed to
        keep.
      */}
      <select
        className="control"
        aria-label="Sort"
        value={narrowing.sort}
        onChange={(event) =>
          onChange({ ...narrowing, sort: event.target.value })
        }
      >
        <option value="">Oldest first</option>
        {offered.sorts.map((sort) => (
          <option key={sort} value={sort}>
            By {sort}
          </option>
        ))}
      </select>

      <Picker
        name="List"
        every="Every list"
        all={offered.lists}
        value={narrowing.list}
        onPick={(list) => onChange({ ...narrowing, list })}
      />

      <Tags
        all={tags}
        on={narrowing.tags}
        onToggle={(tags) => onChange({ ...narrowing, tags })}
      />

      {/*
        One button and not four. `?all=true` takes in the snoozed, the
        completed, the declined and the deleted together, and offering four
        switches over a route with one flag would be this side inventing a
        distinction the store does not make.

        It says everything rather than ended for the same reason: a deletion is
        not an ending, and a Task somebody deleted turning up under a button
        that promised the finished ones would be the label lying about what it
        did.

        It is what puts a Task that is no longer in the everyday read back
        within reach: reopen is offered on every Task, but a completed one is
        not there to be tapped, so this is the way to it.
      */}
      <button
        type="button"
        className={narrowing.all ? 'control on' : 'control'}
        aria-pressed={narrowing.all}
        onClick={() => onChange({ ...narrowing, all: !narrowing.all })}
      >
        {narrowing.all ? 'Everything shown' : 'Show everything'}
      </button>
    </div>
  )
}

/**
 * The Tags, each a switch. More than one can be on, and a Task carrying any of
 * them is in the list: clicking a second Tag is somebody widening what they
 * are willing to look at, not narrowing twice.
 *
 * Switches rather than the `<select>` the List uses. A multiple `<select>` is
 * the platform's control for this and it is a drag on a phone, where the rule
 * is one thumb: these are 44px each and one tap turns one on. It is also the
 * shape the filtering pane wants, which is where these are headed.
 *
 * The Lists are offered as the read listed them and these are not. A List is
 * somewhere a Task is filed and there are few of them, so a stable order is
 * what makes one easy to reach; Tags accumulate, and an order by count alone
 * would bury an old one forever. `rank.ts` is where that difference is argued.
 *
 * A Tag the client cannot name is still drawn, under its id, for the reason
 * `Picker` keeps one: a Tag deleted from another surface while the list is
 * narrowed to it would otherwise leave the list narrowed with nothing on the
 * screen to turn off.
 */
function Tags({
  all,
  on,
  onToggle,
}: {
  all: Collection[]
  on: string[]
  onToggle: (tags: string[]) => void
}) {
  const named = new Set(all.map((one) => one.id))
  const shown = [
    ...all.map((one) => one.id),
    ...on.filter((id) => !named.has(id)),
  ]
  if (shown.length === 0) return null
  return (
    <div className="tags" role="group" aria-label="Tags">
      {shown.map((id) => {
        const lit = on.includes(id)
        return (
          <button
            key={id}
            type="button"
            className={lit ? 'control on' : 'control'}
            aria-pressed={lit}
            onClick={() =>
              onToggle(lit ? on.filter((one) => one !== id) : [...on, id])
            }
          >
            {labelled(all, id)}
          </button>
        )
      })}
    </div>
  )
}

/**
 * The List picker. A Collection is the same shape whichever set it comes from,
 * and this is the one that is picked one at a time.
 *
 * An id the client cannot name is offered under the id itself rather than left
 * matching nothing. A Collection deleted from another surface while the list is
 * narrowed to it would otherwise blank the control while the list stayed
 * narrowed, so the screen would show a wide-open picker over a narrowed list
 * and nothing would say what happened. It is the rule `Sheet.tsx` already
 * follows for a level it does not recognise and for a membership it cannot yet
 * put a name to.
 */
function Picker({
  name,
  every,
  all,
  value,
  onPick,
}: {
  name: string
  /** What the empty option says, which is what leaving it does. */
  every: string
  all: Collection[]
  value: string
  onPick: (value: string) => void
}) {
  const shown = all.map((one) => one.id)
  if (value !== '' && !shown.includes(value)) shown.push(value)
  return (
    <select
      className="control"
      aria-label={name}
      value={value}
      onChange={(event) => onPick(event.target.value)}
    >
      <option value="">{every}</option>
      {shown.map((id) => (
        <option key={id} value={id}>
          {labelled(all, id)}
        </option>
      ))}
    </select>
  )
}

/**
 * The box the list is searched in. It sits above the Tasks rather than among
 * the filtering controls, because it is about the list it is over: at a desk
 * the two are in different panes, and on a phone they are the same column.
 *
 * Typing asks again: each keystroke is a new Narrowing and so a new read, which
 * on a LAN is a request the store answers off one query. Holding the text back
 * until somebody stops typing would mean a list that lags the box it is being
 * searched from, and a timer here to decide when typing stopped.
 *
 * The store is what matches, so this sends the text and nothing else. It
 * searches a Task's own Title, Description and Why, which the store decides and
 * this side does not restate on the screen.
 */
export function Search({
  value,
  onChange,
}: {
  value: string
  onChange: (search: string) => void
}) {
  return (
    <input
      className="control search"
      type="search"
      aria-label="Search"
      placeholder="Search"
      value={value}
      onChange={(event) => onChange(event.target.value)}
    />
  )
}

/**
 * What one option or button reads. A Collection the last read named carries the count the
 * store worked out; an id nothing named reads as itself and carries no count at
 * all, because a zero here would be the client answering a question the store
 * never answered — the Collection may well have Tasks under it, and this side
 * has no way to know.
 */
function labelled(all: Collection[], id: string) {
  const named = all.find((one) => one.id === id)
  return named ? `${named.name} (${named.count})` : id
}
