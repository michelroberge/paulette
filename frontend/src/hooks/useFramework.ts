import { useState, useCallback, useEffect } from 'react';
import { getFramework, setFramework } from '../api/framework';
import type { FrameworkConfig, FrameworkId } from '../types';

const DEFAULT_CONFIG: FrameworkConfig = { framework: 'tailwind' };

export function useFramework(projectId: string | null) {
  const [config, setConfig] = useState<FrameworkConfig>(DEFAULT_CONFIG);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!projectId) return;
    getFramework(projectId)
      .then(setConfig)
      .catch(() => setConfig(DEFAULT_CONFIG));
  }, [projectId]);

  const update = useCallback(async (framework: FrameworkId, customName?: string) => {
    if (!projectId) return;
    setSaving(true);
    try {
      const saved = await setFramework(projectId, { framework, customName });
      setConfig(saved);
    } finally {
      setSaving(false);
    }
  }, [projectId]);

  return { config, saving, update };
}
