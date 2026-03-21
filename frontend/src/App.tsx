import { useState, useEffect, useCallback } from 'react';
import { ProjectList } from './components/project/ProjectList';
import { ProjectHeader } from './components/layout/ProjectHeader';
import { StagesSidebar } from './components/layout/StagesSidebar';
import { ChatPanel } from './components/chat/ChatPanel';
import { ArtifactPreview } from './components/artifact/ArtifactPreview';
import { ApproveButton } from './components/pipeline/ApproveButton';
import { getPipeline } from './api/pipeline';
import { useChat } from './hooks/useChat';
import type { Project, PipelineState, StageName } from './types';
import './App.css';

function App() {
  const [project, setProject] = useState<Project | null>(null);
  const [pipeline, setPipeline] = useState<PipelineState | null>(null);
  const [selectedStage, setSelectedStage] = useState<StageName | null>(null);

  const { messages, streaming, streamingContent, artifactUpdated, loadHistory, send } =
    useChat(project?.id ?? null, selectedStage);

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
        />

        <main className="main-content">
          <ChatPanel
            messages={messages}
            streaming={streaming}
            streamingContent={streamingContent}
            onSend={send}
          />

          {selectedStage && selectedStage !== 'complete' && (
            <ArtifactPreview
              projectId={project.id}
              stage={selectedStage}
              refreshTrigger={artifactUpdated}
            />
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
        </main>
      </div>
    </div>
  );
}

export default App;
