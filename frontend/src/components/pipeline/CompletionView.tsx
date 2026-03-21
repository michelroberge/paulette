import { useState } from 'react';
import type { Project, StageName, VersionBump } from '../../types';

const ARTIFACT_STAGES: { name: StageName; label: string }[] = [
  { name: 'vision', label: 'Vision' },
  { name: 'ux', label: 'UX Design' },
  { name: 'architecture', label: 'Architecture' },
  { name: 'build', label: 'Build Plan' },
  { name: 'review', label: 'Review Report' },
];

interface Props {
  project: Project;
  onNewProject: () => void;
  onViewStage: (stage: StageName) => void;
  onEnhance: (vision: string, bump: VersionBump) => void;
}

export function CompletionView({ project, onNewProject, onViewStage, onEnhance }: Props) {
  const [showEnhanceForm, setShowEnhanceForm] = useState(false);
  const [enhanceVision, setEnhanceVision] = useState('');
  const [versionBump, setVersionBump] = useState<VersionBump>('minor');
  const [submitting, setSubmitting] = useState(false);

  const handleEnhance = async () => {
    if (!enhanceVision.trim()) return;
    setSubmitting(true);
    try {
      onEnhance(enhanceVision.trim(), versionBump);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="completion-view">
      <div className="completion-header">
        <div className="completion-icon">✓</div>
        <h2>Pipeline Complete</h2>
        <p className="completion-subtitle">
          All stages have been approved for <strong>{project.name}</strong> v{project.version}
          {project.iteration > 1 && <span> (iteration {project.iteration})</span>}.
        </p>
      </div>

      <div className="completion-artifacts">
        <h3>Approved Artifacts</h3>
        <ul>
          {ARTIFACT_STAGES.map(({ name, label }) => (
            <li key={name} className="completion-artifact-item">
              <span className="artifact-check">✓</span>
              <button className="artifact-link" onClick={() => onViewStage(name)}>
                {label}
              </button>
            </li>
          ))}
        </ul>
      </div>

      {!showEnhanceForm ? (
        <div className="completion-actions">
          <button className="approve-button" onClick={() => setShowEnhanceForm(true)}>
            Enhance
          </button>
          <button className="approve-button secondary" onClick={onNewProject}>
            Start New Project
          </button>
        </div>
      ) : (
        <div className="enhance-form">
          <h3>Enhance {project.name}</h3>
          <p className="enhance-description">
            Describe how you want to improve your app. The previous iteration's artifacts
            will be used as a starting point for each stage.
          </p>
          <textarea
            className="enhance-vision-input"
            placeholder="I want to improve my app like this: ..."
            value={enhanceVision}
            onChange={e => setEnhanceVision(e.target.value)}
            rows={4}
          />
          <div className="version-bump-selector">
            <label>Version bump (current: v{project.version}):</label>
            <div className="version-bump-options">
              {(['patch', 'minor', 'major'] as VersionBump[]).map(bump => (
                <label key={bump} className="version-bump-option">
                  <input
                    type="radio"
                    name="versionBump"
                    value={bump}
                    checked={versionBump === bump}
                    onChange={() => setVersionBump(bump)}
                  />
                  {bump.charAt(0).toUpperCase() + bump.slice(1)}
                </label>
              ))}
            </div>
          </div>
          <div className="enhance-form-actions">
            <button
              className="approve-button"
              onClick={handleEnhance}
              disabled={!enhanceVision.trim() || submitting}
            >
              {submitting ? 'Starting...' : 'Start Enhancement'}
            </button>
            <button
              className="approve-button secondary"
              onClick={() => { setShowEnhanceForm(false); setEnhanceVision(''); }}
            >
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
