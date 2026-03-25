import { useState, useEffect, useRef } from 'react';
import { importWatchUrl } from '../../api/import';
import { BuildingAnimation } from '../ux/BuildingAnimation';
import type { Project } from '../../types';

interface ImportStreamEvent {
  type: 'log' | 'artifact' | 'error' | 'done';
  content: string;
}

const STEPS = [
  { key: 'architecture', label: 'Architecture' },
  { key: 'ux', label: 'UX Design' },
  { key: 'crossref', label: 'Cross-references' },
  { key: 'vision', label: 'Vision' },
  { key: 'build', label: 'Build Plan' },
];

interface Props {
  project: Project;
  onComplete: () => void;
}

export function ImportProgressView({ project, onComplete }: Props) {
  const [logs, setLogs] = useState<string[]>([]);
  const [completedSteps, setCompletedSteps] = useState<Set<string>>(new Set());
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);
  const logEndRef = useRef<HTMLDivElement>(null);
  const artifactCount = useRef(0);

  useEffect(() => {
    const url = importWatchUrl(project.id);
    const source = new EventSource(url);

    source.onmessage = (e) => {
      const event: ImportStreamEvent = JSON.parse(e.data);

      switch (event.type) {
        case 'log':
          setLogs(prev => [...prev, event.content]);
          break;
        case 'artifact': {
          // Map artifact events to step completion
          // Architecture fires twice (initial + cross-ref update)
          const stage = event.content;
          if (stage === 'architecture') {
            artifactCount.current++;
            if (artifactCount.current === 1) {
              setCompletedSteps(prev => new Set([...prev, 'architecture']));
            } else {
              setCompletedSteps(prev => new Set([...prev, 'crossref']));
            }
          } else {
            setCompletedSteps(prev => new Set([...prev, stage]));
          }
          break;
        }
        case 'error':
          setError(event.content);
          break;
        case 'done':
          setDone(true);
          source.close();
          break;
      }
    };

    source.onerror = () => {
      source.close();
      // If not done yet, might have been a clean close
      if (!done) {
        setDone(true);
      }
    };

    return () => source.close();
  }, [project.id]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [logs]);

  return (
    <div className="completion-view" style={{ padding: '2rem' }}>
      <h2 style={{ marginBottom: '0.5rem' }}>Importing Project</h2>
      <p style={{ color: '#94a3b8', marginBottom: '2rem' }}>
        Analyzing codebase and generating pipeline artifacts...
      </p>

      <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem', marginBottom: '2rem' }}>
        {STEPS.map(step => {
          const isComplete = completedSteps.has(step.key);
          return (
            <div key={step.key} style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
              <span style={{
                width: '1.5rem',
                height: '1.5rem',
                borderRadius: '50%',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontSize: '0.75rem',
                background: isComplete ? '#22c55e' : '#334155',
                color: isComplete ? '#fff' : '#64748b',
                transition: 'all 0.3s',
              }}>
                {isComplete ? '\u2713' : '\u00B7'}
              </span>
              <span style={{ color: isComplete ? '#e2e8f0' : '#64748b' }}>
                {step.label}
              </span>
            </div>
          );
        })}
      </div>

      {!done && !error && <BuildingAnimation />}

      {error && (
        <div style={{ color: '#ef4444', padding: '1rem', background: '#1e1e2e', borderRadius: '0.5rem', marginBottom: '1rem' }}>
          Error: {error}
        </div>
      )}

      <div style={{
        background: '#0f172a',
        borderRadius: '0.5rem',
        padding: '1rem',
        maxHeight: '200px',
        overflow: 'auto',
        fontFamily: 'monospace',
        fontSize: '0.8rem',
        color: '#94a3b8',
      }}>
        {logs.map((log, i) => (
          <div key={i}>{log}</div>
        ))}
        <div ref={logEndRef} />
      </div>

      {done && !error && (
        <div style={{ marginTop: '1.5rem', textAlign: 'center' }}>
          <p style={{ color: '#22c55e', marginBottom: '1rem' }}>
            Import complete! Review each artifact and approve to continue.
          </p>
          <div className="form-actions" style={{ justifyContent: 'center' }}>
            <button className="primary" onClick={onComplete}>
              Start Review
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
