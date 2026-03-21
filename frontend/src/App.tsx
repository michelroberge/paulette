import { useState, useEffect, useCallback } from 'react';
import { ProjectList } from './components/project/ProjectList';
import { ProjectHeader } from './components/layout/ProjectHeader';
import { StagesSidebar } from './components/layout/StagesSidebar';
import { StageView } from './components/layout/StageView';
import { ChatPanel } from './components/chat/ChatPanel';
import { ArtifactPreview } from './components/artifact/ArtifactPreview';
import { UxPanel } from './components/ux/UxPanel';
import { BuildPanel } from './components/build/BuildPanel';
import { ApproveButton } from './components/pipeline/ApproveButton';
import { CompletionView } from './components/pipeline/CompletionView';
import { getPipeline, resetStage } from './api/pipeline';
import { startEnhancement } from './api/enhance';
import { useChat } from './hooks/useChat';
import type { Project, PipelineState, StageName, VersionBump } from './types';
import './App.css';

const KICKOFF_MESSAGES: Partial<Record<StageName, string>> = {
  vision: "Let's start building your product vision. What's the core idea — what problem are you solving and for whom?",
  ux: "I've reviewed the approved vision. Let me propose the initial user flows and screen descriptions for this product.",
  architecture: "I've reviewed the vision and UX design. Let me propose the technical architecture — stack, components, APIs, and data models.",
  build: "I've reviewed all approved artifacts. Let me create a concrete build plan with milestones and tasks.",
  review: "I've reviewed all approved artifacts. Let me perform a structured validation and give you my assessment.",
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

  const { messages, streaming, streamingContent, artifactUpdated, historyLoaded, loadHistory, send, stop } =
    useChat(project?.id ?? null, selectedStage, chatReloadTrigger);

  const loadPipeline = useCallback(async () => {
    if (!project) return;
    const state = await getPipeline(project.id);
    setPipeline(state);
    return state;
  }, [project]);

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

  // Reset tab on stage change
  useEffect(() => {
    setActiveTab('chat');
  }, [selectedStage]);

  // Auto-kickoff: when entering a stage with no history, send the opening message
  useEffect(() => {
    if (!historyLoaded || messages.length > 0 || streaming) return;
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

  if (!project) {
    return <ProjectList onSelect={setProject} />;
  }

  const currentStageInfo = pipeline?.stages.find(s => s.name === selectedStage);
  const isActiveStage = currentStageInfo?.status === 'active';
  const tabs = getTabsForStage(selectedStage);

  return (
    <div className="app-shell">
      <ProjectHeader project={project} onBack={() => { setProject(null); setPipeline(null); }} />

      <div className="app-body">
        <StagesSidebar
          pipeline={pipeline}
          selectedStage={selectedStage}
          onSelectStage={setSelectedStage}
          onReset={handleReset}
        />

        <main className="main-content">
          {pipeline?.currentStage === 'complete' && selectedStage === 'complete' ? (
            <CompletionView
              project={project}
              onNewProject={() => { setProject(null); setPipeline(null); }}
              onViewStage={stage => setSelectedStage(stage)}
              onEnhance={handleEnhance}
            />
          ) : (
            <>
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
                  />
                )}

                {selectedStage === 'build' && (
                  <BuildPanel
                    projectId={project.id}
                    refreshTrigger={artifactUpdated}
                    mode={activeTab === 'execute' ? 'execute' : 'artifact'}
                    onRequestExecuteTab={() => setActiveTab('execute')}
                    hidden={activeTab === 'chat'}
                  />
                )}
              </StageView>

              {isActiveStage && (
                <div className="approve-bar">
                  <ApproveButton
                    projectId={project.id}
                    disabled={streaming}
                    onApproved={handleApproved}
                  />
                </div>
              )}
            </>
          )}
        </main>
      </div>
    </div>
  );
}

export default App;
