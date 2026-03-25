import { useEffect, useMemo } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  useNodesState,
  useEdgesState,
  type Node,
  type Edge,
} from '@xyflow/react';
import dagre from '@dagrejs/dagre';
import '@xyflow/react/dist/style.css';
import type { Bead, BeadGraph } from '../../types';

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

const NODE_WIDTH_EPIC = 240;
const NODE_HEIGHT_EPIC = 70;
const NODE_WIDTH_TASK = 200;
const NODE_HEIGHT_TASK = 56;

function getNodeStyle(bead: Bead, isReady: boolean) {
  const isEpic = bead.type === 'epic';
  const isBlocked = bead.status === 'blocked';

  let color: string;
  let bg = '#1e293b';
  let textColor = '#e2e8f0';
  let opacity = 1;
  let boxShadow: string;

  if (isBlocked) {
    color = '#334155';
    bg = '#111827';
    textColor = '#475569';
    opacity = 0.5;
    boxShadow = 'none';
  } else if (bead.status === 'closed') {
    color = '#22c55e';
    boxShadow = `0 0 0 1px ${color}22`;
  } else if (bead.status === 'in_progress') {
    color = '#3b82f6';
    boxShadow = 'none'; // pulsed via CSS animation class
  } else if (bead.status === 'reviewing') {
    color = '#ef4444';
    boxShadow = 'none'; // pulsed via CSS animation class
  } else if (isReady) {
    color = '#f59e0b';
    boxShadow = `0 0 8px 2px ${color}55`;
  } else {
    color = '#475569';
    boxShadow = `0 0 0 1px ${color}22`;
  }

  return {
    background: bg,
    border: `2px solid ${color}`,
    borderRadius: isEpic ? '10px' : '6px',
    color: textColor,
    fontSize: isEpic ? '13px' : '12px',
    fontWeight: isEpic ? '700' : '400',
    padding: '8px 12px',
    width: isEpic ? NODE_WIDTH_EPIC : NODE_WIDTH_TASK,
    opacity,
    boxShadow,
    cursor: 'pointer',
  };
}

function layoutGraph(beads: Bead[]): { nodes: Node[]; edges: Edge[] } {
  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  // LR for left-to-right within a rank; TB gives us the layered top-down view
  g.setGraph({ rankdir: 'TB', nodesep: 50, ranksep: 90 });

  const beadIds = new Set(beads.map(b => b.id));

  for (const bead of beads) {
    const w = bead.type === 'epic' ? NODE_WIDTH_EPIC : NODE_WIDTH_TASK;
    const h = bead.type === 'epic' ? NODE_HEIGHT_EPIC : NODE_HEIGHT_TASK;
    g.setNode(bead.id, { width: w, height: h });
  }

  const edges: Edge[] = [];

  // Epic → task membership edges (solid lines)
  for (const bead of beads) {
    if (bead.epicId && beadIds.has(bead.epicId)) {
      g.setEdge(bead.epicId, bead.id);
      edges.push({
        id: `epic:${bead.epicId}->${bead.id}`,
        source: bead.epicId,
        target: bead.id,
        style: { stroke: '#64748b', strokeWidth: 1.5 },
        animated: false,
      });
    }
  }

  // Task → task dependency edges (dotted lines)
  for (const bead of beads) {
    for (const depId of bead.deps ?? []) {
      if (!beadIds.has(depId)) continue;
      // avoid duplicating an edge already drawn as epic membership
      const edgeId = `dep:${depId}->${bead.id}`;
      g.setEdge(depId, bead.id);
      edges.push({
        id: edgeId,
        source: depId,
        target: bead.id,
        style: {
          stroke: '#94a3b8',
          strokeWidth: 1.5,
          strokeDasharray: '6 3',
        },
        animated: bead.status === 'in_progress' || bead.status === 'reviewing',
      });
    }
  }

  dagre.layout(g);

  // Compute ready set: open beads whose all deps are closed
  const closedIds = new Set(beads.filter(b => b.status === 'closed').map(b => b.id));
  const readySet = new Set(
    beads
      .filter(b => b.status === 'open' && (b.deps ?? []).every(d => closedIds.has(d)))
      .map(b => b.id),
  );

  const nodes: Node[] = beads.flatMap(bead => {
    const pos = g.node(bead.id);
    if (!pos) return [];
    const w = bead.type === 'epic' ? NODE_WIDTH_EPIC : NODE_WIDTH_TASK;
    const h = bead.type === 'epic' ? NODE_HEIGHT_EPIC : NODE_HEIGHT_TASK;
    const isReady = readySet.has(bead.id);
    const nodeClass =
      bead.status === 'in_progress' ? 'bead-node-active' :
      bead.status === 'reviewing'   ? 'bead-node-reviewing' :
      undefined;
    return [{
      id: bead.id,
      position: { x: pos.x - w / 2, y: pos.y - h / 2 },
      className: nodeClass,
      data: {
        label: (
          <div style={{ lineHeight: 1.3 }}>
            <div style={{ fontSize: '10px', opacity: 0.6, marginBottom: 2 }}>
              {bead.type.toUpperCase()} · {isReady ? 'ready' : bead.status.replace('_', ' ')}
              {(bead.tokens ?? 0) > 0 && ` · ${formatTokens(bead.tokens!)} tok`}
            </div>
            <div>{bead.title}</div>
          </div>
        ),
      },
      style: getNodeStyle(bead, isReady),
    }];
  });

  return { nodes, edges };
}

