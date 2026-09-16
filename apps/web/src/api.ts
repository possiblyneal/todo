// The one place a response becomes either what it carries or an Error saying
// why it did not. `apps/todo/src/api/api.go` is the side that decides those
// words: a refusal's body is `{"error": "<the sentence the CLI would print>"}`,
// so the sentence is thrown as it is rather than restated here.

/** The Error a failed response carries, in the API's own words. */
export async function refused(response: Response): Promise<Error> {
  const body: unknown = await response.json().catch(() => null)
  const sentence =
    body &&
    typeof body === 'object' &&
    'error' in body &&
    typeof body.error === 'string'
      ? body.error
      : `the API answered ${response.status}`
  return new Error(sentence)
}

/**
 * Sends one JSON body and reads one back. Every route that takes a body
 * answers with one, so there is no empty-response case to tell apart.
 */
export async function send<T>(
  method: string,
  path: string,
  body: unknown,
): Promise<T> {
  const response = await fetch(path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!response.ok) throw await refused(response)
  return (await response.json()) as T
}
