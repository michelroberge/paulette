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
import { SkillAnalysisPanel } from './components/build/SkillAnalysisPanel';
import { ApproveButton } from './components/pipeline/ApproveButton';
import { CompletionView } from './components/pipeline/CompletionView';
import { VersionHistoryModal } from './components/git/VersionHistoryModal';
import { ProfileModal } from './components/git/ProfileModal';
import { ConfigurePage } from './components/configure/ConfigurePage';
import { ProjectStageSettings } from './components/configure/ProjectStageSettings';
import { getPipeline, resetStage, watchPipeline } from './api/pipeline';
import { getArtifact } from './api/artifacts';
import { getMock } from './api/mock';
import { getBeadGraph } from './api/beads';
import { startEnhancement } from './api/enhance';
import { getProject, patchProject } from './api/projects';
import { getActiveRuns, sendBtw } from './api/activity';
import type { ActiveRun } from './api/activity';
import { getProjectOverrides } from './api/stageConfig';
import { useChat } from './hooks/useChat';
import { useAgentStream } from './hooks/useAgentStream';
import { AgentStreamingView } from './components/layout/AgentStreamingView';
import type { Project, PipelineState, StageName, VersionBump } from './types';
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
}

function getTabsForStage(stage: StageName | null): StageTab[] {
  if (!stage || stage === 'complete') return [];
  if (stage === 'ux') return [
    { id: 'chat', label: 'Chat' },
    { id: 'artifact', label: 'UX Design' },
    { id: 'mock', label: 'Mock Preview' },
  ];
  if (stage === 'build') {
    const tabs: StageTab[] = [
      { id: 'chat', label: 'Chat' },
      { id: 'artifact', label: 'Build Plan' },
      { id: 'skills', label: 'Skills' },
    ];
    tabs.push({ id: 'execute', label: 'Execute' });
    return tabs;
  }
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
  const [pipeline, setPipeline] = useState<PipelineState | null>(null);
  const [selectedStage, setSelectedStage] = useState<StageName | null>(null);
  const [chatReloadTrigger, setChatReloadTrigger] = useState(0);
  const [activeTab, setActiveTab] = useState<string>('chat');
  const [stageTokens, setStageTokens] = useState<Partial<Record<StageName, number>>>({});
  const [showVersionHistory, setShowVersionHistory] = useState(false);
  const [showProfile, setShowProfile] = useState(false);
  const [showStageSettings, setShowStageSettings] = useState(false);
  const [stagesWithOverrides, setStagesWithOverrides] = useState<Set<StageName>>(new Set());
  const [showImportProgress, setShowImportProgress] = useState(false);
  const [mockGenerated, setMockGenerated] = useState(false);
  const [buildComplete, setBuildComplete] = useState(false);
  const [hasBeads, setHasBeads] = useState(false);
  const [activeRuns, setActiveRuns] = useState<ActiveRun[]>([]);
  const [btwInput, setBtwInput] = useState('');
  const [btwSending, setBtwSending] = useState(false);

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

  const { messages, streaming, streamingContent, artifactUpdated, historyLoaded, nextTurn, connectionError, loadHistory, send, resume, stop } =
    useChat(project?.id ?? null, selectedStage, chatReloadTrigger, selectedStage ? (n) => addTokens(selectedStage, n) : undefined);

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
      loadPipeline().then(state => {
        if (state) setSelectedStage(state.currentStage);
      });
      loadStageOverrides();
    }
  }, [project, loadPipeline, loadStageOverrides]);

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

    (async () => {
      try {
        const artifact = await getArtifact(project.id, selectedStage);
        if (!artifact?.content?.trim()) { setActiveTab('chat'); return; }

        if (selectedStage === 'ux') {
          const mock = await getMock(project.id);
          if (mock.exists) { setActiveTab('mock'); setMockGenerated(true); }
          else setActiveTab('artifact');
        } else if (selectedStage === 'build') {
          const graph = await getBeadGraph(project.id);
          if (graph?.beads?.length) {
            setHasBeads(true);
            setActiveTab('execute');
            setBuildComplete(true);
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
      // Fresh stage — send the opening kickoff message.
      const doKickoff = async () => {
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

  const currentStageInfo = pipeline?.stages.find(s => s.name === selectedStage);
  const isActiveStage = currentStageInfo?.status === 'active';
  const tabs = getTabsForStage(selectedStage);

  return (
    <div className="app-shell">
      <ProjectHeader
        project={project}
        onBack={() => navigate('/')}
        totalTokens={grandTotal}
        onShowHistory={() => setShowVersionHistory(true)}
        onShowProfile={() => setShowProfile(true)}
        onShowStageSettings={() => setShowStageSettings(true)}
        autonomous={!!project.autonomous}
        onToggleAutonomous={handleToggleAutonomous}
      />

      <div className="app-body">
        <StagesSidebar
          pipeline={pipeline}
          selectedStage={selectedStage}
          onSelectStage={setSelectedStage}
          onReset={handleReset}
          stageTokens={stageTokens}
          stagesWithOverrides={stagesWithOverrides}
        />

        <main className="main-content">
          {showImportProgress ? (
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
              {agentActive && agentOperation !== 'chat' && agentStage === selectedStage && selectedStage !== 'ux' && selectedStage !== 'build' ? (
                <AgentStreamingView
                  streamingText={agentStreamingText}
                  operation={agentOperation}
                  btwInput={btwInput}
                  btwSending={btwSending}
                  onBtwChange={setBtwInput}
                  onBtwSubmit={handleBtwSend}
                  btwPendingCount={pipeline?.stages.find(s => s.name === agentStage)?.activity?.pendingBtw?.length ?? 0}
                />
              ) : (
              <StageView tabs={tabs} activeTab={activeTab} onTabChange={setActiveTab}>
                {activeTab === 'chat' && (
                  <ChatPanel
                    messages={messages}
                    streaming={streaming}
                    streamingContent={streamingContent}
                    onSend={send}
                    onStop={stop}
                    connectionError={connectionError}
                    onOpenProjectSettings={() => setShowStageSettings(true)}
                  />
                )}

                {activeTab === 'artifact' && selectedStage && !['ux', 'build', 'complete'].includes(selectedStage) && (
                  <ArtifactPreview
                    projectId={project.id}
                    stage={selectedStage}
                    refreshTrigger={artifactUpdated}
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

                {selectedStage === 'build' && activeTab === 'skills' && (
                  <SkillAnalysisPanel projectId={project.id} />
                )}

                {selectedStage === 'build' && activeTab !== 'skills' && (
                  <BuildPanel
                    projectId={project.id}
                    refreshTrigger={artifactUpdated}
                    mode={activeTab === 'execute' ? 'execute' : 'artifact'}
                    onRequestExecuteTab={() => setActiveTab('execute')}
                    hidden={activeTab === 'chat'}
                    onBeadTokens={(n) => addTokens('build', n)}
                    onExecutionComplete={() => setBuildComplete(true)}
                    onBuildDone={() => setBuildComplete(true)}
                    onHasBeads={setHasBeads}
                    agentActive={agentActive}
                    agentOperation={agentOperation}
                    agentStreamingText={agentStreamingText}
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
      <Route path="/configure" element={<ConfigurePage />} />
    </Routes>
  );
}
