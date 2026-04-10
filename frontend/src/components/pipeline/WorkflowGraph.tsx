import { useCallback, useMemo, useRef, useState } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  type Node,
  type Edge,
  type ReactFlowInstance,
} from '@xyflow/react';
import dagre from '@dagrejs/dagre';
import '@xyflow/react/dist/style.css';
import { workflowNodeTypes } from './WorkflowNodeTypes';
import { WorkflowPromptEditor, type PromptTarget } from './WorkflowPromptEditor';

// ── Node → prompt mapping ──────────────────────────────────────────

export const NODE_PROMPT_MAP: Record<string, string[]> = {
  'vision-chat':      ['stage-vision.md.tmpl'],
  'vision-refine':    ['stage-vision.md.tmpl'],
  'ref-summarize':    ['refinement-summarize.md.tmpl'],
  'ref-loop':         ['refinement-generate-questions.md.tmpl', 'refinement-extract-facts.md.tmpl', 'refinement-score-section.md.tmpl', 'refinement-coherence-check.md.tmpl', 'refinement-critique.md.tmpl', 'refinement-find-gaps.md.tmpl'],
  'ref-synthesize':   ['refinement-synthesize.md.tmpl'],
  'ux-chat':          ['stage-ux.md.tmpl', 'helper-framework-note.md.tmpl', 'helper-journey-id-note.md.tmpl'],
  'mock-specs':       ['mock-specs-synthesize.md.tmpl'],
  'mock-generate':    ['mock-generate.md.tmpl', 'mock-retry.md.tmpl'],
  'mock-extract':     [],
  'mock-plan':        ['mock-screen-planner.md.tmpl'],
  'mock-components':  ['mock-component-planner.md.tmpl', 'mock-component-generate.md.tmpl', 'mock-view-generate.md.tmpl'],
  'mock-assemble':    ['mock-styler.md.tmpl'],
  'arch-specs':       ['arch-specs-synthesize.md.tmpl'],
  'arch-chat':        ['stage-architecture.md.tmpl', 'helper-arch-id-note.md.tmpl', 'helper-validation-commands.md.tmpl'],
  'build-specs':      ['build-specs-synthesize.md.tmpl'],
  'build-chat':       ['stage-build.md.tmpl', 'helper-plan-id-note.md.tmpl'],
  'bead-parse':       ['bead-parse-build-plan.md.tmpl'],
  'bead-code-writer': ['bead-code-writer.md.tmpl'],
  'bead-review':      ['bead-devil-advocate.md.tmpl'],
  'summary':          ['summary.md.tmpl'],
  'enh-vision':       ['enhancement-vision.md.tmpl'],
  'enh-ux':           ['enhancement-ux.md.tmpl'],
  'enh-architecture': ['enhancement-architecture.md.tmpl'],
  'enh-build':        ['enhancement-build.md.tmpl'],
  'skill-analyze':    ['skill-analyze.md.tmpl'],
};

// ── Node → context artifacts (what gets injected into prompts) ─────

export const NODE_CONTEXT_MAP: Record<string, string[]> = {
  'vision-chat':      [],
  'vision-refine':    [],
  'ref-summarize':    [],
  'ref-loop':         [],
  'ref-synthesize':   [],
  'ux-chat':          ['vision.md'],
  'mock-specs':       ['vision.md', 'ux-design.md'],
  'mock-generate':    ['mock-specs.md'],
  'mock-plan':        ['mock-specs.md'],
  'mock-components':  ['mock-specs.md'],
  'mock-assemble':    [],
  'mock-extract':     [],
  'arch-specs':       ['vision.md', 'ux-design.md'],
  'arch-chat':        ['arch-specs.md (or vision.md + ux-design.md)'],
  'build-specs':      ['arch-specs.md', 'architecture.md', 'mock-specs.md'],
  'build-chat':       ['build-specs.md (or vision.md + ux-design.md + architecture.md)'],
  'bead-parse':       ['build-plan.md', 'architecture.md'],
  'bead-code-writer': ['vision.md', 'ux-design.md', 'architecture.md', 'build-plan.md'],
  'bead-review':      ['vision.md', 'ux-design.md', 'architecture.md', 'build-plan.md'],
  'summary':          ['vision.md', 'ux-design.md', 'architecture.md', 'build-plan.md'],
};

