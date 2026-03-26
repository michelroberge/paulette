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
}

export function ConnectionsTab({ highlight: _highlight }: ConnectionsTabProps) {
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
