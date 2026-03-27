/**
 * CredentialWarningToast — fixed bottom-right toast shown whenever API credentials
 * are written to disk during a connection save operation.
 *
 * Behaviour (IACT-009 / ARCH-v0.2.0-028):
 *   - Fixed `bottom-4 right-4` yellow warning styling.
 *   - Auto-dismisses after 6 seconds with a CSS fade-out transition.
 *   - Caller controls visibility via the `visible` prop; the component manages its
 *     own fade-out animation and calls `onDismiss` after the transition completes.
 *   - Only rendered by callers when the provider has credentials — Ollama, LM Studio,
 *     and Claude CLI connections should NOT trigger this toast.
 *
 * Journey: JRN-v0.2.0-002, JRN-v0.2.0-003
 * Architecture: ARCH-v0.2.0-028, IACT-009
 */

import { useState, useEffect, useRef } from 'react';
import type { CSSProperties } from 'react';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface CredentialWarningToastProps {
  /**
   * When `true`, the toast becomes visible and starts the 6-second auto-dismiss
   * timer. Toggling back to `false` externally will also hide the toast
   * (e.g. if the parent unmounts).
   */
  visible: boolean;
  /**
   * Called after the toast finishes its fade-out transition so the parent can
   * reset its `visible` state or do any cleanup.
   */
  onDismiss: () => void;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

/** Total time (ms) the toast is displayed before starting its fade-out. */
const DISPLAY_DURATION_MS = 6000;

/** Duration (ms) of the CSS opacity fade-out transition. */
const FADE_OUT_DURATION_MS = 300;

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

const toastStyle: CSSProperties = {
  position: 'fixed',
  bottom: '1rem',       // bottom-4
  right: '1rem',        // right-4
  zIndex: 50,           // z-50
  maxWidth: '24rem',    // max-w-sm
};

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * CredentialWarningToast
 *
 * Usage:
 * ```tsx
 * const [showToast, setShowToast] = useState(false);
 *
 * // After saving a connection that has credentials:
 * setShowToast(true);
 *
 * <CredentialWarningToast
 *   visible={showToast}
 *   onDismiss={() => setShowToast(false)}
 * />
 * ```
 *
 * Provider filtering: the caller is responsible for only setting `visible=true`
 * when the saved connection type requires credentials. Do NOT show this toast for:
 *   - Ollama (no credentials)
 *   - LM Studio (no credentials)
 *   - Claude CLI (no credentials)
 */
export function CredentialWarningToast({
  visible,
  onDismiss,
}: CredentialWarningToastProps) {
  /**
   * `opacity` drives the CSS transition:
   *   1 → fully visible
   *   0 → invisible (fade-out in progress or complete)
   */
  const [opacity, setOpacity] = useState(0);

  /**
   * `mounted` controls whether the element is in the DOM at all.
   * We keep it mounted during the fade-out so the transition is visible,
   * then remove it once the transition completes.
   */
  const [mounted, setMounted] = useState(false);

  /** Refs to track pending timers so we can clear them on cleanup. */
  const fadeOutTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const unmountTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // -------------------------------------------------------------------------
  // Effect: react to `visible` prop changes
  // -------------------------------------------------------------------------

  useEffect(() => {
    // Clear any previously running timers when `visible` changes.
    if (fadeOutTimerRef.current !== null) {
      clearTimeout(fadeOutTimerRef.current);
      fadeOutTimerRef.current = null;
    }
    if (unmountTimerRef.current !== null) {
      clearTimeout(unmountTimerRef.current);
      unmountTimerRef.current = null;
    }

    if (visible) {
      // Mount the element first, then on the next paint ramp opacity to 1
      // so the browser has a chance to apply the transition.
      setMounted(true);
      // Use rAF to ensure the element is rendered before we trigger the transition.
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          setOpacity(1);
        });
      });

      // After DISPLAY_DURATION_MS, start the fade-out.
      fadeOutTimerRef.current = setTimeout(() => {
        setOpacity(0);

        // After the fade-out transition completes, unmount and notify parent.
        unmountTimerRef.current = setTimeout(() => {
          setMounted(false);
          onDismiss();
        }, FADE_OUT_DURATION_MS);
      }, DISPLAY_DURATION_MS);
    } else {
      // Parent hid the toast externally — fade out immediately.
      setOpacity(0);
      unmountTimerRef.current = setTimeout(() => {
        setMounted(false);
      }, FADE_OUT_DURATION_MS);
    }

    // Cleanup on unmount.
    return () => {
      if (fadeOutTimerRef.current !== null) clearTimeout(fadeOutTimerRef.current);
      if (unmountTimerRef.current !== null) clearTimeout(unmountTimerRef.current);
    };
    // `onDismiss` is intentionally excluded from deps — callers typically pass
    // an inline function and we don't want to restart the timer on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible]);

  // -------------------------------------------------------------------------
  // Render
  // -------------------------------------------------------------------------

  if (!mounted) return null;

  return (
    <div
      style={{
        ...toastStyle,
        opacity,
        transition: `opacity ${FADE_OUT_DURATION_MS}ms ease-in-out`,
      }}
      role="status"
      aria-live="polite"
      aria-atomic="true"
    >
      {/* Toast card — yellow warning styling per IACT-009 / SCR-012 */}
      <div
        style={{
          backgroundColor: 'rgb(254 252 232)', // bg-yellow-50
          borderWidth: '1px',
          borderStyle: 'solid',
          borderColor: 'rgb(253 224 71)',       // border-yellow-300
          color: 'rgb(133 77 14)',              // text-yellow-800
          boxShadow: '0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1)', // shadow-lg
          borderRadius: '0.5rem',              // rounded-lg
          padding: '0.75rem 1rem',             // px-4 py-3
          fontSize: '0.875rem',                // text-sm
          maxWidth: '24rem',                   // max-w-sm
          display: 'flex',
          alignItems: 'flex-start',
          gap: '0.5rem',
        }}
      >
        {/* Warning icon */}
        <span
          aria-hidden="true"
          style={{ flexShrink: 0, fontSize: '1rem', lineHeight: '1.25rem' }}
        >
          ⚠
        </span>

        {/* Message */}
        <div style={{ flex: 1 }}>
          <p style={{ margin: 0 }}>
            Credentials saved to{' '}
            <code
              style={{
                fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                fontSize: '0.8125rem',
                backgroundColor: 'rgb(254 240 138)', // bg-yellow-200-ish
                borderRadius: '0.25rem',
                padding: '0 0.25rem',
              }}
            >
              ~/.paulette/connections.json
            </code>{' '}
            (user-readable only). No keychain integration in this version.
          </p>
        </div>

        {/* Manual dismiss button */}
        <button
          type="button"
          onClick={() => {
            // Trigger immediate fade-out by setting opacity to 0.
            setOpacity(0);
            if (fadeOutTimerRef.current !== null) {
              clearTimeout(fadeOutTimerRef.current);
              fadeOutTimerRef.current = null;
            }
            unmountTimerRef.current = setTimeout(() => {
              setMounted(false);
              onDismiss();
            }, FADE_OUT_DURATION_MS);
          }}
          aria-label="Dismiss warning"
          style={{
            flexShrink: 0,
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            color: 'rgb(133 77 14)', // text-yellow-800
            fontSize: '1rem',
            lineHeight: 1,
            padding: '0 0 0 0.25rem',
            opacity: 0.7,
          }}
        >
          ×
        </button>
      </div>
    </div>
  );
}
