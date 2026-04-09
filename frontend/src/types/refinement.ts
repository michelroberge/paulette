export type SectionName =
  | 'problem'
  | 'users'
  | 'features'
  | 'ux'
  | 'metrics'
  | 'constraints'
  | 'out_of_scope';

export type LoopPhase =
  | 'init'
  | 'summarize'
  | 'generate_questions'
  | 'await_answer'
  | 'extract_facts'
  | 'update_sections'
  | 'score_confidence'
  | 'find_gaps'
  | 'critique'
  | 'synthesize'
  | 'complete';

export interface RefinementQuestion {
  id: string;
  text: string;
  section: SectionName;
  impact: number;
  answered: boolean;
  answer?: string;
}

export type ReviewSource = 'critique' | 'coherence' | 'tension' | 'gap';
export type ReviewStatus = 'pending' | 'addressed' | 'discarded';

export interface ReviewItem {
  id: string;
  text: string;
  source: ReviewSource;
  section: SectionName;
  iteration: number;
  status: ReviewStatus;
  created_at: string;
}

export interface RefinementState {
  idea_raw: string;
  idea_summary: string;
  sections: Partial<Record<SectionName, string>>;
  known_facts: string[];
  open_questions: RefinementQuestion[];
  confidence: Partial<Record<SectionName, number>>;
  iteration: number;
  max_iterations: number;
  phase: LoopPhase;
  error?: string;
  review_items?: ReviewItem[];
}

export interface RefinementEvent {
  phase: LoopPhase;
  iteration: number;
  maxIter: number;
  confidence?: Partial<Record<SectionName, number>>;
  question?: RefinementQuestion;
  message?: string;
  reviewItem?: ReviewItem;
}
