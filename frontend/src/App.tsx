import { useState, useEffect, useCallback } from 'react';
import { ProjectList } from './components/project/ProjectList';
import { ProjectHeader } from './components/layout/ProjectHeader';
import { StagesSidebar } from './components/layout/StagesSidebar';
import { ChatPanel } from './components/chat/ChatPanel';
import { ArtifactPreview } from './components/artifact/ArtifactPreview';
import { UxPanel } from './components/ux/UxPanel';
import { ApproveButton } from './components/pipeline/ApproveButton';
import { CompletionView } from './components/pipeline/CompletionView';
import { getPipeline, resetStage } from './api/pipeline';
import { useChat } from './hooks/useChat';
import type { Project, PipelineState, StageName } from './types';
import './App.css';

const KICKOFF_MESSAGES: Partial<Record<StageName, string>> = {
  vision: "Let's start building your product vision. What's the core idea — what problem are you solving and for whom?",
  ux: "I've reviewed the approved vision. Let me propose the initial user flows and screen descriptions for this product.",
  architecture: "I've reviewed the vision and UX design. Let me propose the technical architecture — stack, components, APIs, and data models.",
  build: "I've reviewed all approved artifacts. Let me create a concrete build plan with milestones and tasks.",
  review: "I've reviewed all approved artifacts. Let me perform a structured validation and give you my assessment.",
};

function App() {
  const [project, setProject] = useState<Project | null>(null);
  const [pipeline, setPipeline] = useState<PipelineState | null>(null);
  const [selectedStage, setSelectedStage] = useState<StageName | null>(null);
  const [chatReloadTrigger, setChatReloadTrigger] = useState(0);

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

  // Auto-kickoff: when entering a stage with no history, send the opening message
  useEffect(() => {
    if (!historyLoaded || messages.length > 0 || streaming) return;
    const currentStageInfo = pipeline?.stages.find(s => s.name === selectedStage);
    if (currentStageInfo?.status !== 'active') return;
    const kickoff = selectedStage ? KICKOFF_MESSAGES[selectedStage] : undefined;
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

  if (!project) {
    return <ProjectList onSelect={setProject} />;
  }

  const currentStageInfo = pipeline?.stages.find(s => s.name === selectedStage);
  const isActiveStage = currentStageInfo?.status === 'active';

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
            />
          ) : (
            <>
              <ChatPanel
                messages={messages}
                streaming={streaming}
                streamingContent={streamingContent}
                onSend={send}
                onStop={stop}
              />

              {selectedStage && selectedStage !== 'complete' && (
                selectedStage === 'ux' ? (
                  <UxPanel
                    projectId={project.id}
                    refreshTrigger={artifactUpdated}
                  />
                ) : (
                  <ArtifactPreview
                    projectId={project.id}
                    stage={selectedStage}
                    refreshTrigger={artifactUpdated}
                  />
                )
              )}

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
