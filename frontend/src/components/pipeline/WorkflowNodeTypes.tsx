import { Handle, Position, type NodeProps } from '@xyflow/react';

// All node types include Left/Right handles so loopback edges can swing wide.
// Handles use ids: top, bottom (default), left-in, left-out, right-in, right-out.

const STAGE_COLORS: Record<string, string> = {
  vision: '#3b82f6',
  ux: '#8b5cf6',
  architecture: '#f59e0b',
  build: '#22c55e',
  complete: '#14b8a6',
  begin: '#64748b',
};

const HANDLE_STYLE_HIDDEN: React.CSSProperties = { background: 'transparent', border: 'none', width: 6, height: 6 };

// ── Stage node ─────────────────────────────────────────────────────

interface StageNodeData {
  label: string;
  stage: string;
  hasPrompt: boolean;
  selected?: boolean;
  isCurrent?: boolean;
  [key: string]: unknown;
}

export function StageNode({ data }: NodeProps) {
  const d = data as unknown as StageNodeData;
  const color = STAGE_COLORS[d.stage] ?? '#64748b';
  const cls = [
    'workflow-node workflow-node-stage',
    d.selected && 'workflow-node-selected',
    d.isCurrent && 'workflow-node-current',
  ].filter(Boolean).join(' ');
  return (
    <div className={cls} style={{ borderColor: color }}>

      <Handle type="target" position={Position.Top} id="top" style={{ background: color }} />
      <Handle type="target" position={Position.Left} id="left-in" style={HANDLE_STYLE_HIDDEN} />
      <Handle type="target" position={Position.Right} id="right-in" style={HANDLE_STYLE_HIDDEN} />
      <div className="workflow-node-label">{d.label}</div>
      {d.hasPrompt && <div className="workflow-node-badge" style={{ background: color }}>prompt</div>}
      <Handle type="source" position={Position.Bottom} id="bottom" style={{ background: color }} />
      <Handle type="source" position={Position.Left} id="left-out" style={HANDLE_STYLE_HIDDEN} />
      <Handle type="source" position={Position.Right} id="right-out" style={HANDLE_STYLE_HIDDEN} />
    </div>
  );
}

// ── Decision node (SVG diamond) ────────────────────────────────────

interface DecisionNodeData {
  label: string;
  selected?: boolean;
  isCurrent?: boolean;
  [key: string]: unknown;
}

export function DecisionNode({ data }: NodeProps) {
  const d = data as unknown as DecisionNodeData;
  const cls = [
    'workflow-node-decision-wrapper',
    d.selected && 'workflow-node-selected',
    d.isCurrent && 'workflow-node-current',
  ].filter(Boolean).join(' ');
  return (
    <div className={cls}>
      <Handle type="target" position={Position.Top} id="top" style={{ background: '#f59e0b' }} />
      <svg className="workflow-node-decision-svg" viewBox="0 0 120 60" xmlns="http://www.w3.org/2000/svg">
        <polygon
          points="60,2 118,30 60,58 2,30"
          fill="#1e293b"
          stroke="#f59e0b"
          strokeWidth="2"
        />
        <text x="60" y="34" textAnchor="middle" fill="#e2e8f0" fontSize="11" fontWeight="600">
          {d.label}
        </text>
      </svg>
      <Handle type="source" position={Position.Bottom} id="yes" style={{ background: '#22c55e', left: '30%' }} />
      <Handle type="source" position={Position.Bottom} id="no" style={{ background: '#ef4444', left: '70%' }} />
      {/* Side handles for wide loopbacks */}
      <Handle type="source" position={Position.Left} id="left-out" style={HANDLE_STYLE_HIDDEN} />
      <Handle type="source" position={Position.Right} id="right-out" style={HANDLE_STYLE_HIDDEN} />
      <Handle type="target" position={Position.Left} id="left-in" style={HANDLE_STYLE_HIDDEN} />
      <Handle type="target" position={Position.Right} id="right-in" style={HANDLE_STYLE_HIDDEN} />
    </div>
  );
}

// ── Refinement node ────────────────────────────────────────────────

interface RefinementNodeData {
  label: string;
  selected?: boolean;
  isCurrent?: boolean;
  [key: string]: unknown;
}

export function RefinementNode({ data }: NodeProps) {
  const d = data as unknown as RefinementNodeData;
  const cls = [
    'workflow-node workflow-node-refinement',
    d.selected && 'workflow-node-selected',
    d.isCurrent && 'workflow-node-current',
  ].filter(Boolean).join(' ');
  return (
    <div className={cls}>
      <Handle type="target" position={Position.Top} id="top" style={{ background: '#a855f7' }} />
      <Handle type="target" position={Position.Left} id="left-in" style={HANDLE_STYLE_HIDDEN} />
      <Handle type="target" position={Position.Right} id="right-in" style={HANDLE_STYLE_HIDDEN} />
      <div className="workflow-node-label">{d.label}</div>
      <Handle type="source" position={Position.Bottom} id="bottom" style={{ background: '#a855f7' }} />
      <Handle type="source" position={Position.Left} id="left-out" style={HANDLE_STYLE_HIDDEN} />
      <Handle type="source" position={Position.Right} id="right-out" style={HANDLE_STYLE_HIDDEN} />
    </div>
  );
}

// ── Build phase node ───────────────────────────────────────────────

interface BeadPhaseNodeData {
  label: string;
  selected?: boolean;
  isCurrent?: boolean;
  [key: string]: unknown;
}

export function BeadPhaseNode({ data }: NodeProps) {
  const d = data as unknown as BeadPhaseNodeData;
  const cls = [
    'workflow-node workflow-node-bead-phase',
    d.selected && 'workflow-node-selected',
    d.isCurrent && 'workflow-node-current',
  ].filter(Boolean).join(' ');
  return (
    <div className={cls}>
      <Handle type="target" position={Position.Top} id="top" style={{ background: '#22c55e' }} />
      <Handle type="target" position={Position.Left} id="left-in" style={HANDLE_STYLE_HIDDEN} />
      <Handle type="target" position={Position.Right} id="right-in" style={HANDLE_STYLE_HIDDEN} />
      <div className="workflow-node-label">{d.label}</div>
      <Handle type="source" position={Position.Bottom} id="bottom" style={{ background: '#22c55e' }} />
      <Handle type="source" position={Position.Left} id="left-out" style={HANDLE_STYLE_HIDDEN} />
      <Handle type="source" position={Position.Right} id="right-out" style={HANDLE_STYLE_HIDDEN} />
    </div>
  );
}

// ── Merge node ─────────────────────────────────────────────────────

export function MergeNode({ data }: NodeProps) {
  const d = data as unknown as { selected?: boolean; [key: string]: unknown };
  return (
    <div className={`workflow-node-merge ${d.selected ? 'workflow-node-selected' : ''}`}>
      <Handle type="target" position={Position.Top} id="left" style={{ background: '#64748b', left: '25%' }} />
      <Handle type="target" position={Position.Top} id="right" style={{ background: '#64748b', left: '75%' }} />
      <Handle type="source" position={Position.Bottom} id="bottom" style={{ background: '#64748b' }} />
    </div>
  );
}

export const workflowNodeTypes = {
  stage: StageNode,
  decision: DecisionNode,
  refinement: RefinementNode,
  beadPhase: BeadPhaseNode,
  merge: MergeNode,
};
