import type { ReactElement } from 'react';
import type { StageName, StageActivity } from '../../types';

// Each robot shares the same base face but has a distinct accessory marking its stage.
// Rendered as inline SVG so they scale with font-size and inherit currentColor.

const robots: Record<StageName, ReactElement> = {
  vision: (
    <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden>
      {/* Lightbulb antenna */}
      <line x1="12" y1="2" x2="12" y2="5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      <circle cx="12" cy="1.5" r="1" fill="currentColor"/>
      {/* Head */}
      <rect x="5" y="5" width="14" height="10" rx="2" stroke="currentColor" strokeWidth="1.5"/>
      {/* Eyes */}
      <circle cx="9" cy="9" r="1.5" fill="currentColor"/>
      <circle cx="15" cy="9" r="1.5" fill="currentColor"/>
      {/* Smile */}
      <path d="M9 12 Q12 14 15 12" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" fill="none"/>
      {/* Body */}
      <rect x="8" y="15" width="8" height="6" rx="1" stroke="currentColor" strokeWidth="1.5"/>
      {/* Legs */}
      <line x1="10" y1="21" x2="10" y2="24" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      <line x1="14" y1="21" x2="14" y2="24" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
    </svg>
  ),
  ux: (
    <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden>
      {/* Pencil antenna */}
      <line x1="12" y1="2" x2="12" y2="5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      <path d="M10.5 1 L13.5 1 L14 2 L12 2.5 L10 2 Z" fill="currentColor"/>
      {/* Head */}
      <rect x="5" y="5" width="14" height="10" rx="2" stroke="currentColor" strokeWidth="1.5"/>
      {/* Eyes — one winking */}
      <circle cx="9" cy="9" r="1.5" fill="currentColor"/>
      <line x1="13.5" y1="9" x2="16.5" y2="9" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      {/* Smile */}
      <path d="M9 12 Q12 14 15 12" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" fill="none"/>
      {/* Body */}
      <rect x="8" y="15" width="8" height="6" rx="1" stroke="currentColor" strokeWidth="1.5"/>
      {/* Arms holding palette */}
      <line x1="5" y1="17" x2="8" y2="17" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      <circle cx="4" cy="17" r="1.5" stroke="currentColor" strokeWidth="1.2"/>
      <line x1="16" y1="21" x2="16" y2="24" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      <line x1="12" y1="21" x2="12" y2="24" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
    </svg>
  ),
  architecture: (
    <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden>
      {/* Gear antenna */}
      <line x1="12" y1="2" x2="12" y2="5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      <circle cx="12" cy="1.5" r="1.5" stroke="currentColor" strokeWidth="1.2"/>
      <circle cx="12" cy="1.5" r="0.5" fill="currentColor"/>
      {/* Head */}
      <rect x="5" y="5" width="14" height="10" rx="2" stroke="currentColor" strokeWidth="1.5"/>
      {/* Gear eye on the left */}
      <circle cx="9" cy="9" r="2" stroke="currentColor" strokeWidth="1.2"/>
      <circle cx="9" cy="9" r="0.7" fill="currentColor"/>
      {/* Regular eye on the right */}
      <circle cx="15" cy="9" r="1.5" fill="currentColor"/>
      {/* Neutral mouth */}
      <line x1="9" y1="13" x2="15" y2="13" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      {/* Body */}
      <rect x="8" y="15" width="8" height="6" rx="1" stroke="currentColor" strokeWidth="1.5"/>
      {/* Legs */}
      <line x1="10" y1="21" x2="10" y2="24" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      <line x1="14" y1="21" x2="14" y2="24" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
    </svg>
  ),
  build: (
    <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden>
      {/* Wrench antenna */}
      <line x1="12" y1="2" x2="12" y2="5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      <path d="M10 0.5 Q12 2 14 0.5" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" fill="none"/>
      {/* Head */}
      <rect x="5" y="5" width="14" height="10" rx="2" stroke="currentColor" strokeWidth="1.5"/>
      {/* Eyes — determined look */}
      <circle cx="9" cy="9" r="1.5" fill="currentColor"/>
      <circle cx="15" cy="9" r="1.5" fill="currentColor"/>
      {/* Frown of concentration */}
      <path d="M9 13 Q12 11.5 15 13" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" fill="none"/>
      {/* Body */}
      <rect x="8" y="15" width="8" height="6" rx="1" stroke="currentColor" strokeWidth="1.5"/>
      {/* Arm with wrench */}
      <line x1="16" y1="17" x2="20" y2="15" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      <circle cx="21" cy="14.5" r="1.2" stroke="currentColor" strokeWidth="1.2"/>
      {/* Leg */}
      <line x1="10" y1="21" x2="10" y2="24" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      <line x1="14" y1="21" x2="14" y2="24" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
    </svg>
  ),
  complete: (
    <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden>
      {/* Star antenna */}
      <line x1="12" y1="2.5" x2="12" y2="5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      <path d="M12 0 L12.5 1.5 L14 1.5 L13 2.3 L13.4 3.8 L12 3 L10.6 3.8 L11 2.3 L10 1.5 L11.5 1.5 Z" fill="currentColor"/>
      {/* Head */}
      <rect x="5" y="5" width="14" height="10" rx="2" stroke="currentColor" strokeWidth="1.5"/>
      {/* Eyes — happy */}
      <path d="M7.5 8.5 Q9 7 10.5 8.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" fill="none"/>
      <path d="M13.5 8.5 Q15 7 16.5 8.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" fill="none"/>
      {/* Big smile */}
      <path d="M8 12 Q12 15 16 12" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" fill="none"/>
      {/* Body */}
      <rect x="8" y="15" width="8" height="6" rx="1" stroke="currentColor" strokeWidth="1.5"/>
      {/* Raised arms */}
      <line x1="8" y1="17" x2="4" y2="13" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      <line x1="16" y1="17" x2="20" y2="13" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      {/* Legs */}
      <line x1="10" y1="21" x2="10" y2="24" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      <line x1="14" y1="21" x2="14" y2="24" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
    </svg>
  ),
  ui: (
    <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden>
      {/* Palette antenna */}
      <line x1="12" y1="2" x2="12" y2="5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      <circle cx="12" cy="1.5" r="1" fill="currentColor"/>
      {/* Head */}
      <rect x="5" y="5" width="14" height="10" rx="2" stroke="currentColor" strokeWidth="1.5"/>
      {/* Eyes */}
      <circle cx="9" cy="9" r="1.5" fill="currentColor"/>
      <circle cx="15" cy="9" r="1.5" fill="currentColor"/>
      {/* Smile */}
      <path d="M9 12 Q12 14 15 12" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" fill="none"/>
      {/* Body */}
      <rect x="8" y="15" width="8" height="6" rx="1" stroke="currentColor" strokeWidth="1.5"/>
      {/* Legs */}
      <line x1="10" y1="21" x2="10" y2="24" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      <line x1="14" y1="21" x2="14" y2="24" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
    </svg>
  ),
};

interface Props {
  stage: StageName;
  activity?: StageActivity;
  size?: 'normal' | 'small';
}

export function StageRobot({ stage, activity, size = 'normal' }: Props) {
  const title = activity?.status === 'running'
    ? `${stage}: ${activity.operation} running…`
    : activity?.status === 'failed'
    ? `${stage}: ${activity.error ?? 'operation failed'}`
    : stage;

  return (
    <span
      className={`stage-robot stage-robot--${size} ${activity?.status ? `stage-robot--${activity.status}` : ''}`}
      title={title}
    >
      {robots[stage]}
      {activity?.status === 'running' && <span className="robot-pulse-ring" aria-hidden />}
      {activity?.status === 'failed' && <span className="robot-fail-badge" aria-label="failed">!</span>}
    </span>
  );
}
