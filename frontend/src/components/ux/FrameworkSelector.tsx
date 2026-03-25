import { useState } from 'react';
import { useFramework } from '../../hooks/useFramework';
import type { FrameworkId } from '../../types';

interface Props {
  projectId: string;
}

const FRAMEWORK_OPTIONS: { id: FrameworkId; label: string }[] = [
  { id: 'tailwind',  label: 'Tailwind CSS' },
  { id: 'bootstrap', label: 'Bootstrap 5' },
  { id: 'mui',       label: 'Material UI' },
  { id: 'shadcn',    label: 'Shadcn/UI' },
  { id: 'vanilla',   label: 'Vanilla CSS' },
  { id: 'other',     label: 'Other...' },
];

export function FrameworkSelector({ projectId }: Props) {
  const { config, saving, update } = useFramework(projectId);
  const [customName, setCustomName] = useState('');
  const [showCustomInput, setShowCustomInput] = useState(false);

  const handleChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const val = e.target.value as FrameworkId;
    if (val === 'other') {
      setShowCustomInput(true);
      setCustomName(config.customName ?? '');
    } else {
      setShowCustomInput(false);
      update(val);
    }
  };

  const handleCustomSubmit = () => {
    const name = customName.trim();
    if (name) {
      update('other', name);
      setShowCustomInput(false);
    }
  };

  return (
    <div className="framework-selector">
      <select
        className="framework-select"
        value={config.framework}
        onChange={handleChange}
        disabled={saving}
        title="UI Framework"
      >
        {FRAMEWORK_OPTIONS.map(opt => (
          <option key={opt.id} value={opt.id}>{opt.label}</option>
        ))}
      </select>
      {showCustomInput && (
        <input
          className="framework-custom-input"
          placeholder="Framework name..."
          value={customName}
          onChange={e => setCustomName(e.target.value)}
          onBlur={handleCustomSubmit}
          onKeyDown={e => { if (e.key === 'Enter') handleCustomSubmit(); if (e.key === 'Escape') setShowCustomInput(false); }}
          autoFocus
        />
      )}
    </div>
  );
}
