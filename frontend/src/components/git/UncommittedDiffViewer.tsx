import { useEffect, useState } from 'react';
import { DiffEditor } from '@monaco-editor/react';
import { getWorkingDiff } from '../../api/git';
import type { WorkingDiffEntry } from '../../api/git';

interface Props {
  projectId: string;
  onClose: () => void;
}

interface TreeNode {
  name: string;
  path: string;
  isFile: boolean;
  children: TreeNode[];
}

function buildTree(paths: string[]): TreeNode[] {
  const root: TreeNode[] = [];
  for (const raw of paths) {
    const parts = raw.split(/[/\\]/);
    let current = root;
    let built = '';
    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      const sep = raw.includes('/') ? '/' : '\\';
      built = built ? `${built}${sep}${part}` : part;
      const isFile = i === parts.length - 1;
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

function TreeItem({
  node,
  selectedPath,
  onSelect,
  depth,
}: {
  node: TreeNode;
  selectedPath: string;
  onSelect: (path: string) => void;
  depth: number;
}) {
  const [open, setOpen] = useState(true);
  const indent = depth * 12 + 8;

  if (node.isFile) {
    const isSelected = selectedPath === node.path;
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
        <TreeItem
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

export function UncommittedDiffViewer({ projectId, onClose }: Props) {
  const [entries, setEntries] = useState<WorkingDiffEntry[]>([]);
  const [selectedPath, setSelectedPath] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getWorkingDiff(projectId)
      .then(data => {
        setEntries(data);
        if (data.length > 0) setSelectedPath(data[0].path);
      })
      .catch(e => setError(String(e)))
      .finally(() => setLoading(false));
  }, [projectId]);

  const selected = entries.find(e => e.path === selectedPath);
  const tree = buildTree(entries.map(e => e.path));

  return (
    <div className="uncommitted-diff-overlay" onClick={e => e.stopPropagation()}>
      <div className="uncommitted-diff-header">
        <span className="uncommitted-diff-title">Uncommitted changes</span>
        <button className="uncommitted-diff-close" onClick={onClose}>✕</button>
      </div>
      <div className="uncommitted-diff-body">
        {loading && <div className="uncommitted-diff-loading">Loading…</div>}
        {error && <div className="uncommitted-diff-error">{error}</div>}
        {!loading && !error && (
          <>
            <div className="bead-code-tree uncommitted-diff-tree">
              {tree.map(node => (
                <TreeItem
                  key={node.path}
                  node={node}
                  selectedPath={selectedPath}
                  onSelect={setSelectedPath}
                  depth={0}
                />
              ))}
            </div>
            <div className="bead-code-editor uncommitted-diff-editor">
              {selected ? (
                <DiffEditor
                  key={selectedPath}
                  height="100%"
                  language={selected.language}
                  original={selected.original}
                  modified={selected.modified}
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
              ) : (
                <div className="bead-code-empty">Select a file to view its diff.</div>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
