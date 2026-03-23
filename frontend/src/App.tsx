import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
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
import { getPipeline, approveStage, resetStage, getSummary } from './api/pipeline';
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

function getTabsForStage(stage: StageName | null): StageTab[] {
  if (!stage || stage === 'complete') return [];
  if (stage === 'ux') return [
    { id: 'chat', label: 'Chat' },
    { id: 'artifact', label: 'UX Design' },
    { id: 'mock', label: 'Mock Preview' },
  ];
  if (stage === 'build') return [
    { id: 'chat', label: 'Chat' },
    { id: 'artifact', label: 'Build Plan' },
    { id: 'execute', label: 'Execute' },
  ];
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

  // Reset tab and gate states on stage change
  useEffect(() => {
    setActiveTab('chat');
    setMockGenerated(false);
    setBuildComplete(false);
  }, [selectedStage]);

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
  useEffect(() => {
    if (!historyLoaded || messages.length > 0 || streaming) return;
    if (project?.imported) return;
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

  const approveAndAdvance = useCallback(async () => {
    if (!project) return;
    try {
      await approveStage(project.id);
      const state = await loadPipeline();
      if (state) setSelectedStage(state.currentStage);
      const updated = await getProject(project.id);
      setProject(updated);
    } catch (err) {
      console.error('Auto-approve failed:', err);
    }
  }, [project?.id, loadPipeline]); // eslint-disable-line react-hooks/exhaustive-deps

  // Auto-approve for vision & architecture (simple stages with no sub-steps)
  const prevArtifactUpdated = useRef(0);
  useEffect(() => {
    if (!project?.autonomous || !artifactUpdated || streaming) return;
    if (artifactUpdated === prevArtifactUpdated.current) return;
    prevArtifactUpdated.current = artifactUpdated;
    if (project.imported) return;
    if (selectedStage === 'complete') return;
    // UX and build have sub-steps; they approve via callbacks
    if (selectedStage === 'ux' || selectedStage === 'build') {
      // Auto-switch to the relevant tab
      if (selectedStage === 'ux') setActiveTab('mock');
      if (selectedStage === 'build') {
        // Autonomous: go straight to execute (autoGenerate calls generate() directly)
        // Non-autonomous: show plan first so user can review and click Generate Beads
        setActiveTab(project.autonomous ? 'execute' : 'artifact');
      }
      return;
    }
    const timer = setTimeout(() => approveAndAdvance(), 2000);
    return () => clearTimeout(timer);
  }, [artifactUpdated, streaming]); // eslint-disable-line react-hooks/exhaustive-deps

  // UX mock completion → enable approval (or auto-approve in autonomous mode)
  const handleMockComplete = useCallback(() => {
    setMockGenerated(true);
    if (!project?.autonomous) return;
    setTimeout(() => approveAndAdvance(), 2000);
  }, [project?.autonomous, approveAndAdvance]);

  // Build execution completion → enable approval (or auto-approve in autonomous mode)
  const handleExecutionComplete = useCallback(() => {
    setBuildComplete(true);
    if (!project?.autonomous) return;
    setTimeout(() => approveAndAdvance(), 2000);
  }, [project?.autonomous, approveAndAdvance]);

  // Auto-enhance after summary is ready
  const autoEnhanceTriggered = useRef(false);
  useEffect(() => {
    if (!project?.autonomous || !project.summaryReady) {
      autoEnhanceTriggered.current = false;
      return;
    }
    if (project.currentStage !== 'complete') return;
    if (project.iteration >= 10) return;
    if (autoEnhanceTriggered.current) return;
    autoEnhanceTriggered.current = true;

    const doAutoEnhance = async () => {
      try {
        const { content, exists } = await getSummary(project.id);
        if (!exists || !content) return;

        const match = content.match(/## Suggested Enhancements\n([\s\S]*?)(?=\n## |$)/);
        const suggestions = match?.[1]?.trim();
        if (!suggestions) return;

        await new Promise(resolve => setTimeout(resolve, 3000));
        await handleEnhance(suggestions, 'minor');
      } catch (err) {
        console.error('Auto-enhance failed:', err);
      }
    };

    doAutoEnhance();
  }, [project?.summaryReady, project?.autonomous, project?.currentStage]); // eslint-disable-line react-hooks/exhaustive-deps

  if (!project) {
    return <ProjectList onSelect={setProject} />;
  }

  const currentStageInfo = pipeline?.stages.find(s => s.name === selectedStage);
  const isActiveStage = currentStageInfo?.status === 'active';
  const tabs = getTabsForStage(selectedStage);

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
                    autoGenerate={!!project.autonomous && isActiveStage}
                    onMockComplete={handleMockComplete}
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
                    autoGenerate={!!project.autonomous && isActiveStage}
                    autoExecute={!!project.autonomous && isActiveStage}
                    onExecutionComplete={handleExecutionComplete}
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
                      disabled={streaming || (selectedStage === 'ux' && !mockGenerated) || (selectedStage === 'build' && !buildComplete)}
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