// ── Node descriptions (shown for ALL nodes, static/read-only) ──────

export const NODE_DESCRIPTION_MAP: Record<string, string> = {
  'begin':            'Entry point of the pipeline. The user creates a new project and provides an initial idea or description.',
  'vision-mode':      'Decision: does the user want guided vision refinement (structured Q&A loop that scores confidence per section) or free-form chat with the Vision agent? This is a toggle in the vision step.',
  'vision-chat':      'Free-form conversation with the Vision agent. The user discusses their product idea iteratively. The agent produces a structured vision document in an <artifact> envelope. The conversation loops until the user approves the artifact.',
  'vision-approved':  'Approval gate: the user reviews the vision artifact and either approves it (advancing to UX) or continues refining. Approval requires a non-empty artifact.',
  'ref-summarize':    'First step of guided refinement. The idea is summarized into exactly 2 sentences to establish a baseline. Designed for small models (1B–7B).',
  'ref-loop':         'Iterative refinement loop. Each iteration: ask all unanswered questions → extract facts (deterministic) → classify into sections (keyword heuristic) → append to sections → score confidence (bullet/word count) → coherence check (structural) → gap detection (template-based) → tension check (LLM). Repeats until all sections reach 0.85 confidence or max iterations.',
  'ref-converged':    'Decision: have all vision sections (Problem, Users, Features, UX, Metrics, Constraints, Out of Scope) reached the minimum confidence threshold of 0.85?',
  'ref-synthesize':   'Final step of guided refinement (LLM). Accumulated bullet-point facts across all sections are synthesized into a polished vision document. This is the only LLM call that produces the final artifact.',
  'vision-merge':     'Merge point where the guided and free-form vision paths rejoin before proceeding to UX Design.',
  'ux-chat':          'Conversation with the UX agent. The approved vision is injected as context. The agent proposes user journeys, screen descriptions, navigation flows, and interaction patterns. Each journey gets a JRN-v*-NNN ID. Framework preference (Tailwind, Bootstrap, etc.) is injected.',
  'ux-approved':      'Approval gate: the user reviews the UX design artifact. Approval promotes the artifact and triggers mock generation.',
  'mock-specs':       'Synthesis step: the AI condenses the Vision and UX Design artifacts into a single, concise Mock Specs document optimized for HTML mockup generation. The output focuses only on visual layout, screens, navigation, and interactions — omitting business rationale and architecture. If the synthesized document is smaller than the originals combined, it replaces them as the sole context for mock generation. Otherwise, the originals are used as fallback. Cached at {dataDir}/ux/mock-specs.md.',
  'mock-mode':        'Decision: which mock generation path to use? The simple path (single LLM call) is used for small artifacts. The orchestrated path (multi-step) is used when the provider is Ollama OR the UX artifact exceeds 6KB. The orchestrated path decomposes screens into components for smaller, focused LLM calls.',
  'mock-generate':    'Simple path: a single LLM call generates the full HTML mockup. The mock specs (or vision + UX) and framework instructions are sent as context. The response is validated for an HTML envelope. If invalid, the LLM is retried with a correction prompt (up to MaxRetries attempts).',
  'mock-extract':     'HTML extraction and validation. The LLM response is checked for an <htmlcontent> envelope or raw HTML block. If valid HTML is found, it is saved to {dataDir}/ux/mock.html. If not, the correction retry loop triggers.',
  'mock-plan':        'Orchestrated Step 1: Screen Planner. The LLM reads the mock specs and produces a JSON list of screens with IDs, titles, and descriptions. This decomposes the full UX into manageable units for parallel generation.',
  'mock-components':  'Orchestrated Step 2: Component Generation. For each screen, a Component Planner decomposes it into layout components (nav, sidebar, card, form, etc.), then per-component HTML is generated in parallel (max 3 workers). Each component gets framework-specific instructions.',
  'mock-assemble':    'Orchestrated Step 3: Deterministic Assembly. Screen fragments are combined into the final HTML document with tab navigation between screens. A CSS class sanitizer strips framework-incompatible classes. The result is saved to {dataDir}/ux/mock.html.',
  'mock-done':        'Merge point where both mock generation paths (simple and orchestrated) rejoin. The generated mock.html is available for preview.',
  'arch-specs':       'Synthesis step: the AI condenses the Vision and UX Design artifacts into a concise Architecture Input Specs document. Extracts functional requirements, data entities, API surface, integration points, and non-functional requirements — omitting visual design details. If the synthesis is smaller than the originals combined, it replaces them as context for the Architecture agent. Otherwise, the originals are used. Cached at {dataDir}/architecture/arch-specs.md.',
  'arch-chat':        'Conversation with the Architecture agent. Architecture Input Specs (or raw vision + UX) are injected as context. The agent defines tech stack, APIs, data models, system components, and validation commands. Each element gets an ARCH-v*-NNN ID cross-referencing journey IDs.',
  'arch-approved':    'Approval gate: the user reviews the architecture. On approval, dev/build/run validation commands are extracted from the artifact and stored on the project for later use in the build validation gate.',
  'build-specs':      'Synthesis step: the AI condenses the Architecture Input Specs (or vision + UX), the Architecture artifact, and the Mock Specs (screens & interactions) into a concise Build Planning Specs document. This gives the Build Planner a 360° view: what to build (architecture), what it looks like (mock specs), and how components relate. If the synthesis is smaller than the originals combined, it replaces them. Cached at {dataDir}/build/build-specs.md.',
  'build-chat':       'Conversation with the Build Planner agent. Build Planning Specs (or all prior raw artifacts) are injected as context. The agent produces a build plan with milestones, tasks, dependencies, and traceability references to JRN-* and ARCH-* IDs.',
  'build-approved':   'Approval gate: the user reviews the build plan. On approval, the plan is parsed into structured beads (epics + tasks) for automated execution.',
  'bead-parse':       'Parse the approved build plan (build.md) + architecture into structured JSON: epics with tasks, dependencies, tags, target files, and journey/architecture references. Uses a dedicated parser prompt with strict JSON output format.',
  'bead-code-writer': 'Agentic code generation for a single task (bead). The agent has Write, Edit, and Bash tool access. All 4 stage artifacts are injected as context, filtered by bead tags to reduce waste. Matched skill templates are injected when available.',
  'bead-review':      "Devil's Advocate review of the code just written. The reviewer checks completeness, correctness, integration, and whether the implementation matches the task description. Issues already tracked in the backlog are skipped.",
  'bead-lgtm':        "Decision: did the Devil's Advocate approve the implementation? If the response starts with 'LGTM', the bead is closed. Otherwise, a correction bead is created with the findings and re-executed (up to 3 iterations).",
  'bead-all-done':    'Decision: are all beads in the project closed? If yes, proceed to the validation gate. If no, pick the next ready bead (one whose dependencies are all closed) and execute it.',
  'bead-validate':    'Validation gate: run the dev/build/run commands extracted from the architecture artifact. If any command fails, a fix bead is created and executed, then validation retries (up to MaxValidationRetries).',
  'summary':          'Generate an iteration summary document from all 4 stage artifacts. The summary captures key decisions, architecture choices, build strategy, and suggests 3–5 concrete enhancements for the next iteration.',
  'summary-approve':  'Final approval: the user reviews the summary. Approval promotes all artifacts to docs/{version}/, creates a git tag, archives chat histories and bead graphs, and optionally creates a PR to the base branch.',
  'done':             'Pipeline complete. The project is fully documented and versioned. The user can start an enhancement iteration (which creates a new version and re-enters the pipeline with enhancement context).',
};

