// The controls over the list: which Tasks it asks for and what order they come
// back in. They sit above the list, where a thumb reaches them.
//
// Nothing here filters anything. Each control sets one field of the Narrowing
// and the next poll asks the API again, so the list on the screen is always a
// list the store described rather than one this side sifted.

import type { Collection, Narrowing } from './state'

export function Narrow({
  narrowing,
  lists,
  sorts,
  onChange,
}: {
  narrowing: Narrowing
  lists: Collection[]
  // What `?sort=` accepts, as `GET /api/state` answered it. The client keeps no
  // list of its own, so a sort added to the store is offered here the day it
  // lands and one this side invented cannot be offered at all.
  sorts: string[]
  onChange: (narrowing: Narrowing) => void
}) {
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
        {sorts.map((sort) => (
          <option key={sort} value={sort}>
            By {sort}
          </option>
        ))}
      </select>

      <select
        className="control"
        aria-label="List"
        value={narrowing.list}
        onChange={(event) =>
          onChange({ ...narrowing, list: event.target.value })
        }
      >
        <option value="">Every list</option>
        {lists.map((list) => (
          <option key={list.id} value={list.id}>
            {list.name} ({list.count})
          </option>
        ))}
      </select>

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
