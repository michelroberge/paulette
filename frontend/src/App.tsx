import { useState, useEffect, useCallback, useMemo } from 'react';
import { Routes, Route, useNavigate, useParams } from 'react-router-dom';
import { ProjectList } from './components/project/ProjectList';
import { ImportProgressView } from './components/import/ImportProgressView';
import { ProjectHeader } from './components/layout/ProjectHeader';
import { StagesSidebar } from './components/layout/StagesSidebar';
import { StageView } from './components/layout/StageView';
import { ChatPanel } from './components/chat/ChatPanel';
import { ArtifactPreview } from './components/artifact/ArtifactPreview';
import { UxPanel } from './components/ux/UxPanel';
import { BuildPanel } from './components/build/BuildPanel';
import { BuildWizardView } from './components/build/BuildWizardView';
import { BuildingAnimation } from './components/ux/BuildingAnimation';
import { SkillAnalysisPanel } from './components/build/SkillAnalysisPanel';
import { ApproveButton } from './components/pipeline/ApproveButton';
import { CompletionView } from './components/pipeline/CompletionView';
import { VersionHistoryModal } from './components/git/VersionHistoryModal';
import { ProfileModal } from './components/git/ProfileModal';
import { VersionSelectorDropdown } from './components/layout/VersionSelectorDropdown';
import { VersionHistoryView } from './components/layout/VersionHistoryView';
import { ConfigurePage } from './components/configure/ConfigurePage';
import { ProjectStageSettings } from './components/configure/ProjectStageSettings';
import { RunLogView } from './components/RunLogView';
import { WorkflowPage } from './components/pipeline/WorkflowPage';
import { ConnectionWorkflowPage } from './components/pipeline/ConnectionWorkflowPage';
import { getPipeline, resetStage, watchPipeline } from './api/pipeline';
import { getArtifact } from './api/artifacts';
import { getMock } from './api/mock';
import { getBeadGraph } from './api/beads';
import { startEnhancement } from './api/enhance';
import { getProject, patchProject } from './api/projects';
import { getActiveRuns, sendBtw } from './api/activity';
import type { ActiveRun } from './api/activity';
import { getProjectOverrides, getGlobalDefaults } from './api/stageConfig';
import { getConnection } from './api/connections';
import { useChat } from './hooks/useChat';
import { useRefinementLoop } from './hooks/useRefinementLoop';
import { useAgentStream } from './hooks/useAgentStream';
import { RefinementPanel, SECTION_LABELS } from './components/chat/RefinementPanel';
import { AgentStreamingView } from './components/layout/AgentStreamingView';
import type { Project, PipelineState, StageName, VersionBump, Message } from './types';
import './App.css';

const KICKOFF_MESSAGES: Partial<Record<StageName, string>> = {
  vision: "Let's start building your product vision. What's the core idea — what problem are you solving and for whom?",
  ux: "I've reviewed the approved vision. Let me propose the initial user flows and screen descriptions for this product.",
  architecture: "I've reviewed the vision and UX design. Let me propose the technical architecture — stack, components, APIs, and data models.",
  build: "I've reviewed all approved artifacts. Let me create a concrete build plan with milestones and tasks.",
};

const PREV_STAGE: Partial<Record<StageName, StageName>> = {
  ux: 'vision',
  architecture: 'ux',
  build: 'architecture',
};

interface StageTab {
  id: string;
  label: string;
  dimmed?: boolean;
}

function getTabsForStage(stage: StageName | null): StageTab[] {
  if (!stage || stage === 'complete') return [];
  if (stage === 'ux') return [
    { id: 'chat', label: 'Chat' },
    { id: 'artifact', label: 'UX Design' },
    { id: 'mock', label: 'Mock Preview' },
  ];
  if (stage === 'build') {
    return [
      { id: 'chat', label: 'Chat' },
      { id: 'artifact', label: 'Build Plan' },
      { id: 'skills', label: 'Skills' },
      { id: 'generate', label: 'Generate Beads' },
      { id: 'execute', label: 'Implement' },
    ];
  }
  if (stage === 'vision') return [
    { id: 'chat', label: 'Chat' },
    { id: 'artifact', label: 'Artifact' },
  ];
  return [
    { id: 'chat', label: 'Chat' },
    { id: 'artifact', label: 'Artifact' },
  ];
}

