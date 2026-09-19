// What a Task carries, drawn as a list of what it says. It is a read and
// nothing else: no control, no Lease, no write.
//
// It is one component because two screens draw the same Task. The detail
// screen draws the Task as it is now, and the history draws it as one entry
// left it, and a Task that read differently on the two would be the client
// disagreeing with itself about what the store said.

import { nameOf, type Collection, type Offered, type Task } from './state'
import { estimate } from './write'

export function Attributes({
  task,
  offered,
}: {
  task: Task
  /** The Lists and Tags the last read named, for putting a name to an id. */
  offered: Offered
}) {
  return (
    <dl className="carried">
      <Carried name="Description" value={task.description} />
      <Carried name="Why" value={task.why} />
      <Carried name="Deadline" value={task.deadline} />
      <Carried name="Estimate" value={estimate(task.estimateSeconds)} />
      <Carried name="Priority" value={task.priority} />
      <Carried name="Impact" value={task.impact} />
      <Carried name="Color" value={task.color} />
      <Carried name="Lists" value={named(task.lists, offered.lists)} />
      <Carried name="Tags" value={named(task.tags, offered.tags)} />
      <Carried name="Series" value={task.series} />
      <Carried name="Created" value={task.createdAt} />
      {Object.entries(task.fields ?? {}).map(([name, value]) => (
        <Carried key={name} name={name} value={value} />
      ))}
    </dl>
  )
}

function Carried({ name, value }: { name: string; value?: string }) {
  if (!value) return null
  return (
    <>
      <dt>{name}</dt>
      <dd>{value}</dd>
    </>
  )
}

/**
 * The Lists or Tags a Task carries, by name where the read named them. An id
 * the read did not name is drawn as the id rather than dropped, for the same
 * reason the sheet ticks one: a membership nobody can see is one nobody can
 * take off.
 */
function named(ids: string[] | undefined, all: Collection[]): string {
  return (ids ?? []).map((id) => nameOf(all, id)).join(', ')
}
