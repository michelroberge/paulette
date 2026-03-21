export type StageName = 'vision' | 'ux' | 'architecture' | 'build' | 'review' | 'complete';
export type StageStatus = 'locked' | 'active' | 'approved';

export interface Project {
  id: string;
  name: string;
  author: string;
  version: string;
  hostDir: string;
  currentStage: StageName;
  createdAt: string;
  updatedAt: string;
}

export interface StageInfo {
  name: StageName;
  status: StageStatus;
  artifactPath: string;
}

export interface PipelineState {
  currentStage: StageName;
  stages: StageInfo[];
}

export interface Message {
  role: 'user' | 'assistant';
  content: string;
  timestamp: string;
}

export interface ChatHistory {
  messages: Message[];
}

export interface StreamEvent {
  type: 'chunk' | 'artifact' | 'done' | 'error';
  content: string;
}
