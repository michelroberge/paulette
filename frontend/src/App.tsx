import { useState, useEffect, useCallback, useMemo } from 'react';
import { ProjectList } from './components/project/ProjectList';
import { ImportProgressView } from './components/import/ImportProgressView';
import { ProjectHeader } from './components/layout/ProjectHeader';
import { StagesSidebar } from './components/layout/StagesSidebar';
import { StageView } from './components/layout/StageView';
import { ChatPanel } from './components/chat/ChatPanel';
import { ArtifactPreview } from './components/artifact/ArtifactPreview';
import { UxPanel } from './components/ux/UxPanel';
import { BuildPanel } from './components/build/BuildPanel';
import { ApproveButton } from './components/pipeline/ApproveButton';
import { CompletionView } from './components/pipeline/CompletionView';
import { VersionHistoryModal } from './components/git/VersionHistoryModal';
import { getPipeline, approveStage, resetStage } from './api/pipeline';
import { getArtifact } from './api/artifacts';
import { getMock } from './api/mock';
import { getBeadGraph } from './api/beads';
import { startEnhancement } from './api/enhance';
import { getProject, patchProject } from './api/projects';
import { useChat } from './hooks/useChat';
import type { Project, PipelineState, StageName, VersionBump } from './types';
import './App.css';

const KICKOFF_MESSAGES: Partial<Record<StageName, string>> = {
  vision: "Let's start building your product vision. What's the core idea — what problem are you solving and for whom?",
  ux: "I've reviewed the approved vision. Let me propose the initial user flows and screen descriptions for this product.",
  architecture: "I've reviewed the vision and UX design. Let me propose the technical architecture — stack, components, APIs, and data models.",
  build: "I've reviewed all approved artifacts. Let me create a concrete build plan with milestones and tasks.",
};

interface StageTab {
  id: string;
  label: string;
}

function getTabsForStage(stage: StageName | null, imported?: boolean): StageTab[] {
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
    ];
    if (!imported) tabs.push({ id: 'execute', label: 'Execute' });
    return tabs;
  }
  return [
    { id: 'chat', label: 'Chat' },
    { id: 'artifact', label: 'Artifact' },
  ];
}

function App() {
  const [project, setProject] = useState<Project | null>(null);
  const [pipeline, setPipeline] = useState<PipelineState | null>(null);
  const [selectedStage, setSelectedStage] = useState<StageName | null>(null);
  const [chatReloadTrigger, setChatReloadTrigger] = useState(0);
  const [activeTab, setActiveTab] = useState<string>('chat');
  const [stageTokens, setStageTokens] = useState<Partial<Record<StageName, number>>>({});
  const [showVersionHistory, setShowVersionHistory] = useState(false);
  const [showImportProgress, setShowImportProgress] = useState(false);
  const [mockGenerated, setMockGenerated] = useState(false);
  const [buildComplete, setBuildComplete] = useState(false);

  const addTokens = useCallback((stage: StageName, n: number) => {
    if (n <= 0) return;
    setStageTokens(prev => ({ ...prev, [stage]: (prev[stage] ?? 0) + n }));
  }, []);

  const grandTotal = useMemo(
    () => Object.values(stageTokens).reduce((s, n) => s + (n ?? 0), 0),
    [stageTokens],
  );

  const { messages, streaming, streamingContent, artifactUpdated, historyLoaded, loadHistory, send, stop } =
    useChat(project?.id ?? null, selectedStage, chatReloadTrigger, selectedStage ? (n) => addTokens(selectedStage, n) : undefined);

  const loadPipeline = useCallback(async () => {
    if (!project) return;
    const state = await getPipeline(project.id);
    setPipeline(state);
    return state;
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
    }
  }, [project, loadPipeline]);

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

    (async () => {
      try {
        const artifact = await getArtifact(project.id, selectedStage);
        if (!artifact?.content?.trim()) { setActiveTab('chat'); return; }

        if (selectedStage === 'ux') {
          const mock = await getMock(project.id);
          if (mock.exists) { setActiveTab('mock'); setMockGenerated(true); }
          else setActiveTab('artifact');
        } else if (selectedStage === 'build') {
          if (!project.imported) {
            const graph = await getBeadGraph(project.id);
            if (graph?.beads?.length) { setActiveTab('execute'); setBuildComplete(true); }
            else setActiveTab('artifact');
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

  // Auto-kickoff: when entering a stage with no history, send the opening message
  // Skip for imported projects — artifacts are pre-populated, user reviews manually
  // Skip for autonomous mode — the backend orchestrator drives chat
  useEffect(() => {
    if (!historyLoaded || messages.length > 0 || streaming) return;
    if (project?.imported || project?.autonomous) return;
    const currentStageInfo = pipeline?.stages.find(s => s.name === selectedStage);
    if (currentStageInfo?.status !== 'active') return;
    let kickoff = selectedStage ? KICKOFF_MESSAGES[selectedStage] : undefined;
    if (project?.enhancementVision && selectedStage === 'vision') {
      kickoff = `This is an enhancement iteration. Here's what I want to improve: ${project.enhancementVision}`;
    }
    if (kickoff) send(kickoff);
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

  const handleToggleAutonomous = async () => {
    if (!project) return;
    const updated = await patchProject(project.id, { autonomous: !project.autonomous });
    setProject(updated);
  };

  if (!project) {
    return <ProjectList onSelect={setProject} />;
  }

  const currentStageInfo = pipeline?.stages.find(s => s.name === selectedStage);
  const isActiveStage = currentStageInfo?.status === 'active';
  const tabs = getTabsForStage(selectedStage, project?.imported);

  return (
    <div className="app-shell">
      <ProjectHeader
        project={project}
        onBack={() => { setProject(null); setPipeline(null); }}
        totalTokens={grandTotal}
        onShowHistory={() => setShowVersionHistory(true)}
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
              onNewProject={() => { setProject(null); setPipeline(null); }}
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
              <StageView tabs={tabs} activeTab={activeTab} onTabChange={setActiveTab}>
                {activeTab === 'chat' && (
                  <ChatPanel
                    messages={messages}
                    streaming={streaming}
                    streamingContent={streamingContent}
                    onSend={send}
                    onStop={stop}
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

                {selectedStage === 'build' && (
                  <BuildPanel
                    projectId={project.id}
                    refreshTrigger={artifactUpdated}
                    mode={activeTab === 'execute' ? 'execute' : 'artifact'}
                    onRequestExecuteTab={() => setActiveTab('execute')}
                    hidden={activeTab === 'chat'}
                    onBeadTokens={(n) => addTokens('build', n)}
                    onExecutionComplete={() => setBuildComplete(true)}
                    onBuildDone={() => setBuildComplete(true)}
                  />
                )}
              </StageView>

              {isActiveStage && (
                <div className="approve-bar">
                  {project.autonomous ? (
                    <span className="auto-approve-indicator">Auto-approve active — will advance automatically</span>
                  ) : (
                    <ApproveButton
                      projectId={project.id}
                      disabled={streaming || (selectedStage === 'ux' && !mockGenerated) || (selectedStage === 'build' && !buildComplete && !project?.imported)}
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
    </div>
  );
}

export default App;
