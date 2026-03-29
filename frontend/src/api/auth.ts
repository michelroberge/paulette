import { apiFetch, apiStreamUrl } from './client';

export interface AuthStatus {
  authenticated: boolean;
  account?: string;
}

export function getAuthStatus(): Promise<AuthStatus> {
  return apiFetch<AuthStatus>('/auth/status');
}

export function logout(): Promise<void> {
  return apiFetch('/auth/logout', { method: 'POST' }).then(() => undefined);
}

export function sendLoginCode(code: string): Promise<void> {
  return apiFetch('/auth/login/input', {
    method: 'POST',
    headers: { 'Content-Type': 'text/plain' },
    body: code,
  }).then(() => undefined);
}

/** Opens an SSE connection to /api/auth/login and calls onLine for each output
 *  line the claude CLI emits.  Returns a cleanup function to close the stream. */
export function startLogin(
  onLine: (line: string) => void,
  onDone: () => void,
  onError: (msg: string) => void,
): () => void {
  const url = apiStreamUrl('/auth/login');
  const es = new EventSource(url);

  es.onmessage = (e) => {
    try {
      const msg = JSON.parse(e.data) as { type: string; line?: string };
      if (msg.type === 'output' && msg.line) {
        onLine(msg.line);
      } else if (msg.type === 'done') {
        es.close();
        onDone();
      } else if (msg.type === 'error') {
        es.close();
        onError(msg.line ?? 'auth error');
      }
    } catch {
      // ignore malformed events
    }
  };

  es.onerror = () => {
    // EventSource auto-reconnects; the backend will create a fresh session
    // if the previous one finished.  Don't surface transient reconnects as
    // errors — only report if the connection is permanently closed.
    if (es.readyState === EventSource.CLOSED) {
      onError('Connection lost — please reopen the login panel.');
    }
  };

  return () => es.close();
}
