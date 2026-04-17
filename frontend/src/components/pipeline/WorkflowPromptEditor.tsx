import { useCallback, useEffect, useState } from 'react';
import Editor from '@monaco-editor/react';
import { getPrompt, updatePrompt, resetPrompt, type PromptDetail } from '../../api/prompts';
import { getConnectionPrompt, updateConnectionPrompt, resetConnectionPrompt } from '../../api/connectionPrompts';

export type PromptTarget =
  | { type: 'project'; projectId: string; aiMode: 'files' | 'rag' }
  | { type: 'connection'; connectionId: string };

interface Props {
  target: PromptTarget;
  promptNames: string[];
  contextArtifacts?: string[];
  nodeDescription?: string | null;
  nodeLabel?: string;
  onClose: () => void;
}

export function WorkflowPromptEditor({ target, promptNames, contextArtifacts, nodeDescription, nodeLabel, onClose }: Readonly<Props>) {
  const [activeTab, setActiveTab] = useState(0);
  const [prompt, setPrompt] = useState<PromptDetail | null>(null);
  const [content, setContent] = useState('');
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const hasPrompts = promptNames.length > 0;
  const currentName = promptNames[activeTab] ?? promptNames[0];
  const editable = hasPrompts && (target.type === 'connection' || target.aiMode === 'files');

  const loadPrompt = useCallback(async (name: string) => {
    setError(null);
    try {
      const detail = target.type === 'project'
        ? await getPrompt(target.projectId, name)
        : await getConnectionPrompt(target.connectionId, name);
      setPrompt(detail);
      setContent(detail.content);
      setDirty(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load prompt');
    }
  }, [target]);

  useEffect(() => {
    if (currentName) loadPrompt(currentName);
  }, [currentName, loadPrompt]);

  const handleSave = async () => {
    if (!currentName || !editable) return;
    setSaving(true);
    try {
      if (target.type === 'project') {
        await updatePrompt(target.projectId, currentName, content);
      } else {
        await updateConnectionPrompt(target.connectionId, currentName, content);
      }
      setDirty(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  const handleReset = async () => {
    if (!currentName || !editable) return;
    setSaving(true);
    try {
      if (target.type === 'project') {
        await resetPrompt(target.projectId, currentName);
      } else {
        await resetConnectionPrompt(target.connectionId, currentName);
      }
      await loadPrompt(currentName);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to reset');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="workflow-prompt-editor">
      <div className="workflow-prompt-editor-header">
        <h3>{hasPrompts ? 'Prompt Template' : (nodeLabel || 'Details')}</h3>
        <button className="workflow-prompt-close" onClick={onClose}>&times;</button>
      </div>

      {nodeDescription && (
        <div className="workflow-prompt-description">
          {nodeDescription}
        </div>
      )}

      {contextArtifacts && contextArtifacts.length > 0 && (
        <div className="workflow-prompt-context">
          <span className="workflow-prompt-context-label">Context Artifacts:</span>
          {contextArtifacts.map(a => (
            <span key={a} className="workflow-prompt-context-file">{a}</span>
          ))}
        </div>
      )}

      {hasPrompts && (
        <>
          {promptNames.length > 1 && (
            <div className="workflow-prompt-tabs">
              {promptNames.map((name, i) => (
                <button
                  key={name}
                  className={`workflow-prompt-tab ${i === activeTab ? 'active' : ''}`}
                  onClick={() => setActiveTab(i)}
                >
                  {name.replace('.md.tmpl', '')}
                </button>
              ))}
            </div>
          )}

          {prompt && (
            <div className="workflow-prompt-meta">
              <span className="workflow-prompt-category">{prompt.category}</span>
              <span className="workflow-prompt-desc">{prompt.description}</span>
              {prompt.variables.length > 0 && (
                <div className="workflow-prompt-vars">
                  Variables: {prompt.variables.map(v => (
                    <code key={v}>{`{{.${v}}}`}</code>
                  ))}
                </div>
              )}
            </div>
          )}

          {error && <div className="workflow-prompt-error">{error}</div>}

          <div className="workflow-prompt-editor-body">
            <Editor
              height="100%"
              language="markdown"
              theme="vs-dark"
              value={content}
              onChange={val => { setContent(val ?? ''); setDirty(true); }}
              options={{
                readOnly: !editable,
                minimap: { enabled: false },
                wordWrap: 'on',
                lineNumbers: 'on',
                fontSize: 13,
                scrollBeyondLastLine: false,
              }}
            />
          </div>

          {editable && (
            <div className="workflow-prompt-actions">
              <button
                className="workflow-prompt-save"
                onClick={handleSave}
                disabled={!dirty || saving}
              >
                {saving ? 'Saving...' : 'Save'}
              </button>
              <button
                className="workflow-prompt-reset"
                onClick={handleReset}
                disabled={saving}
              >
                Reset to Default
              </button>
            </div>
          )}

          {!editable && (
            <div className="workflow-prompt-readonly-note">
              Read-only — switch to Files mode to edit prompts
            </div>
          )}
        </>
      )}
    </div>
  );
}
