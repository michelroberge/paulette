/**
 * ConnectionsTab — list of all defined LLM provider connections with CRUD actions.
 *
 * Implemented in task 5.2. This stub file exists so that ConfigurePage.tsx (task 5.1)
 * compiles while the full component is being developed.
 *
 * Journey: JRN-v0.2.0-001 through JRN-v0.2.0-005
 * Architecture: ARCH-v0.2.0-023, SCR-008
 */

import type { CSSProperties } from 'react';

export interface ConnectionsTabProps {
  /**
   * Optional connection ID to highlight on mount (from the ?highlight= query param).
   * Used for deep-linking from inline connection error banners (IACT-011).
   */
  highlight?: string;
  /**
   * Called by ConnectionsTab once the highlight pulse animation has fired so that
   * ConfigurePage can remove the ?highlight= query param from the URL.
   * This prevents the card from re-pulsing every time the user returns to this tab.
   */
  clearHighlight?: () => void;
}

export function ConnectionsTab({ highlight: _highlight, clearHighlight: _clearHighlight }: ConnectionsTabProps) {
  const placeholderStyle: CSSProperties = {
    padding: '4rem 0',
    textAlign: 'center',
    color: '#64748b',
    fontSize: '0.9375rem',
  };

  return (
    <div style={placeholderStyle}>
      <p>Connections tab — coming in task 5.2.</p>
    </div>
  );
}