// ── Graph definition ───────────────────────────────────────────────

interface Props {
  target: PromptTarget;
  useRefinementLoop?: boolean;
  currentNodeId?: string;
}

function n(id: string, type: string, label: string, extra: Record<string, unknown> = {}): Node {
  return { id, type, data: { label, hasPrompt: !!NODE_PROMPT_MAP[id], ...extra }, position: { x: 0, y: 0 } };
}

function e(id: string, source: string, target: string, opts: Partial<Edge> = {}): Edge {
  return { id, source, target, type: 'smoothstep', ...opts };
}

function eDashed(id: string, source: string, target: string, label: string, color = '#64748b'): Edge {
  return {
    id, source, target, type: 'smoothstep',
    animated: true, label,
    style: { strokeDasharray: '5,5', stroke: color },
    labelStyle: { fill: color, fontSize: 10 },
  };
}

function buildGraph(_useRefinement?: boolean) {
  const nodes: Node[] = [];
  const edges: Edge[] = [];

  // ── Begin ──────────────────────────────────────────────────────
  nodes.push(n('begin', 'stage', 'Begin', { stage: 'begin' }));

  // ── Vision mode decision ───────────────────────────────────────
  nodes.push(n('vision-mode', 'decision', 'Guided?'));
  edges.push(e('e-begin-vmode', 'begin', 'vision-mode'));

  // ── Free path (chat) ──────────────────────────────────────────
  nodes.push(n('vision-chat', 'stage', 'Vision Chat', { stage: 'vision' }));
  nodes.push(n('vision-approved', 'decision', 'Approved?'));
  edges.push({ ...e('e-vmode-free', 'vision-mode', 'vision-chat'), sourceHandle: 'no', label: 'Free', labelStyle: { fill: '#94a3b8', fontSize: 10 } });
  edges.push(e('e-vchat-approved', 'vision-chat', 'vision-approved'));
  // Vision chat loop — swings RIGHT
  edges.push({ ...eDashed('e-vapproved-loop', 'vision-approved', 'vision-chat', 'refine', '#3b82f6'), sourceHandle: 'right-out', targetHandle: 'right-in' });

  // ── Guided path (refinement loop) ─────────────────────────────
  nodes.push(n('ref-summarize', 'refinement', 'Summarize Idea'));
  nodes.push(n('ref-loop', 'refinement', 'Q&A + Score Loop'));
  nodes.push(n('ref-converged', 'decision', 'Converged?'));
  nodes.push(n('ref-synthesize', 'refinement', 'Synthesize Vision'));

  edges.push({ ...e('e-vmode-guided', 'vision-mode', 'ref-summarize'), sourceHandle: 'yes', label: 'Guided', labelStyle: { fill: '#94a3b8', fontSize: 10 } });
  edges.push(e('e-rsum-rloop', 'ref-summarize', 'ref-loop'));
  edges.push(e('e-rloop-rconv', 'ref-loop', 'ref-converged'));
  edges.push({ ...e('e-rconv-synth', 'ref-converged', 'ref-synthesize'), sourceHandle: 'yes', label: 'yes', labelStyle: { fill: '#22c55e', fontSize: 10 } });
  // Q&A convergence loop — swings LEFT
  edges.push({ ...eDashed('e-rconv-loop', 'ref-converged', 'ref-loop', 'no', '#ef4444'), sourceHandle: 'left-out', targetHandle: 'left-in' });
  edges.push(e('e-rsynth-vapproved', 'ref-synthesize', 'vision-approved'));

  // ── Merge after vision ─────────────────────────────────────────
  nodes.push(n('vision-merge', 'merge', ''));
  edges.push({ ...e('e-vapproved-merge', 'vision-approved', 'vision-merge'), sourceHandle: 'yes', label: 'approved', labelStyle: { fill: '#22c55e', fontSize: 10 } });

  // ── UX Stage ──────────────────────────────────────────────────
  nodes.push(n('ux-chat', 'stage', 'UX Design', { stage: 'ux' }));
  nodes.push(n('ux-approved', 'decision', 'Approved?'));
  nodes.push(n('mock-specs', 'stage', 'Synthesize Specs', { stage: 'ux' }));
  nodes.push(n('mock-mode', 'decision', 'Large?'));
  // Simple path
  nodes.push(n('mock-generate', 'stage', 'Generate HTML', { stage: 'ux' }));
  nodes.push(n('mock-extract', 'beadPhase', 'Extract HTML'));
  // Orchestrated path
  nodes.push(n('mock-plan', 'beadPhase', 'Screen Planner'));
  nodes.push(n('mock-components', 'beadPhase', 'Component Gen'));
  nodes.push(n('mock-assemble', 'beadPhase', 'Assemble HTML'));
  // Merge
  nodes.push(n('mock-done', 'merge', ''));

  edges.push(e('e-vmerge-ux', 'vision-merge', 'ux-chat'));
  edges.push(e('e-ux-approved', 'ux-chat', 'ux-approved'));
  // UX loop — swings LEFT
  edges.push({ ...eDashed('e-ux-loop', 'ux-approved', 'ux-chat', 'refine', '#8b5cf6'), sourceHandle: 'left-out', targetHandle: 'left-in' });
  edges.push({ ...e('e-ux-specs', 'ux-approved', 'mock-specs'), sourceHandle: 'yes', label: 'approved', labelStyle: { fill: '#22c55e', fontSize: 10 } });
  edges.push(e('e-specs-mockmode', 'mock-specs', 'mock-mode'));
  // Simple path (small artifact)
  edges.push({ ...e('e-mockmode-gen', 'mock-mode', 'mock-generate'), sourceHandle: 'no', label: 'Simple', labelStyle: { fill: '#94a3b8', fontSize: 10 } });
  edges.push(e('e-gen-extract', 'mock-generate', 'mock-extract'));
  // Retry loop on HTML extraction failure — swings RIGHT
  edges.push({ ...eDashed('e-extract-retry', 'mock-extract', 'mock-generate', 'retry', '#ef4444'), sourceHandle: 'right-out', targetHandle: 'right-in' });
  edges.push(e('e-extract-done', 'mock-extract', 'mock-done'));
  // Orchestrated path (large artifact / Ollama)
  edges.push({ ...e('e-mockmode-plan', 'mock-mode', 'mock-plan'), sourceHandle: 'yes', label: 'Orchestrated', labelStyle: { fill: '#94a3b8', fontSize: 10 } });
  edges.push(e('e-plan-comp', 'mock-plan', 'mock-components'));
  edges.push(e('e-comp-asm', 'mock-components', 'mock-assemble'));
  edges.push(e('e-asm-done', 'mock-assemble', 'mock-done'));

  // ── Architecture Synthesis + Stage ─────────────────────────────
  nodes.push(n('arch-specs', 'stage', 'Synthesize Arch Specs', { stage: 'architecture' }));
  nodes.push(n('arch-chat', 'stage', 'Architecture', { stage: 'architecture' }));
  nodes.push(n('arch-approved', 'decision', 'Approved?'));

  edges.push(e('e-mock-archspecs', 'mock-done', 'arch-specs'));
  edges.push(e('e-archspecs-arch', 'arch-specs', 'arch-chat'));
  edges.push(e('e-arch-approved', 'arch-chat', 'arch-approved'));
  // Architecture loop — swings LEFT
  edges.push({ ...eDashed('e-arch-loop', 'arch-approved', 'arch-chat', 'refine', '#f59e0b'), sourceHandle: 'left-out', targetHandle: 'left-in' });

  // ── Build Synthesis + Stage ───────────────────────────────────
  nodes.push(n('build-specs', 'stage', 'Synthesize Build Specs', { stage: 'build' }));
  nodes.push(n('build-chat', 'stage', 'Build Plan', { stage: 'build' }));
  nodes.push(n('build-approved', 'decision', 'Approved?'));
  nodes.push(n('bead-parse', 'beadPhase', 'Parse Build Plan'));
  nodes.push(n('bead-code-writer', 'beadPhase', 'Code Writer'));
  nodes.push(n('bead-review', 'beadPhase', "Devil's Advocate"));
  nodes.push(n('bead-lgtm', 'decision', 'LGTM?'));
  nodes.push(n('bead-validate', 'beadPhase', 'Validation Gate'));
  nodes.push(n('bead-all-done', 'decision', 'All Beads Done?'));

  edges.push({ ...e('e-arch-buildspecs', 'arch-approved', 'build-specs'), sourceHandle: 'yes', label: 'approved', labelStyle: { fill: '#22c55e', fontSize: 10 } });
  edges.push(e('e-buildspecs-build', 'build-specs', 'build-chat'));
  edges.push(e('e-build-approved', 'build-chat', 'build-approved'));
  // Build plan loop — swings LEFT
  edges.push({ ...eDashed('e-build-loop', 'build-approved', 'build-chat', 'refine', '#22c55e'), sourceHandle: 'left-out', targetHandle: 'left-in' });
  edges.push({ ...e('e-build-parse', 'build-approved', 'bead-parse'), sourceHandle: 'yes', label: 'approved', labelStyle: { fill: '#22c55e', fontSize: 10 } });
  edges.push(e('e-parse-code', 'bead-parse', 'bead-code-writer'));
  edges.push(e('e-code-review', 'bead-code-writer', 'bead-review'));
  edges.push(e('e-review-lgtm', 'bead-review', 'bead-lgtm'));
  // LGTM issues loop — swings RIGHT
  edges.push({ ...eDashed('e-lgtm-fix', 'bead-lgtm', 'bead-code-writer', 'issues', '#ef4444'), sourceHandle: 'right-out', targetHandle: 'right-in' });
  edges.push({ ...e('e-lgtm-alldone', 'bead-lgtm', 'bead-all-done'), sourceHandle: 'yes', label: 'LGTM', labelStyle: { fill: '#22c55e', fontSize: 10 } });
  // All beads done loop — swings LEFT
  edges.push({ ...eDashed('e-alldone-next', 'bead-all-done', 'bead-code-writer', 'next bead', '#22c55e'), sourceHandle: 'left-out', targetHandle: 'left-in' });
  edges.push({ ...e('e-alldone-validate', 'bead-all-done', 'bead-validate'), sourceHandle: 'yes', label: 'all done', labelStyle: { fill: '#22c55e', fontSize: 10 } });

  // ── Complete Stage ────────────────────────────────────────────
  nodes.push(n('summary', 'stage', 'Generate Summary', { stage: 'complete' }));
  nodes.push(n('summary-approve', 'decision', 'Approved?'));
  nodes.push(n('done', 'stage', 'Done', { stage: 'complete' }));

  // Validation gate loop — creates fix bead, swings LEFT back to Code Writer
  edges.push({ ...eDashed('e-validate-fix', 'bead-validate', 'bead-code-writer', 'fix bead', '#ef4444'), sourceHandle: 'left-out', targetHandle: 'left-in' });
  edges.push(e('e-validate-summary', 'bead-validate', 'summary'));
  edges.push(e('e-summary-approve', 'summary', 'summary-approve'));
  edges.push({ ...e('e-approve-done', 'summary-approve', 'done'), sourceHandle: 'yes', label: 'approved', labelStyle: { fill: '#22c55e', fontSize: 10 } });
  // Summary regenerate loop — swings RIGHT
  edges.push({ ...eDashed('e-summary-loop', 'summary-approve', 'summary', 'regenerate', '#14b8a6'), sourceHandle: 'right-out', targetHandle: 'right-in' });

  return { nodes, edges };
}

