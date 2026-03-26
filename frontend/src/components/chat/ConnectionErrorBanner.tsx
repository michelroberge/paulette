import { useNavigate } from 'react-router-dom';
import type { ConnectionError } from '../../types/provider';

// Re-export so callers only need one import site for the type.
export type { ConnectionError } from '../../types/provider';

/**
 * Attempt to parse a StreamEvent error content string as a ConnectionError.
 *
 * Returns the parsed object when ALL of the following are true:
 *   - The content is valid JSON
 *   - `isConnectionError` is exactly `true`
 *   - `connectionId`, `connectionName`, and `reason` are non-empty strings
 *
 * Falls back to `null` for plain-text errors, generic backend errors, or
 * any object that is missing / has blank required fields — preventing a
 * banner from rendering with blank connection name or reason text.
 */
export function parseConnectionError(content: string): ConnectionError | null {
  if (!content) return null;
  try {
    const parsed = JSON.parse(content);
    if (
      parsed &&
      parsed.isConnectionError === true &&
      typeof parsed.connectionId === 'string' && parsed.connectionId.trim() !== '' &&
      typeof parsed.connectionName === 'string' && parsed.connectionName.trim() !== '' &&
      typeof parsed.reason === 'string' && parsed.reason.trim() !== ''
    ) {
      return parsed as ConnectionError;
    }
  } catch {
    // Not JSON — treat as a regular error message.
  }
  return null;
}

interface ConnectionErrorBannerProps {
  /** Human-readable connection name, e.g. "Local Llama3". */
  connectionName: string;
  /** Connection ID used to build the deep-link highlight param. */
  connectionId: string;
  /** Short error reason, e.g. "Connection refused at http://localhost:11434". */
  reason: string;
  /**
   * Called when the user clicks "Change stage connection".
   * Should open the ProjectStageSettings slide-over for the current project.
   * If omitted, the secondary CTA is not rendered.
   */
  onOpenProjectSettings?: () => void;
  /**
   * Called when the user clicks "Retry".
   * Should resume the last unanswered user message without requiring the user
   * to re-type it. Typically wired to `resume()` from `useChat`.
   * If omitted, the Retry button is not rendered.
   * The banner is cleared automatically by `useChat` when `resume()` is called.
   */
  onRetry?: () => void;
}

/**
 * ConnectionErrorBanner — SCR-012
 *
 * Inline error banner rendered inside the Chat tab when the assigned LLM
 * provider connection is unreachable. Implements JRN-v0.2.0-008 / IACT-011.
 *
 * Per SCR-012: "The banner does not auto-dismiss; it persists until the user
 * either fixes the connection and retries, or navigates away." There is
 * intentionally no close/dismiss button — the only resolution paths are
 * fixing the connection (via "Go to Configure →") or reassigning the stage
 * (via "Change stage connection").
 *
 * CTAs:
 *   1. "Go to Configure →" — deep-links to /configure?tab=connections&highlight={id}
 *      so the failing connection card is highlighted with a 3-second red pulse.
 *   2. "Change stage connection" — opens the ProjectStageSettings panel so the
 *      user can reassign this stage without leaving the project context.
 */
export function ConnectionErrorBanner({
  connectionName,
  connectionId,
  reason,
  onOpenProjectSettings,
  onRetry,
}: ConnectionErrorBannerProps) {
  const navigate = useNavigate();

  const handleConfigure = () => {
    navigate(`/configure?tab=connections&highlight=${connectionId}`);
  };

  return (
    <div className="bg-red-50 border border-red-200 rounded-lg p-4 flex items-start gap-3 mx-4 my-2">
      {/* Warning icon */}
      <span className="text-red-500 text-sm mt-0.5 shrink-0" aria-hidden="true">
        ⚠
      </span>

      {/* Error body */}
      <div className="flex-1 min-w-0">
        <p className="text-red-800 text-sm font-medium">
          Connection &ldquo;{connectionName}&rdquo; is unreachable.
        </p>
        <p className="text-red-700 text-sm mt-0.5 break-words">{reason}</p>

        {/* Action links */}
        <div className="flex items-center flex-wrap gap-x-4 gap-y-1 mt-2">
          <button
            type="button"
            onClick={handleConfigure}
            className="text-blue-600 hover:underline font-medium text-sm"
          >
            Go to Configure →
          </button>

          {onOpenProjectSettings && (
            <button
              type="button"
              onClick={onOpenProjectSettings}
              className="text-gray-600 hover:underline text-sm"
            >
              Change stage connection
            </button>
          )}

          {onRetry && (
            <button
              type="button"
              onClick={onRetry}
              className="text-green-600 hover:underline font-medium text-sm"
            >
              ↺ Retry
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
