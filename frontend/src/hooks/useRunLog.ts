import { useState, useEffect, useCallback, useRef } from 'react';
import type { RunLogEntry } from '../api/runlog';
import { getProjectRunLog } from '../api/runlog';

const POLL_INTERVAL_MS = 10_000;

export function useRunLog(projectId: string | null) {
  const [entries, setEntries] = useState<RunLogEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const refresh = useCallback(async () => {
    if (!projectId) return;
    try {
      const data = await getProjectRunLog(projectId);
      setEntries(data);
    } catch {
      // silently ignore — stale data is fine
    }
  }, [projectId]);

  useEffect(() => {
    if (!projectId) {
      setEntries([]);
      return;
    }

    setLoading(true);
    refresh().finally(() => setLoading(false));

    // Poll while any entry is running
    function scheduleNext(data: RunLogEntry[]) {
      if (timerRef.current) clearTimeout(timerRef.current);
      const hasRunning = data.some(e => e.status === 'running');
      if (hasRunning) {
        timerRef.current = setTimeout(async () => {
          const updated = await getProjectRunLog(projectId).catch(() => data);
          setEntries(updated);
          scheduleNext(updated);
        }, POLL_INTERVAL_MS);
      }
    }

    getProjectRunLog(projectId!).then(data => {
      setEntries(data);
      scheduleNext(data);
    }).catch(() => {});

    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [projectId]); // eslint-disable-line react-hooks/exhaustive-deps

  return { entries, loading, refresh };
}
