import { useEffect, useRef, useState } from 'react';
import Editor, { DiffEditor } from '@monaco-editor/react';
import { getBeadFiles, getBeadDiff } from '../../api/beads';
import type { BeadFileEntry, BeadDiffEntry, BeadStatus } from '../../types';

interface Props {
  projectId: string;
  beadId: string;
  beadStatus: BeadStatus;
  targetFiles: string[];
}

export function BeadCodeTab({ projectId, beadId, beadStatus, targetFiles }: Props) {
  const [files, setFiles] = useState<BeadFileEntry[]>([]);
  const [diffs, setDiffs] = useState<BeadDiffEntry[]>([]);
  const [selectedPath, setSelectedPath] = useState<string>('');
  const [viewMode, setViewMode] = useState<'source' | 'diff'>('source');
  const [loading, setLoading] = useState(false);
  const [liveIndicator, setLiveIndicator] = useState(false);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const isLive = beadStatus === 'in_progress';
  const canDiff = beadStatus === 'closed' || beadStatus === 'reviewing';

  const fetchFiles = async () => {
    if (targetFiles.length === 0) return;
    try {
      const resp = await getBeadFiles(projectId, beadId);
      setFiles(resp.files);
      if (!selectedPath && resp.files.length > 0) {
        setSelectedPath(resp.files[0].path);
      }
      if (isLive) {
        setLiveIndicator(true);
        setTimeout(() => setLiveIndicator(false), 800);
      }
    } catch (err) {
      console.error('Failed to load bead files:', err);
    }
  };

  const fetchDiff = async () => {
    if (targetFiles.length === 0) return;
    try {
      const resp = await getBeadDiff(projectId, beadId);
      setDiffs(resp.diffs);
    } catch (err) {
      console.error('Failed to load bead diff:', err);
    }
  };

  // Initial load
  useEffect(() => {
    setLoading(true);
    fetchFiles().finally(() => setLoading(false));
  }, [beadId]); // eslint-disable-line react-hooks/exhaustive-deps

  // Poll every 5s when in_progress
  useEffect(() => {
    if (isLive) {
      pollRef.current = setInterval(fetchFiles, 5000);
    } else {
      if (pollRef.current) {
        clearInterval(pollRef.current);
        pollRef.current = null;
      }
    }
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, [isLive, beadId]); // eslint-disable-line react-hooks/exhaustive-deps

  // Load diffs when switching to diff view
  useEffect(() => {
    if (viewMode === 'diff' && canDiff && diffs.length === 0) {
      fetchDiff();
    }
  }, [viewMode]); // eslint-disable-line react-hooks/exhaustive-deps

  if (targetFiles.length === 0) {
    return (
      <div className="bead-code-empty">
        <span style={{ color: '#64748b', fontStyle: 'italic' }}>No target files specified for this bead.</span>
      </div>
    );
  }

  const selectedFile = files.find(f => f.path === selectedPath);
  const selectedDiff = diffs.find(d => d.path === selectedPath);
  const language = selectedFile?.language ?? selectedDiff?.language ?? 'plaintext';

  return (
    <div className="bead-code-tab">
      {/* Toolbar */}
      <div className="bead-code-toolbar">
        <div className="bead-code-file-selector">
          <select
            value={selectedPath}
            onChange={e => setSelectedPath(e.target.value)}
            className="bead-code-select"
          >
            {(files.length > 0 ? files.map(f => f.path) : targetFiles).map(path => (
              <option key={path} value={path}>{path}</option>
            ))}
          </select>
          {isLive && (
            <span className={`bead-code-live-badge${liveIndicator ? ' bead-code-live-badge--pulse' : ''}`}>
              LIVE
            </span>
          )}
          {loading && <span className="bead-code-loading-indicator">loading…</span>}
        </div>

        {canDiff && (
          <div className="bead-code-view-toggle">
            <button
              className={`bead-code-toggle-btn${viewMode === 'source' ? ' bead-code-toggle-btn--active' : ''}`}
              onClick={() => setViewMode('source')}
            >
              Source
            </button>
            <button
              className={`bead-code-toggle-btn${viewMode === 'diff' ? ' bead-code-toggle-btn--active' : ''}`}
              onClick={() => setViewMode('diff')}
            >
              Diff
            </button>
          </div>
        )}
      </div>

      {/* Editor area */}
      <div className="bead-code-editor">
        {viewMode === 'source' || !canDiff ? (
          <Editor
            height="100%"
            language={language}
            value={selectedFile?.content ?? ''}
            theme="vs-dark"
            options={{
              readOnly: true,
              minimap: { enabled: false },
              scrollBeyondLastLine: false,
              fontSize: 13,
              lineNumbers: 'on',
              wordWrap: 'on',
              renderWhitespace: 'none',
            }}
            loading={<div className="bead-code-editor-loading">Loading editor…</div>}
          />
        ) : (
          <DiffEditor
            height="100%"
            language={language}
            original={selectedDiff?.original ?? ''}
            modified={selectedDiff?.modified ?? ''}
            theme="vs-dark"
            options={{
              readOnly: true,
              minimap: { enabled: false },
              scrollBeyondLastLine: false,
              fontSize: 13,
              renderSideBySide: true,
            }}
            loading={<div className="bead-code-editor-loading">Loading diff…</div>}
          />
        )}
      </div>
    </div>
  );
}
