import type { ReactNode } from 'react';

interface StageTab {
  id: string;
  label: string;
  dimmed?: boolean;
}

interface Props {
  tabs: StageTab[];
  activeTab: string;
  onTabChange: (id: string) => void;
  children: ReactNode;
}

export function StageView({ tabs, activeTab, onTabChange, children }: Props) {
  return (
    <div className="stage-view">
      <div className="stage-tab-bar">
        {tabs.map(t => (
          <button
            key={t.id}
            className={`stage-tab${activeTab === t.id ? ' active' : ''}${t.dimmed ? ' dimmed' : ''}`}
            onClick={() => onTabChange(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>
      <div className="stage-tab-content">{children}</div>
    </div>
  );
}