function AgentIcon({ delay }: { delay: number }) {
  return (
    <svg
      width="12" height="14" viewBox="0 0 12 14"
      className="agent-activity-icon"
      style={{ animationDelay: `${delay.toFixed(2)}s` }}
    >
      {/* antenna */}
      <line x1="6" y1="0" x2="6" y2="2" stroke="#3b82f6" strokeWidth="1" />
      <circle cx="6" cy="1" r="0.8" fill="#3b82f6" />
      {/* head */}
      <rect x="2" y="2" width="8" height="6" rx="1.5" fill="#3b82f6" />
      {/* eyes */}
      <circle cx="4.2" cy="5" r="1" fill="#0f172a" />
      <circle cx="7.8" cy="5" r="1" fill="#0f172a" />
      {/* body */}
      <rect x="3" y="9" width="6" height="4" rx="1" fill="#3b82f6" />
    </svg>
  );
}

function DevilIcon({ delay }: { delay: number }) {
  return (
    <svg
      width="12" height="14" viewBox="0 0 12 14"
      className="agent-activity-icon"
      style={{ animationDelay: `${delay.toFixed(2)}s` }}
    >
      {/* left horn */}
      <path d="M3 3 L1.5 0 L4.5 2" fill="#ef4444" />
      {/* right horn */}
      <path d="M9 3 L10.5 0 L7.5 2" fill="#ef4444" />
      {/* head */}
      <rect x="2" y="2" width="8" height="6" rx="1.5" fill="#ef4444" />
      {/* eyes */}
      <circle cx="4.2" cy="5" r="1" fill="#0f172a" />
      <circle cx="7.8" cy="5" r="1" fill="#0f172a" />
      {/* body */}
      <rect x="3" y="9" width="6" height="4" rx="1" fill="#ef4444" />
      {/* tail */}
      <path d="M9 11 Q11.5 9.5 10.5 13" stroke="#ef4444" strokeWidth="1.2" fill="none" strokeLinecap="round" />
    </svg>
  );
}

interface Props {
  graph: BeadGraph;
  onBeadClick?: (beadId: string) => void;
}

export function BeadGraph({ graph, onBeadClick }: Props) {
  const activeCount = graph.beads.filter(b => b.status === 'in_progress').length;
  const reviewingCount = graph.beads.filter(b => b.status === 'reviewing').length;
  const totalTokens = graph.beads.reduce((sum, b) => sum + (b.tokens ?? 0), 0);

  const { nodes: layoutNodes, edges: layoutEdges } = useMemo(
    () => layoutGraph(graph.beads),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [
      graph.beads.length,
      graph.beads.map(b => `${b.status}:${b.epicId ?? ''}:${(b.deps ?? []).join('|')}:${b.tokens ?? 0}`).join(','),
    ],
  );

  const [nodes, setNodes, onNodesChange] = useNodesState(layoutNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(layoutEdges);

  useEffect(() => {
    setNodes(layoutNodes);
    setEdges(layoutEdges);
  }, [layoutNodes, layoutEdges, setNodes, setEdges]);

  if (graph.beads.length === 0) {
    return (
      <div className="bead-graph-empty">
        <p>No beads generated yet.</p>
      </div>
    );
  }

  return (
    <div className="bead-graph-container">
      <div className="bead-graph-legend">
        <span className="bead-legend-item bead-legend-ready">● ready</span>
        <span className="bead-legend-item bead-legend-progress">● in progress</span>
        <span className="bead-legend-item bead-legend-done">● done</span>
        <span className="bead-legend-item bead-legend-blocked">● blocked</span>
        <span className="bead-legend-sep" />
        <span className="bead-legend-item bead-legend-edge-member">— member of epic</span>
        <span className="bead-legend-item bead-legend-edge-dep">╌ depends on</span>
        {activeCount > 0 && (
          <span className="agent-activity-indicator">
            {Array.from({ length: activeCount }, (_, i) => (
              <AgentIcon key={i} delay={i * 0.15} />
            ))}
          </span>
        )}
        {reviewingCount > 0 && (
          <span className="agent-activity-indicator">
            {Array.from({ length: reviewingCount }, (_, i) => (
              <DevilIcon key={i} delay={i * 0.15} />
            ))}
          </span>
        )}
        {totalTokens > 0 && (
          <span className="bead-token-total">{formatTokens(totalTokens)} tok</span>
        )}
      </div>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeClick={(_event, node) => onBeadClick?.(node.id)}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={!!onBeadClick}
        proOptions={{ hideAttribution: true }}
      >
        <Background color="#334155" gap={16} />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
