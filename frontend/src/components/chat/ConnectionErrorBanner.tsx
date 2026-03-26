import { useNavigate } from 'react-router-dom';

/**
 * Structured data embedded in a StreamEvent{type: 'error'} content field
 * when the error originates from a failing provider connection.
 * The backend encodes this as JSON in the content string.
 */
export interface ConnectionError {
  isConnectionError: true;
  connectionId: string;
  connectionName: string;
  reason: string;
}

/**
 * Attempt to parse a StreamEvent error content string as a ConnectionError.
 * Returns the parsed object if it is a connection error, or null otherwise.
 * Falls back gracefully for plain-text errors or generic backend errors.
 */
export function parseConnectionError(content: string): ConnectionError | null {
  if (!content) return null;
  try {
    const parsed = JSON.parse(content);
    if (parsed && parsed.isConnectionError === true) {
      return parsed as ConnectionError;
    }
  } catch {
    // Not JSON — treat as a regular error message
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
   * Called when the user dismisses the banner.
   * If omitted the banner renders without a close button (persists until navigation).
   */
  onDismiss?: () => void;
  /**
   * Called when the user clicks "Change stage connection".
   * Should open the ProjectStageSettings slide-over for the current project.
   * If omitted, the secondary CTA is not rendered.
   */
  onOpenProjectSettings?: () => void;
}

/**
 * ConnectionErrorBanner — SCR-012
 *
 * Inline error banner rendered inside the Chat tab when the assigned LLM
 * provider connection is unreachable. Implements JRN-v0.2.0-008 / IACT-011.
 *
 * The banner persists until the user dismisses it, navigates away, or retries.
 * It provides two actionable CTAs:
 *   1. "Go to Configure →" — deep-links to /configure?tab=connections&highlight={id}
 *      so the failing connection card is highlighted with a 3-second red pulse.
 *   2. "Change stage connection" — opens the ProjectStageSettings panel so the
 *      user can reassign this stage without leaving the project context.
 */
export function ConnectionErrorBanner({
  connectionName,
  connectionId,
  reason,
  onDismiss,
  onOpenProjectSettings,
}: ConnectionErrorBannerProps) {
  const navigate = useNavigate();

  const handleConfigure = () => {
    navigate(`/configure?tab=connections&highlight=${encodeURIComponent(connectionId)}`);
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
        {reason && (
          <p className="text-red-700 text-sm mt-0.5 break-words">{reason}</p>
        )}

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
        </div>
      </div>

      {/* Optional dismiss button */}
      {onDismiss && (
        <button
          type="button"
          onClick={onDismiss}
          className="text-red-400 hover:text-red-600 transition-colors shrink-0 leading-none text-lg"
          aria-label="Dismiss connection error"
        >
          ×
        </button>
      )}
    </div>
  );
}
