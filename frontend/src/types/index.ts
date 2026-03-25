export type StageName = 'vision' | 'ux' | 'architecture' | 'build' | 'complete';
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
  imported?: boolean;
  summaryReady: boolean;
  summaryApproved?: boolean;
  autonomous?: boolean;
  baseBranch?: string;
  devCommands?: string[];
  buildCommands?: string[];
  runCommands?: string[];
  summaryTokens?: number;
  stageTokens?: Partial<Record<StageName, number>>;
  createdAt: string;
  updatedAt: string;
}

export type VersionBump = 'major' | 'minor' | 'patch';

export interface StageActivity {
  operation: string;   // "chat" | "mock" | "beads-generate" | "beads-execute" | "summary"
  status: 'running' | 'failed';
  startedAt: string;
  error?: string;
  pendingBtw?: Array<{ message: string; sentAt: string }>;
}

export interface StageInfo {
  name: StageName;
  status: StageStatus;
  artifactPath: string;
  activity?: StageActivity;
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
  nextTurn: 'agent' | 'user';
}

export interface StreamEvent {
  type: 'chunk' | 'artifact' | 'done' | 'error' | 'tokens' | 'plan_limit';
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
  tags?: string[];
  targetFiles?: string[];
  journeyRefs?: string[];
  archRefs?: string[];
  tokens?: number;
  preExecutionCommit?: string;
}

export interface BeadFileEntry {
  path: string;
  content: string;
  language: string;
}

export interface BeadFilesResponse {
  files: BeadFileEntry[];
}

export interface BeadDiffEntry {
  path: string;
  original: string;
  modified: string;
  language: string;
}

export interface BeadDiffResponse {
  diffs: BeadDiffEntry[];
}

export interface InstructBead {
  title: string;
  description: string;
  type: string;
  epicId?: string;
  deps?: string[];
  targetFiles?: string[];
  tags?: string[];
  priority: number;
}

export interface InstructUpdate {
  id: string;
  description?: string;
  title?: string;
}

export interface InstructionPlan {
  reasoning: string;
  buildPlanChanges?: string;
  architectureChanges?: string;
  newBeads?: InstructBead[];
  updatedBeads?: InstructUpdate[];
}

export interface BeadGraph {
  generatedAt: string;
  projectId: string;
  beads: Bead[];
}

export interface BeadDetail extends Bead {
  notes: string;
  executionContent: string;
  chatMessages: Message[];
}

export interface BuildStreamEvent {
  type: 'chunk' | 'bead_created' | 'bead_update' | 'log' | 'done' | 'error' | 'plan_limit';
  content: string;
}

export interface CommitEntry {
  hash: string;
  shortHash: string;
  message: string;
  author: string;
  date: string;
  tags: string[];
}

export interface GitStatus {
  clean: boolean;
  dirty: number;
  hasRemote: boolean;
  remoteUrl: string;
  branch: string;
  remoteBranch: string;
  gitUserName: string;
  gitUserEmail: string;
}

// Session token tracking
export type SessionKind = 'chat' | 'mock' | 'beads-generate' | 'beads-execute' | 'summary';

export interface Session {
  id: string;
  stage: StageName;
  kind: SessionKind;
  iteration: number;
  startedAt: string;
  endedAt: string;
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
}

export interface StageSummary {
  count: number;
  tokens: number;
}

export interface SessionSummary {
  sessions: Session[];
  byStage: Partial<Record<StageName, StageSummary>>;
  grandTotal: number;
}

// Skills
export interface SkillParam {
  name: string;
  description: string;
  default?: string;
}

export interface Skill {
  id: string;
  name: string;
  description: string;
  category: string;
  tags: string[];
  version: number;
  parameters?: SkillParam[];
  createdAt: string;
  updatedAt: string;
  createdBy: string;
}

export interface SkillSuggestion {
  name: string;
  description: string;
  category: string;
  tags: string[];
  parameters?: SkillParam[];
  promptTemplate: string;
  approved: boolean;
  sourceBeads?: string[];
}

export interface SkillSuggestions {
  suggestions: SkillSuggestion[];
}
