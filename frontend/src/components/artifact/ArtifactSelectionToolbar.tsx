import { useState, useEffect, useRef, useCallback } from 'react';
import { refineArtifact, applyRefine, manualEditArtifact } from '../../api/refine';
import type { StreamEvent, Message } from '../../types';

interface Props {
  projectId: string;
  stage: string;
  onArtifactUpdated: () => void;
}

interface ToolbarPosition {
  top: number;
  left: number;
}

export function ArtifactSelectionToolbar({ projectId, stage, onArtifactUpdated }: Props) {
  const [selectedText, setSelectedText] = useState('');
  const [toolbarPos, setToolbarPos] = useState<ToolbarPosition | null>(null);
  const [mode, setMode] = useState<'idle' | 'finetune' | 'manual'>('idle');
  const [instruction, setInstruction] = useState('');
  const [streaming, setStreaming] = useState(false);
  const [streamedContent, setStreamedContent] = useState('');
  const [refinedText, setRefinedText] = useState('');
  const [conversationHistory, setConversationHistory] = useState<Message[]>([]);
  const [manualText, setManualText] = useState('');
  const [applying, setApplying] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  const handleSelection = useCallback(() => {
    const selection = window.getSelection();
    if (!selection || selection.isCollapsed || !selection.toString().trim()) {
      // Don't dismiss if in finetune/manual mode
      if (mode === 'idle') {
        setToolbarPos(null);
        setSelectedText('');
      }
      return;
    }

    // Check if selection is within an artifact-content element
    const range = selection.getRangeAt(0);
    const container = range.commonAncestorContainer;
    const artifactEl = (container instanceof Element ? container : container.parentElement)?.closest('.artifact-content');
    if (!artifactEl) {
      if (mode === 'idle') {
        setToolbarPos(null);
        setSelectedText('');
      }
      return;
    }

    const text = selection.toString().trim();
    if (text) {
      const rect = range.getBoundingClientRect();
      const parentRect = artifactEl.closest('.artifact-preview')?.getBoundingClientRect();
      if (parentRect) {
        setToolbarPos({
          top: rect.top - parentRect.top - 40,
          left: rect.left - parentRect.left + rect.width / 2,
        });
      }
      setSelectedText(text);
    }
  }, [mode]);

  useEffect(() => {
    document.addEventListener('mouseup', handleSelection);
    return () => document.removeEventListener('mouseup', handleSelection);
  }, [handleSelection]);

  const handleFineTune = () => {
    setMode('finetune');
    setInstruction('');
    setStreamedContent('');
    setRefinedText('');
    setConversationHistory([]);
  };

  const handleManualEdit = () => {
    setMode('manual');
    setManualText(selectedText);
  };

  const handleSendInstruction = async () => {
    if (!instruction.trim() || streaming) return;
    setStreaming(true);
    setStreamedContent('');
    setRefinedText('');

    const controller = new AbortController();
    abortRef.current = controller;
    let accumulated = '';

    try {
      await refineArtifact(
        projectId,
        stage,
        {
          selection: selectedText,
          instruction: instruction.trim(),
          conversationHistory,
        },
        (event: StreamEvent) => {
          if (event.type === 'chunk') {
            accumulated += event.content;
            setStreamedContent(accumulated);
          } else if (event.type === 'done') {
            const final = event.content || accumulated;
            setRefinedText(final);
            setStreamedContent(final);
            setStreaming(false);
            // Update conversation history for follow-up refinements
            setConversationHistory(prev => [
              ...prev,
              { role: 'user', content: instruction.trim(), timestamp: new Date().toISOString() },
              { role: 'assistant', content: final, timestamp: new Date().toISOString() },
            ]);
          } else if (event.type === 'error') {
            setStreaming(false);
          }
        },
        controller.signal,
      );
    } catch {
      setStreaming(false);
    }
    setInstruction('');
  };

  const handleAcceptRefine = async () => {
    if (!refinedText || applying) return;
    setApplying(true);
    try {
      await applyRefine(projectId, stage, selectedText, refinedText);
      onArtifactUpdated();
      resetState();
    } catch (err) {
      console.error('Failed to apply refinement:', err);
    } finally {
      setApplying(false);
    }
  };

  const handleAcceptManual = async () => {
    if (applying) return;
    setApplying(true);
    try {
      await manualEditArtifact(projectId, stage, selectedText, manualText);
      onArtifactUpdated();
      resetState();
    } catch (err) {
      console.error('Failed to apply manual edit:', err);
    } finally {
      setApplying(false);
    }
  };

  const resetState = () => {
    setMode('idle');
    setSelectedText('');
    setToolbarPos(null);
    setInstruction('');
    setStreamedContent('');
    setRefinedText('');
    setConversationHistory([]);
    setManualText('');
    abortRef.current?.abort();
  };

  // Floating toolbar on text selection
  if (mode === 'idle' && toolbarPos && selectedText) {
    return (
      <div
        className="artifact-selection-toolbar"
        style={{
          position: 'absolute',
          top: `${toolbarPos.top}px`,
          left: `${toolbarPos.left}px`,
          transform: 'translateX(-50%)',
          zIndex: 100,
        }}
      >
        <button onClick={handleFineTune} className="toolbar-btn toolbar-btn-ai">
          Fine Tune with AI
        </button>
        <button onClick={handleManualEdit} className="toolbar-btn toolbar-btn-edit">
          Manually Edit
        </button>
      </div>
    );
  }

  // Fine Tune with AI dialog
  if (mode === 'finetune') {
    return (
      <div className="artifact-refine-dialog">
        <div className="refine-header">
          <h4>Fine Tune with AI</h4>
          <button onClick={resetState} className="refine-close">&times;</button>
        </div>
        <div className="refine-selection-preview">
          <label>Selected text:</label>
          <div className="refine-selection-text">{selectedText}</div>
        </div>
        {conversationHistory.length > 0 && (
          <div className="refine-history">
            {conversationHistory.map((msg, i) => (
              <div key={i} className={`refine-msg refine-msg-${msg.role}`}>
                <strong>{msg.role === 'user' ? 'You' : 'AI'}:</strong> {msg.content}
              </div>
            ))}
          </div>
        )}
        {(streaming || streamedContent) && !refinedText && (
          <div className="refine-streaming">
            <label>AI suggestion:</label>
            <div className="refine-stream-text">{streamedContent || 'Thinking...'}</div>
          </div>
        )}
        {refinedText && (
          <div className="refine-result">
            <label>Refined text:</label>
            <div className="refine-result-text">{refinedText}</div>
            <div className="refine-actions">
              <button onClick={handleAcceptRefine} disabled={applying} className="refine-btn refine-btn-accept">
                {applying ? 'Applying...' : 'Accept'}
              </button>
              <button onClick={resetState} className="refine-btn refine-btn-cancel">
                Cancel
              </button>
            </div>
          </div>
        )}
        <div className="refine-input-row">
          <input
            type="text"
            value={instruction}
            onChange={e => setInstruction(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && handleSendInstruction()}
            placeholder={conversationHistory.length > 0 ? "Refine further..." : "How should this be changed?"}
            disabled={streaming}
            className="refine-input"
            autoFocus
          />
          <button
            onClick={handleSendInstruction}
            disabled={streaming || !instruction.trim()}
            className="refine-btn refine-btn-send"
          >
            {streaming ? '...' : 'Send'}
          </button>
        </div>
      </div>
    );
  }

  // Manual Edit dialog
  if (mode === 'manual') {
    return (
      <div className="artifact-refine-dialog">
        <div className="refine-header">
          <h4>Manual Edit</h4>
          <button onClick={resetState} className="refine-close">&times;</button>
        </div>
        <textarea
          value={manualText}
          onChange={e => setManualText(e.target.value)}
          className="refine-textarea"
          rows={Math.min(20, manualText.split('\n').length + 3)}
          autoFocus
        />
        <div className="refine-actions">
          <button onClick={handleAcceptManual} disabled={applying} className="refine-btn refine-btn-accept">
            {applying ? 'Applying...' : 'OK'}
          </button>
          <button onClick={resetState} className="refine-btn refine-btn-cancel">
            Cancel
          </button>
        </div>
      </div>
    );
  }

  return null;
}