// ── Layout with dagre (top-to-bottom cascade) ──────────────────────

function layoutGraph(nodes: Node[], edges: Edge[]) {
  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: 'TB', nodesep: 80, ranksep: 90 });

  nodes.forEach(node => {
    const isStage = node.type === 'stage';
    const isDecision = node.type === 'decision';
    const isMerge = node.type === 'merge';
    g.setNode(node.id, {
      width: isMerge ? 24 : isDecision ? 140 : isStage ? 180 : 160,
      height: isMerge ? 24 : isDecision ? 70 : isStage ? 56 : 44,
    });
  });

  edges.forEach(edge => {
    g.setEdge(edge.source, edge.target);
  });

  dagre.layout(g);

  return nodes.map(node => {
    const pos = g.node(node.id);
    const dim = { width: pos.width ?? 170, height: pos.height ?? 56 };
    return {
      ...node,
      position: {
        x: pos.x - dim.width / 2,
        y: pos.y - dim.height / 2,
      },
    };
  });
}

// ── Component ──────────────────────────────────────────────────────

export function WorkflowGraph({ target, useRefinementLoop, currentNodeId }: Readonly<Props>) {
  const [selectedNode, setSelectedNode] = useState<string | null>(null);
  const rfRef = useRef<ReactFlowInstance | null>(null);

  const { nodes: rawNodes, edges } = useMemo(
    () => buildGraph(!!useRefinementLoop),
    [useRefinementLoop],
  );

  // Mark selected and current nodes in data so CSS classes apply.
  const nodesWithState = useMemo(() => {
    return rawNodes.map(node => ({
      ...node,
      data: {
        ...node.data,
        selected: node.id === selectedNode,
        isCurrent: node.id === currentNodeId,
      },
    }));
  }, [rawNodes, selectedNode, currentNodeId]);

  const nodes = useMemo(() => layoutGraph(nodesWithState, edges), [nodesWithState, edges]);

  const handleNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    // Allow clicking any node that has a description or prompts
    if (NODE_DESCRIPTION_MAP[node.id] || NODE_PROMPT_MAP[node.id]) {
      setSelectedNode(prev => prev === node.id ? null : node.id);
    }
  }, []);

  const promptNames = selectedNode ? (NODE_PROMPT_MAP[selectedNode] ?? []) : [];
  const contextArtifacts = selectedNode ? (NODE_CONTEXT_MAP[selectedNode] ?? []) : [];
  const description = selectedNode ? (NODE_DESCRIPTION_MAP[selectedNode] ?? null) : null;
  const selectedLabel = selectedNode
    ? (rawNodes.find(nd => nd.id === selectedNode)?.data as Record<string, unknown>)?.label as string ?? selectedNode
    : '';

  return (
    <div className="workflow-graph-container">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={workflowNodeTypes}
        onNodeClick={handleNodeClick}
        onPaneClick={() => setSelectedNode(null)}
        onInit={(instance) => {
          rfRef.current = instance;
          // Center on the current step (or Begin for connection mode).
          const centerId = currentNodeId || 'begin';
          const centerNode = nodes.find(nd => nd.id === centerId);
          if (centerNode) {
            const x = centerNode.position.x + 90;
            const y = centerNode.position.y + 28;
            instance.setCenter(x, y, { zoom: 0.85, duration: 0 });
          }
        }}
        minZoom={0.2}
        maxZoom={1.5}
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <Controls position="bottom-left" />
      </ReactFlow>

      {selectedNode && (
        <WorkflowPromptEditor
          target={target}
          promptNames={promptNames}
          contextArtifacts={contextArtifacts}
          nodeDescription={description}
          nodeLabel={selectedLabel}
          onClose={() => setSelectedNode(null)}
        />
      )}
    </div>
  );
}
