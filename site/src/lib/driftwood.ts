/* A client for a Driftwood instance's control plane, for the try-it-out page.
 *
 * Everything here talks to the same origin the page was served from. That is
 * not a convenience: the control plane's CORS allowlist is deliberately
 * localhost-only (internal/server/server.go, isAllowedOrigin), so a page on any
 * other origin cannot call it at all and would be refused before it started.
 * The deployment that makes this work is nginx serving the static site at / and
 * proxying /_driftwood/* to the binary — see deploy/nginx.conf.
 *
 * No request here mutates anything the operator did not ask for: the two writes
 * are "set the mock simulator's mode" and "send one request at the mock
 * simulator". Both are confined to the simulator Driftwood serves itself.
 */

export const CONTROL_PREFIX = '/_driftwood';

/** The simulator's only meaningful endpoint. It is served by the proxy before
 *  any dial is attempted (proxy.go, IsMockPath), so it works with no target
 *  configured and with the target down. */
export const MOCK_PATH = `${CONTROL_PREFIX}/mock/users`;

export type MockMode =
  | 'NORMAL'
  | 'TYPE_BREAK'
  | 'MISSING_FIELD'
  | 'NULL_BREAK'
  | 'ADDED_FIELD';

/** pkg/types/types.go — the four values ContractStatus can take. NO_BASELINE
 *  is included because a request to an endpoint with no stored contract really
 *  does report it, and the UI should be able to say so rather than showing a
 *  blank where a status goes. */
export type ContractStatus = 'NO_BASELINE' | 'MATCH' | 'WARNING' | 'BREAKING';

export type DiffDelta = {
  json_path: string;
  kind: string;
  severity: 'BREAKING' | 'WARNING' | 'INFO' | string;
  message: string;
  expected: string;
  actual: string;
};

export type ContractDiff = {
  has_breaking_changes: boolean;
  has_warnings: boolean;
  deltas: DiffDelta[] | null;
};

export type CapturedTraffic = {
  id: string;
  method: string;
  path: string;
  url: string;
  status_code: number;
  duration_ms: number;
  contract_status: ContractStatus;
  diff?: ContractDiff | null;
};

export type AlertEvent = {
  traffic_id: string;
  endpoint: string;
  contract_status: ContractStatus;
  diff?: ContractDiff | null;
};

/** An SSE frame. The stream sends no `event:` line, so the type is the JSON
 *  payload's own `type` field — see internal/events/events.go, SSEHandler. The
 *  one exception is the initial `event: ping`, which EventSource does not
 *  deliver to onmessage, so it never reaches this type. */
export type EventMessage =
  | { type: 'traffic'; timestamp: string; data: CapturedTraffic }
  | { type: 'alert'; timestamp: string; data: AlertEvent }
  | { type: string; timestamp: string; data: unknown };

/**
 * Reads a JSON response, refusing anything that is not one.
 *
 * This is the check that a plain `res.ok` cannot make. A static host with an
 * SPA fallback answers an unknown /_driftwood/api/* path with 200 and the
 * site's own index.html, so `ok` is true, the body is HTML, and a page that
 * trusted the status code would report a live instance and then fail on the
 * first real call with a parse error. Every asset answering 200 with the page
 * itself is a failure this project has already shipped once.
 */
async function readJSON<T>(res: Response): Promise<T> {
  const text = await res.text();
  try {
    return JSON.parse(text) as T;
  } catch {
    throw new Error(
      `expected JSON from ${res.url} but got ${res.headers.get('content-type') || 'no content type'}`
    );
  }
}

/**
 * Is there a Driftwood control plane on this origin?
 *
 * `/api/traffic` is used as the probe rather than any dedicated endpoint: it is
 * the cheapest real call that only the control plane answers, and its reply is
 * JSON, which is what makes the check above meaningful.
 */
export async function probe(): Promise<boolean> {
  try {
    const res = await fetch(`${CONTROL_PREFIX}/api/traffic`, {
      headers: { Accept: 'application/json' },
    });
    if (!res.ok) return false;
    await readJSON<unknown>(res);
    return true;
  } catch {
    return false;
  }
}

/** Sets the simulator's mode. The server echoes the mode it is now in, which is
 *  what this returns — the authority is the server's answer, not the request. */
export async function setMockMode(mode: MockMode): Promise<MockMode> {
  const res = await fetch(`${CONTROL_PREFIX}/api/mock/mode`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mode }),
  });
  if (!res.ok) throw new Error(`setting mock mode failed: ${res.status}`);
  const body = await readJSON<{ mode: MockMode }>(res);
  return body.mode;
}

/**
 * Sends one request at the simulator, through the proxy, so it is recorded and
 * diffed exactly like traffic to a real API.
 *
 * The response body is returned as text rather than parsed. It is the payload
 * the diff was computed from, and showing it verbatim — including the mode
 * where it is deliberately malformed — is the point.
 */
export async function sendMockRequest(): Promise<{ status: number; body: string }> {
  const res = await fetch(MOCK_PATH, { headers: { Accept: 'application/json' } });
  return { status: res.status, body: await res.text() };
}

/**
 * Opens the event stream and calls back for every frame.
 *
 * Returns the unsubscribe function rather than the EventSource: a caller that
 * holds the source can close it twice or reopen it, and the failure that causes
 * — a second stream whose events are handled by the first stream's handler — is
 * silent. Handing back only the way to stop it makes that unrepresentable.
 */
export function openStream(onEvent: (msg: EventMessage) => void): () => void {
  const source = new EventSource(`${CONTROL_PREFIX}/events`);
  source.onmessage = (m) => {
    try {
      onEvent(JSON.parse(m.data) as EventMessage);
    } catch {
      /* A frame that is not JSON is not something this page can render. The
         stream is allowed to carry event types this page has never heard of,
         so a parse failure here is not worth tearing the stream down for. */
    }
  };
  return () => source.close();
}
