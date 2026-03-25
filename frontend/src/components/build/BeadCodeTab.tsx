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

// ── File tree helpers ────────────────────────────────────────────────────────

interface TreeNode {
  name: string;
  path: string;
  isFile: boolean;
  children: TreeNode[];
}

function buildTree(paths: string[]): TreeNode[] {
  const root: TreeNode[] = [];
  for (const raw of paths) {
    // Split on either slash style but preserve original path as leaf path
    const parts = raw.split(/[/\\]/);
    let current = root;
    let built = '';
    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      // Reconstruct path using the same separator as found in original
      const sep = raw.includes('/') ? '/' : '\\';
      built = built ? `${built}${sep}${part}` : part;
      const isFile = i === parts.length - 1;
      // For leaf nodes, use the original raw path so it matches files array exactly
      const nodePath = isFile ? raw : built;
      let node = current.find(n => n.name === part);
      if (!node) {
        node = { name: part, path: nodePath, isFile, children: [] };
        current.push(node);
      }
      current = node.children;
    }
  }
  return root;
}

interface TreeNodeProps {
  node: TreeNode;
  selectedPath: string;
  onSelect: (path: string) => void;
  depth: number;
}

function TreeNodeItem({ node, selectedPath, onSelect, depth }: TreeNodeProps) {
  const [open, setOpen] = useState(true);
  const indent = depth * 12 + 8;

  if (node.isFile) {
    const isSelected = selectedPath === node.path ||
      selectedPath.replace(/\\/g, '/') === node.path;
    return (
      <div
        className={`bead-tree-item bead-tree-file${isSelected ? ' bead-tree-item--selected' : ''}`}
        style={{ paddingLeft: `${indent}px` }}
        onClick={() => onSelect(node.path)}
        title={node.path}
      >
        <span className="bead-tree-icon bead-tree-icon--file">&#x1F4C4;</span>
        <span className="bead-tree-name">{node.name}</span>
      </div>
    );
  }

  return (
    <div>
      <div
        className="bead-tree-item bead-tree-dir"
        style={{ paddingLeft: `${indent}px` }}
        onClick={() => setOpen(o => !o)}
      >
        <span className="bead-tree-icon bead-tree-icon--arrow">{open ? '▾' : '▸'}</span>
        <span className="bead-tree-name">{node.name}</span>
      </div>
      {open && node.children.map(child => (
        <TreeNodeItem
          key={child.path}
          node={child}
          selectedPath={selectedPath}
          onSelect={onSelect}
          depth={depth + 1}
        />
      ))}
    </div>
  );
}

// ── Main component ───────────────────────────────────────────────────────────

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

  useEffect(() => {
    setLoading(true);
    fetchFiles().finally(() => setLoading(false));
  }, [beadId]); // eslint-disable-line react-hooks/exhaustive-deps

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

  const treePaths = files.length > 0 ? files.map(f => f.path) : targetFiles;
  const tree = buildTree(treePaths);

  const selectedFile = files.find(f => f.path === selectedPath);
  const selectedDiff = diffs.find(d => d.path === selectedPath);
  const language = selectedFile?.language ?? selectedDiff?.language ?? 'plaintext';

  return (
    <div className="bead-code-tab">
      {/* Left: file tree */}
      <div className="bead-code-tree">
        {loading && <div className="bead-code-tree-loading">loading…</div>}
        {tree.map(node => (
          <TreeNodeItem
            key={node.path}
            node={node}
            selectedPath={selectedPath}
            onSelect={setSelectedPath}
            depth={0}
          />
        ))}
      </div>

      {/* Right: toolbar + editor */}
      <div className="bead-code-main">
        <div className="bead-code-toolbar">
          <div className="bead-code-status">
            {isLive && (
              <span className={`bead-code-live-badge${liveIndicator ? ' bead-code-live-badge--pulse' : ''}`}>
                LIVE
              </span>
            )}
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

        <div className="bead-code-editor">
          {viewMode === 'source' || !canDiff ? (
            <Editor
              key={selectedPath}
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
              key={selectedPath}
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
    </div>
  );
}
