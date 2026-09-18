// The controls over the list: which Tasks it asks for and what order they come
// back in. The TUI reached these with `s`, `h` and `l`; here they sit above the
// list, because a phone has no keys to hang them off.
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
        on. The empty option is the everyday view rather than a fifth sort or a
        List called nothing, so it says what leaving it does.
      */}
      <select
        className="control"
        aria-label="Sort"
        value={narrowing.sort}
        onChange={(event) =>
          onChange({ ...narrowing, sort: event.target.value })
        }
      >
        <option value="">Newest first</option>
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

        It is what puts an ended Task back within reach: reopen is offered on
        every Task, but a completed one is not in the everyday read to be
        tapped, so this is the way to it.
      */}
      <button
        type="button"
        className={narrowing.all ? 'control on' : 'control'}
        aria-pressed={narrowing.all}
        onClick={() => onChange({ ...narrowing, all: !narrowing.all })}
      >
        {narrowing.all ? 'Ended shown' : 'Show ended'}
      </button>
    </div>
  )
}