// ── Project List Page ──────────────────────────────────────────────────────────

function ProjectListPage() {
  const navigate = useNavigate();
  return (
    <ProjectList
      onSelect={(p) => navigate(`/projects/${p.id}`)}
      onConfigure={() => navigate('/configure')}
    />
  );
}

// ── Project Detail Page ────────────────────────────────────────────────────────

function ProjectDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const [project, setProject] = useState<Project | null>(null);
  const [loadError, setLoadError] = useState(false);
  const [pipelineError, setPipelineError] = useState<string | null>(null);
  const [pipeline, setPipeline] = useState<PipelineState | null>(null);
  const [selectedStage, setSelectedStage] = useState<StageName | null>(null);
  const [chatReloadTrigger, setChatReloadTrigger] = useState(0);
  const [activeTab, setActiveTab] = useState<string>('chat');
  const [stageTokens, setStageTokens] = useState<Partial<Record<StageName, number>>>({});
  const [showVersionHistory, setShowVersionHistory] = useState(false);
  const [versionSelectorOpen, setVersionSelectorOpen] = useState(false);
  const [viewingVersion, setViewingVersion] = useState<string | null>(null);
  const [showProfile, setShowProfile] = useState(false);
  const [showStageSettings, setShowStageSettings] = useState(false);
  const [stagesWithOverrides, setStagesWithOverrides] = useState<Set<StageName>>(new Set());
  const [showImportProgress, setShowImportProgress] = useState(false);
  const [mockGenerated, setMockGenerated] = useState(false);
  const [buildComplete, setBuildComplete] = useState(false);
  const [hasBeads, setHasBeads] = useState(false);
  const [hasBuildArtifact, setHasBuildArtifact] = useState(false);
  const [activeRuns, setActiveRuns] = useState<ActiveRun[]>([]);
  const [btwInput, setBtwInput] = useState('');
  const [btwSending, setBtwSending] = useState(false);
  const [showRunLog, setShowRunLog] = useState(false);
  const [guidedMode, setGuidedMode] = useState(false);

  // Load project from URL param on mount / ID change
  useEffect(() => {
    if (!id) { navigate('/'); return; }
    setLoadError(false);
    setProject(null);
    setPipeline(null);
    getProject(id).then(setProject).catch(() => {
      setLoadError(true);
    });
  }, [id, navigate]);

  const addTokens = useCallback((stage: StageName, n: number) => {
    if (n <= 0) return;
    setStageTokens(prev => ({ ...prev, [stage]: (prev[stage] ?? 0) + n }));
  }, []);

  const grandTotal = useMemo(
    () => Object.values(stageTokens).reduce((s, n) => s + (n ?? 0), 0),
    [stageTokens],
  );

  const { messages, streaming, streamingContent, artifactUpdated, historyLoaded, nextTurn, connectionError, ragSources, loadHistory, send, seed, resume, stop, addLocalMessage, seedMessages } =
    useChat(project?.id ?? null, selectedStage, chatReloadTrigger, selectedStage ? (n) => addTokens(selectedStage, n) : undefined);

  const refinementLoop = useRefinementLoop(
    project?.id ?? null,
    (n) => addTokens('vision', n),
  );

  const handleContinueFromRefinement = useCallback((followUpMessage: string) => {
    const chatMessages: Message[] = [];
    for (const entry of refinementLoop.log) {
      if (entry.phase === 'await_answer' && entry.question && !entry.userAnswer) {
        chatMessages.push({
          role: 'assistant',
          content: `**${SECTION_LABELS[entry.question.section] ?? entry.question.section}**: ${entry.question.text}`,
          timestamp: new Date(entry.timestamp).toISOString(),
        });
      }
      if (entry.userAnswer) {
        chatMessages.push({
          role: 'user',
          content: entry.userAnswer,
          timestamp: new Date(entry.timestamp).toISOString(),
        });
      }
    }
    chatMessages.push({
      role: 'assistant',
      content: 'Vision document generated. You can continue refining it here.',
      timestamp: new Date().toISOString(),
    });
    seedMessages(chatMessages);
    setGuidedMode(false);
    setTimeout(() => send(followUpMessage), 0);
  }, [refinementLoop.log, seedMessages, send]);

  const { active: agentActive, streamingText: agentStreamingText, operation: agentOperation, stage: agentStage } =
    useAgentStream(project?.id ?? null, activeRuns);

  const loadPipeline = useCallback(async () => {
    if (!project) return;
    const state = await getPipeline(project.id);
    setPipeline(state);
    return state;
  }, [project]);

  /** Load per-project stage overrides and update the pip accent set. */
  const loadStageOverrides = useCallback(async () => {
    if (!project) return;
    try {
      const config = await getProjectOverrides(project.id);
      const overridden = new Set<StageName>(
        (Object.entries(config.overrides) as [StageName, unknown][])
          .filter(([, v]) => v != null)
          .map(([k]) => k),
      );
      setStagesWithOverrides(overridden);
    } catch {
      // Non-critical — silently swallow; no override accents shown
    }
  }, [project]);

  // Restore persisted stage tokens when selecting a project
  useEffect(() => {
    if (project) {
      setStageTokens(project.stageTokens ?? {});
      // Show import progress for newly imported projects
      if (project.imported && project.currentStage === 'vision') {
        setShowImportProgress(true);
      } else {
        setShowImportProgress(false);
      }
    }
  }, [project?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (project) {
      setPipelineError(null);
      loadPipeline().then(state => {
        if (state) setSelectedStage(state.currentStage);
      }).catch(err => {
        console.error('Failed to load pipeline:', err);
        setPipelineError(err?.message || 'Failed to load pipeline');
      });
      loadStageOverrides();
    }
  }, [project, loadPipeline, loadStageOverrides]);

  // Detect Ollama connection for vision stage → default guided mode
  useEffect(() => {
    if (!project || selectedStage !== 'vision') return;
    let cancelled = false;
    (async () => {
      try {
        const [overrides, defaults] = await Promise.all([
          getProjectOverrides(project.id),
          getGlobalDefaults(),
        ]);
        const assignment = overrides.overrides.vision ?? defaults.stageDefaults?.vision;
        if (!assignment?.connectionId) return;
        const conn = await getConnection(assignment.connectionId);
        if (!cancelled) setGuidedMode(conn.providerType === 'ollama');
      } catch {
        // Non-critical; leave default (false)
      }
    })();
    return () => { cancelled = true; };
  }, [project?.id, selectedStage]); // eslint-disable-line react-hooks/exhaustive-deps

  // Subscribe to live pipeline updates via SSE
  useEffect(() => {
    if (!project) return;
    const controller = new AbortController();
    watchPipeline(project.id, setPipeline, controller.signal).catch(() => {});
    return () => controller.abort();
  }, [project?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  // Poll active runs so the sidebar knows which run IDs to use for /btw
  useEffect(() => {
    if (!project) { setActiveRuns([]); return; }
    let cancelled = false;
    const poll = async () => {
      try {
        const runs = await getActiveRuns(project.id);
        if (!cancelled) setActiveRuns(runs);
      } catch { /* ignore */ }
    };
    poll();
    const id = setInterval(poll, 3000);
    return () => { cancelled = true; clearInterval(id); };
  }, [project?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    loadHistory();
  }, [loadHistory]);

  // Smart tab default: show most advanced available content on stage navigation
  useEffect(() => {
    if (!project || !selectedStage || selectedStage === 'complete') {
      setActiveTab('chat');
      setMockGenerated(false);
      setBuildComplete(false);
      return;
    }
    setMockGenerated(false);
    setBuildComplete(false);
    setHasBeads(false);
    setHasBuildArtifact(false);

    (async () => {
      try {
        const artifact = await getArtifact(project.id, selectedStage);
        const artifactExists = !!artifact?.content?.trim();
        if (!artifactExists) { setActiveTab('chat'); return; }

        if (selectedStage === 'ux') {
          const mock = await getMock(project.id);
          if (mock.exists) { setActiveTab('mock'); setMockGenerated(true); }
          else setActiveTab('artifact');
        } else if (selectedStage === 'build') {
          setHasBuildArtifact(true);
          const graph = await getBeadGraph(project.id);
          const hasBeadsLocal = !!graph?.beads?.length;
          if (hasBeadsLocal) {
            setHasBeads(true);
            setBuildComplete(true);
          }
          // Restore last-visited wizard step for this project if valid.
          const VALID_BUILD_STEPS = ['chat', 'artifact', 'skills', 'generate', 'execute'] as const;
          const saved = localStorage.getItem(`paulette.build.wizard.${project.id}`);
          if (saved && (VALID_BUILD_STEPS as readonly string[]).includes(saved)) {
            setActiveTab(saved);
          } else if (hasBeadsLocal) {
            setActiveTab('execute');
          } else {
            setActiveTab('artifact');
          }
        } else {
          setActiveTab('artifact');
        }
      } catch {
        setActiveTab('chat');
      }
    })();
  }, [selectedStage]); // eslint-disable-line react-hooks/exhaustive-deps

  // Persist build wizard step per-project so returning to Build resumes where the user left off.
  useEffect(() => {
    if (!project || selectedStage !== 'build') return;
    localStorage.setItem(`paulette.build.wizard.${project.id}`, activeTab);
  }, [activeTab, selectedStage, project?.id]);

  // When a build artifact is produced mid-session, unblock the wizard Next button
  useEffect(() => {
    if (selectedStage === 'build' && artifactUpdated > 0) {
      setHasBuildArtifact(true);
    }
  }, [artifactUpdated, selectedStage]);

  // Auto-switch to artifact tab when artifact is generated (for non-vision stages)
  useEffect(() => {
    if (artifactUpdated > 0 && selectedStage && selectedStage !== 'vision' && selectedStage !== 'complete') {
      if (activeTab === 'chat') {
        setActiveTab('artifact');
      }
    }
  }, [artifactUpdated]); // eslint-disable-line react-hooks/exhaustive-deps

  // Autopilot: advance build wizard past Build Plan / Skills when the orchestrator
  // starts generating or executing beads. Skills analysis remains user-initiated,
  // so we skip that step automatically in auto mode.
  useEffect(() => {
    if (selectedStage !== 'build' || !agentActive) return;
    if (agentOperation === 'beads-generate' && (activeTab === 'artifact' || activeTab === 'skills')) {
      setActiveTab('generate');
    } else if (agentOperation === 'beads-execute' && (activeTab === 'artifact' || activeTab === 'skills' || activeTab === 'generate')) {
      setActiveTab('execute');
    }
  }, [agentOperation, agentActive, selectedStage]); // eslint-disable-line react-hooks/exhaustive-deps

  // Poll for summaryReady when at Complete stage
  useEffect(() => {
    if (!project || project.currentStage !== 'complete' || project.summaryReady) return;
    const id = setInterval(async () => {
      const updated = await getProject(project.id);
      if (updated.summaryReady) {
        setProject(updated);
        if (updated.summaryTokens) addTokens('complete', updated.summaryTokens);
        clearInterval(id);
      }
    }, 3000);
    return () => clearInterval(id);
  }, [project?.id, project?.currentStage, project?.summaryReady]); // eslint-disable-line react-hooks/exhaustive-deps

  // Auto-kickoff: when entering a stage, send the opening message or resume if interrupted.
  // Skip for imported projects — artifacts are pre-populated, user reviews manually.
  // Skip for autonomous mode — the backend orchestrator drives chat.
  useEffect(() => {
    if (!historyLoaded || streaming) return;
    if (project?.imported || project?.autonomous) return;
    const currentStageInfo = pipeline?.stages.find(s => s.name === selectedStage);
    if (currentStageInfo?.status !== 'active') return;
    if (nextTurn !== 'agent') return;

    if (messages.length === 0) {
      // Fresh vision stage on a new project — greet the user and wait for their input
      // rather than immediately calling the AI with a generic kickoff message.
      if (selectedStage === 'vision' && !project?.enhancementVision) {
        addLocalMessage("Provide a short description of your idea, we'll work it out together.");
        return;
      }
      // Fresh stage — send the opening kickoff message.
      const doKickoff = async () => {
        // Vision (non-enhancement) opens with a local assistant greeting and waits for the
        // user's first input, rather than auto-triggering the AI with a synthetic user turn.
        if (selectedStage === 'vision' && !project?.enhancementVision) {
          seed("Let's work together to make your vision come to life.");
          return;
        }
        let kickoff = selectedStage ? KICKOFF_MESSAGES[selectedStage] : undefined;
        if (project?.enhancementVision) {
          if (selectedStage === 'vision') {
            kickoff = `This is an enhancement iteration. Here's what I want to improve: ${project.enhancementVision}`;
          } else if (kickoff) {
            const prevStage = selectedStage ? PREV_STAGE[selectedStage] : undefined;
            let prevArtifact = '';
            if (prevStage && project?.id) {
              try {
                const artifact = await getArtifact(project.id, prevStage);
                if (artifact?.content?.trim()) prevArtifact = artifact.content.trim();
              } catch { /* ignore */ }
            }
            kickoff = prevArtifact
              ? `${kickoff}\n\nThis is an enhancement iteration — focus on: ${project.enhancementVision}\n\nApproved ${prevStage} artifact to build upon:\n\n${prevArtifact}`
              : `${kickoff} This is an enhancement iteration — focus on: ${project.enhancementVision}`;
          }
        }
        if (kickoff) send(kickoff);
      };
      doKickoff();
    } else {
      // Unanswered user message (e.g. server restarted mid-generation) — resume.
      resume();
    }
  }, [historyLoaded]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleReset = async (stage: StageName) => {
    const state = await resetStage(project!.id, stage);
    setPipeline(state);
    setSelectedStage(stage);
    setChatReloadTrigger(t => t + 1);
  };

  const handleApproved = async () => {
    const state = await loadPipeline();
    if (state) setSelectedStage(state.currentStage);
  };

  const handleEnhance = async (vision: string, bump: VersionBump) => {
    if (!project) return;
    const result = await startEnhancement(project.id, vision, bump);
    setProject(result.project);
    setPipeline(result.pipeline);
    setSelectedStage('vision');
    setChatReloadTrigger(t => t + 1);
  };

  const handleGitReset = (resetProject: Project, resetPipeline: PipelineState) => {
    setProject(resetProject);
    setPipeline(resetPipeline);
    setSelectedStage(resetPipeline.currentStage);
    setChatReloadTrigger(t => t + 1);
    setShowVersionHistory(false);
  };

  // ── Autonomous mode ──

  const handleBtwSend = async (e: React.SyntheticEvent) => {
    e.preventDefault();
    if (!project) return;
    const agentRun = activeRuns.find(r => r.stage === agentStage && r.operation !== 'chat' && r.operation !== 'mock');
    if (!agentRun || !btwInput.trim()) return;
    setBtwSending(true);
    try {
      await sendBtw(project.id, agentRun.id, btwInput.trim());
      setBtwInput('');
    } catch { /* ignore */ }
    finally { setBtwSending(false); }
  };

  const handleToggleAutonomous = async () => {
    if (!project) return;
    const updated = await patchProject(project.id, { autonomous: !project.autonomous });
    setProject(updated);
  };

  const handleChangeAIMode = async (mode: 'files' | 'rag') => {
    if (!project) return;
    const updated = await patchProject(project.id, { aiMode: mode });
    setProject(updated);
  };

  // ── Loading / error states ──

  if (loadError) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', minHeight: '100vh', gap: '1rem' }}>
        <p style={{ color: '#ef4444' }}>Project not found.</p>
        <button onClick={() => navigate('/')} style={{ color: '#60a5fa', background: 'none', border: 'none', cursor: 'pointer' }}>
          ← Back to projects
        </button>
      </div>
    );
  }

  if (!project) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: '100vh' }}>
        <span style={{ color: '#64748b' }}>Loading…</span>
      </div>
    );
  }

  if (pipelineError) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', minHeight: '100vh', gap: '1rem' }}>
        <p style={{ color: '#ef4444' }}>Failed to load pipeline: {pipelineError}</p>
        <button onClick={() => { setPipelineError(null); loadPipeline().then(state => { if (state) setSelectedStage(state.currentStage); }).catch(err => setPipelineError(err?.message || 'Failed to load pipeline')); }} style={{ color: '#60a5fa', background: 'none', border: 'none', cursor: 'pointer' }}>
          Retry
        </button>
        <button onClick={() => navigate('/')} style={{ color: '#60a5fa', background: 'none', border: 'none', cursor: 'pointer' }}>
          ← Back to projects
        </button>
      </div>
    );
  }

  const currentStageInfo = pipeline?.stages.find(s => s.name === selectedStage);
  const isActiveStage = currentStageInfo?.status === 'active';
  const tabs = getTabsForStage(selectedStage);

  return (
    <div className="app-shell">
      <div style={{ position: 'relative' }}>
        <ProjectHeader
          project={project}
          onBack={() => navigate('/')}
          totalTokens={grandTotal}
          onShowHistory={() => setShowVersionHistory(true)}
          onShowProfile={() => setShowProfile(true)}
          onShowStageSettings={() => setShowStageSettings(true)}
          onShowRunLog={() => setShowRunLog(v => !v)}
          autonomous={!!project.autonomous}
          onToggleAutonomous={handleToggleAutonomous}
          onChangeAIMode={handleChangeAIMode}
          onShowWorkflow={() => navigate(`/projects/${project.id}/workflow`)}
          onVersionClick={() => setVersionSelectorOpen(v => !v)}
          viewingVersion={viewingVersion}
        />
        {versionSelectorOpen && (
          <VersionSelectorDropdown
            projectId={project.id}
            currentVersion={project.version}
            onSelect={v => { setViewingVersion(v); setVersionSelectorOpen(false); }}
            onClose={() => setVersionSelectorOpen(false)}
          />
        )}
      </div>

      <div className="app-body">
        {!viewingVersion && (
          <StagesSidebar
            pipeline={pipeline}
            selectedStage={selectedStage}
            onSelectStage={setSelectedStage}
            onReset={handleReset}
            stageTokens={stageTokens}
            stagesWithOverrides={stagesWithOverrides}
          />
        )}

        <main className="main-content">
          {showRunLog ? (
            <RunLogView projectId={project.id} onClose={() => setShowRunLog(false)} />
          ) : viewingVersion ? (
            <VersionHistoryView
              project={project}
              version={viewingVersion}
              onClose={() => setViewingVersion(null)}
            />
          ) : showImportProgress ? (
            <ImportProgressView
              project={project}
              onComplete={() => {
                setShowImportProgress(false);
                loadPipeline();
                setChatReloadTrigger(t => t + 1);
              }}
            />
          ) : pipeline?.currentStage === 'complete' && selectedStage === 'complete' ? (
            <CompletionView
              project={project}
              activity={pipeline?.stages.find(s => s.name === 'complete')?.activity}
              onNewProject={() => navigate('/')}
              onViewStage={stage => setSelectedStage(stage)}
              onEnhance={handleEnhance}
              onSummaryReady={async () => {
                const updated = await getProject(project.id);
                setProject(updated);
              }}
            />
          ) : (
            <>
              {project.imported && isActiveStage && (
                <div style={{
                  padding: '0.5rem 1rem',
                  background: '#1e293b',
                  borderBottom: '1px solid #334155',
                  color: '#94a3b8',
                  fontSize: '0.8rem',
                }}>
                  This artifact was auto-generated from your codebase. Review and refine via chat, then approve.
                </div>
              )}
              {agentActive && agentOperation !== 'chat' && agentOperation !== 'refinement' && agentStage === selectedStage && selectedStage !== 'ux' && selectedStage !== 'build' ? (
                <AgentStreamingView
                  streamingText={agentStreamingText}
                  operation={agentOperation}
                  btwInput={btwInput}
                  btwSending={btwSending}
                  onBtwChange={setBtwInput}
                  onBtwSubmit={handleBtwSend}
                  btwPendingCount={pipeline?.stages.find(s => s.name === agentStage)?.activity?.pendingBtw?.length ?? 0}
                />
              ) : selectedStage === 'build' ? (
              <BuildWizardView
                activeStep={activeTab}
                onStepChange={setActiveTab}
                canAdvance={{
                  chat:     hasBuildArtifact,
                  artifact: true,
                  skills:   true,
                  generate: hasBeads,
                  execute:  true,
                }}
              >
                {activeTab === 'chat' && (
                  streaming ? (
                    <div className="mock-split-layout">
                      <div className="mock-main-column">
                        <BuildingAnimation />
                      </div>
                      <div className="mock-activity-panel">
                        <div className="mock-activity-header">
                          <span className="mock-stream-dot" />
                          <span>Generating Build Plan…</span>
                        </div>
                        <div className="mock-activity-content">
                          {streamingContent || 'Starting…'}
                        </div>
                      </div>
                    </div>
                  ) : (
                    <ChatPanel
                      messages={messages}
                      streaming={streaming}
                      streamingContent={streamingContent}
                      onSend={send}
                      onStop={stop}
                      connectionError={connectionError}
                      onOpenProjectSettings={() => setShowStageSettings(true)}
                      onRetry={resume}
                      ragSources={ragSources}
                    />
                  )
                )}

                {activeTab === 'skills' && (
                  <SkillAnalysisPanel projectId={project.id} />
                )}

                {activeTab !== 'chat' && activeTab !== 'skills' && (
                  <BuildPanel
                    projectId={project.id}
                    refreshTrigger={artifactUpdated}
                    mode={activeTab === 'execute' || activeTab === 'generate' ? 'execute' : 'artifact'}
                    onRequestExecuteTab={() => setActiveTab('generate')}
                    hidden={false}
                    onBeadTokens={(n) => addTokens('build', n)}
                    onExecutionComplete={() => setBuildComplete(true)}
                    onBuildDone={() => setBuildComplete(true)}
                    onHasBeads={setHasBeads}
                    agentActive={agentActive}
                    agentOperation={agentOperation}
                    agentStreamingText={agentStreamingText}
                    hideGenerateButton={activeTab === 'artifact'}
                  />
                )}
              </BuildWizardView>
              ) : (
              <StageView tabs={tabs} activeTab={activeTab} onTabChange={setActiveTab}>
                {activeTab === 'chat' && (
                  selectedStage === 'vision' ? (
                    <div className="vision-chat-container">
                      <div className="guided-toggle-bar">
                        <label className={`guided-toggle${guidedMode ? ' active' : ''}`}>
                          <span className="toggle-label">Guided</span>
                          <input
                            type="checkbox"
                            checked={guidedMode}
                            onChange={() => setGuidedMode(g => !g)}
                          />
                          <span className="guided-toggle-track">
                            <span className="guided-toggle-thumb" />
                          </span>
                        </label>
                      </div>
                      {guidedMode ? (
                        <RefinementPanel
                          loop={refinementLoop}
                          onSend={(msg) => refinementLoop.start(msg)}
                          onContinue={handleContinueFromRefinement}
                        />
                      ) : (
                        <ChatPanel
                          messages={messages}
                          streaming={streaming}
                          streamingContent={streamingContent}
                          onSend={send}
                          onStop={stop}
                          connectionError={connectionError}
                          onOpenProjectSettings={() => setShowStageSettings(true)}
                          onRetry={resume}
                          ragSources={ragSources}
                        />
                      )}
                    </div>
                  ) : streaming && selectedStage && ['ux', 'architecture'].includes(selectedStage) ? (
                    <div className="mock-split-layout">
                      <div className="mock-main-column">
                        <BuildingAnimation />
                      </div>
                      <div className="mock-activity-panel">
                        <div className="mock-activity-header">
                          <span className="mock-stream-dot" />
                          <span>Generating {selectedStage === 'ux' ? 'UX Design' : 'Architecture'}…</span>
                        </div>
                        <div className="mock-activity-content">
                          {streamingContent || 'Starting…'}
                        </div>
                      </div>
                    </div>
                  ) : (
                    <ChatPanel
                      messages={messages}
                      streaming={streaming}
                      streamingContent={streamingContent}
                      onSend={send}
                      onStop={stop}
                      connectionError={connectionError}
                      onOpenProjectSettings={() => setShowStageSettings(true)}
                      onRetry={resume}
                      ragSources={ragSources}
                      onGenerateArtifact={selectedStage && ['ux', 'architecture'].includes(selectedStage) ? () => send('Please generate the complete artifact document based on our discussion so far. Wrap it in <artifact> tags.') : undefined}
                      generateArtifactLabel={selectedStage === 'ux' ? 'UX Design' : selectedStage === 'architecture' ? 'Architecture' : undefined}
                    />
                  )
                )}

                {activeTab === 'artifact' && selectedStage && !['ux', 'build', 'complete'].includes(selectedStage) && (
                  <ArtifactPreview
                    projectId={project.id}
                    stage={selectedStage}
                    refreshTrigger={artifactUpdated + refinementLoop.artifactUpdated}
                    onArtifactUpdated={() => setChatReloadTrigger(t => t + 1)}
                  />
                )}

                {selectedStage === 'ux' && (
                  <UxPanel
                    projectId={project.id}
                    refreshTrigger={artifactUpdated}
                    mode={activeTab === 'mock' ? 'mock' : 'artifact'}
                    onRequestMockTab={() => setActiveTab('mock')}
                    hidden={activeTab === 'chat'}
                    onMockTokens={(n) => addTokens('ux', n)}
                    onMockComplete={() => setMockGenerated(true)}
                    onMockLoaded={() => setMockGenerated(true)}
                  />
                )}
              </StageView>
              )}

              {isActiveStage && (
                <div className="approve-bar">
                  {project.autonomous ? (
                    <span className="auto-approve-indicator">Auto-approve active — will advance automatically</span>
                  ) : (
                    <ApproveButton
                      projectId={project.id}
                      disabled={streaming || (selectedStage === 'ux' && !mockGenerated)}
                      warning={
                        selectedStage === 'build'
                          ? !hasBeads
                            ? "Are you sure? You didn't build anything yet"
                            : !buildComplete
                              ? "Are you sure? There's still work to do!"
                              : undefined
                          : undefined
                      }
                      onApproved={handleApproved}
                    />
                  )}
                </div>
              )}
            </>
          )}
        </main>
      </div>

      {showVersionHistory && (
        <VersionHistoryModal
          projectId={project.id}
          onClose={() => setShowVersionHistory(false)}
          onReset={handleGitReset}
        />
      )}
      {showProfile && (
        <ProfileModal
          projectId={project.id}
          initialName={project.author}
          onClose={() => setShowProfile(false)}
        />
      )}
      {showStageSettings && (
        <ProjectStageSettings
          projectId={project.id}
          projectName={project.name}
          onClose={() => setShowStageSettings(false)}
          onOverridesChange={loadStageOverrides}
        />
      )}
    </div>
  );
}

// ── App Router ─────────────────────────────────────────────────────────────────

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<ProjectListPage />} />
      <Route path="/projects/:id" element={<ProjectDetailPage />} />
      <Route path="/projects/:id/workflow" element={<WorkflowPage />} />
      <Route path="/configure/connections/:connId/workflow" element={<ConnectionWorkflowPage />} />
      <Route path="/configure" element={<ConfigurePage />} />
    </Routes>
  );
}
