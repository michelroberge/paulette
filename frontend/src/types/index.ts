export type StageName = 'vision' | 'ux' | 'architecture' | 'build' | 'review' | 'complete';
export type StageStatus = 'locked' | 'active' | 'approved';

export interface Project {
  id: string;
  name: string;
  author: string;
  version: string;
  hostDir: string;
  currentStage: StageName;
  iteration: number;
  enhancementVision?: string;
  createdAt: string;
  updatedAt: string;
}

export type VersionBump = 'major' | 'minor' | 'patch';

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
  isError?: boolean;
}

export interface ChatHistory {
  messages: Message[];
}

export interface StreamEvent {
  type: 'chunk' | 'artifact' | 'done' | 'error' | 'tokens';
  content: string;
}

export type FrameworkId = 'tailwind' | 'bootstrap' | 'mui' | 'shadcn' | 'vanilla' | 'other';

export interface FrameworkConfig {
  framework: FrameworkId;
  customName?: string;
}

export type BeadStatus = 'open' | 'in_progress' | 'reviewing' | 'closed' | 'blocked';
export type BeadType = 'epic' | 'task' | 'feature';

export interface Bead {
  id: string;
  title: string;
  description?: string;
  type: BeadType;
  status: BeadStatus;
  priority: number;
  epicId?: string;
  deps: string[];
}

export interface BeadGraph {
  generatedAt: string;
  projectId: string;
  beads: Bead[];
}

export interface BuildStreamEvent {
  type: 'chunk' | 'bead_created' | 'bead_update' | 'log' | 'done' | 'error';
  content: string;
}
