import { useEffect, useRef, useState } from 'react';
import { listVersions } from '../../api/versions';
import type { VersionListEntry } from '../../types';

interface Props {
  projectId: string;
  currentVersion: string;
  onSelect: (version: string) => void;
  onClose: () => void;
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
  } catch {
    return iso;
  }
}

export function VersionSelectorDropdown({ projectId, currentVersion, onSelect, onClose }: Readonly<Props>) {
  const [versions, setVersions] = useState<VersionListEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    listVersions(projectId)
      .then(setVersions)
      .catch(() => setVersions([]))
      .finally(() => setLoading(false));
  }, [projectId]);

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onClose();
      }
    }
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [onClose]);

  return (
    <div className="version-dropdown" ref={ref}>
      <div className="version-dropdown-header">Version history</div>
      {loading ? (
        <div className="version-dropdown-item version-dropdown-loading">Loading…</div>
      ) : versions.length === 0 ? (
        <div className="version-dropdown-item version-dropdown-empty">No versions found</div>
      ) : (
        versions.map(v => {
          const isCurrent = v.version === currentVersion;
          return (
            <button
              key={v.version}
              className={`version-dropdown-item${isCurrent ? ' version-dropdown-current' : ''}`}
              onClick={() => {
                if (!isCurrent) onSelect(v.version);
                else onClose();
              }}
            >
              <span className="version-dropdown-ver">v{v.version}</span>
              {v.hasMeta && v.iteration != null && (
                <span className="version-dropdown-iter"> iter {v.iteration}</span>
              )}
              {isCurrent && <span className="version-dropdown-tag"> (current)</span>}
              {v.hasMeta && v.approvedAt && (
                <span className="version-dropdown-date">{formatDate(v.approvedAt)}</span>
              )}
            </button>
          );
        })
      )}
    </div>
  );
}
