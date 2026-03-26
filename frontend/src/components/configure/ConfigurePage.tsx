/**
 * Configure Paulette page — two-tab layout for managing LLM provider connections
 * and pipeline stage defaults.
 *
 * Tabs:
 *   - Connections: CRUD for named provider connections (ConnectionsTab)
 *   - Stage Defaults: per-stage connection + model assignment (StageDefaultsTab)
 *
 * Query param support:
 *   ?tab=connections|defaults   — selects the active tab on load
 *   ?highlight=<connection-id>  — highlights a specific connection card (deep-link from error banners)
 *
 * Journey: JRN-v0.2.0-001, JRN-v0.2.0-006
 * Architecture: ARCH-v0.2.0-022, SCR-007
 */

import type { CSSProperties } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { ConnectionsTab } from './ConnectionsTab';
import { StageDefaultsTab } from './StageDefaultsTab';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type TabId = 'connections' | 'defaults';

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

const styles = {
  page: {
    minHeight: '100vh',
    background: '#0f172a',
    color: '#e2e8f0',
  } as CSSProperties,

  inner: {
    maxWidth: '56rem', // max-w-4xl (~896px)
    margin: '0 auto',
    padding: '2rem 1.5rem',
  } as CSSProperties,

  breadcrumb: {
    display: 'flex',
    alignItems: 'center',
    gap: '0.25rem',
    background: 'none',
    border: 'none',
    color: '#60a5fa',
    cursor: 'pointer',
    fontSize: '0.875rem',
    padding: 0,
    marginBottom: '1.5rem',
    fontFamily: 'inherit',
  } as CSSProperties,

  breadcrumbArrow: {
    fontSize: '1rem',
    lineHeight: 1,
  } as CSSProperties,

  heading: {
    fontSize: '1.5rem',
    fontWeight: 700,
    color: '#f8fafc',
    marginBottom: '0.25rem',
  } as CSSProperties,

  subheading: {
    fontSize: '0.875rem',
    color: '#94a3b8',
    marginBottom: '1.75rem',
  } as CSSProperties,

  tabStrip: {
    display: 'flex',
    borderBottom: '1px solid #334155',
    marginBottom: '1.75rem',
  } as CSSProperties,

  tabContent: {
    // Tab content area — no additional wrapper styling needed; each tab manages its own layout
  } as CSSProperties,
};

// ---------------------------------------------------------------------------
// TabButton
// ---------------------------------------------------------------------------

interface TabButtonProps {
  id: TabId;
  label: string;
  active: boolean;
  onClick: () => void;
}

function TabButton({ id, label, active, onClick }: TabButtonProps) {
  const tabStyle: CSSProperties = {
    background: 'none',
    border: 'none',
    borderBottom: active ? '2px solid #3b82f6' : '2px solid transparent',
    color: active ? '#3b82f6' : '#94a3b8',
    cursor: 'pointer',
    fontFamily: 'inherit',
    fontSize: '0.9375rem',
    fontWeight: active ? 600 : 400,
    padding: '0.625rem 1rem',
    marginBottom: '-1px', // Overlap the strip border so the active underline sits flush
    transition: 'color 0.15s ease, border-color 0.15s ease',
  };

  return (
    <button
      style={tabStyle}
      onClick={onClick}
      type="button"
      role="tab"
      id={`tab-${id}`}
      aria-selected={active}
      aria-controls={`tabpanel-${id}`}
      tabIndex={active ? 0 : -1}
    >
      {label}
    </button>
  );
}

// ---------------------------------------------------------------------------
// ConfigurePage
// ---------------------------------------------------------------------------

export function ConfigurePage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  // Derive active tab directly from the URL — the URL is the single source of truth.
  // This ensures Back/Forward navigation and deep-links all reflect the real state
  // without any dual-state synchronisation logic.
  const tabParam = searchParams.get('tab');
  const highlightParam = searchParams.get('highlight') ?? undefined;
  const activeTab: TabId = tabParam === 'defaults' ? 'defaults' : 'connections';

  /**
   * Navigate to a tab by updating the URL.
   *
   * Rules:
   * - The `?tab=` param is always written so the URL stays canonical.
   * - The `?highlight=` param is intentionally dropped when switching tabs:
   *   once the user moves away from the Connections tab the deep-link highlight
   *   should not re-fire when they come back (IACT-011).
   * - Switching to the same tab the user is already on is a no-op so we avoid
   *   pushing redundant history entries.
   */
  const handleTabClick = (tab: TabId) => {
    if (tab === activeTab && !highlightParam) return; // already there, nothing to change
    setSearchParams({ tab }); // drops ?highlight= intentionally
  };

  return (
    <div style={styles.page}>
      <div style={styles.inner}>
        {/* Breadcrumb ← Home */}
        <button
          style={styles.breadcrumb}
          onClick={() => navigate('/')}
          type="button"
          aria-label="Back to home"
        >
          <span style={styles.breadcrumbArrow}>←</span>
          <span>Home</span>
        </button>

        {/* Page header */}
        <h1 style={styles.heading}>Configure Paulette</h1>
        <p style={styles.subheading}>
          Manage LLM provider connections and set which model to use at each pipeline stage.
        </p>

        {/* Two-tab strip */}
        <div style={styles.tabStrip} role="tablist" aria-label="Configuration sections">
          <TabButton
            id="connections"
            label="Connections"
            active={activeTab === 'connections'}
            onClick={() => handleTabClick('connections')}
          />
          <TabButton
            id="defaults"
            label="Stage Defaults"
            active={activeTab === 'defaults'}
            onClick={() => handleTabClick('defaults')}
          />
        </div>

        {/* Tab panels */}
        <div
          style={styles.tabContent}
          role="tabpanel"
          id={`tabpanel-${activeTab}`}
          aria-labelledby={`tab-${activeTab}`}
        >
          {activeTab === 'connections' && (
            <ConnectionsTab
              // Pass the ?highlight= param so ConnectionsTab can pulse the matching card
              // (IACT-011 deep-link behaviour from connection error banners).
              // ConnectionsTab is responsible for calling clearHighlight() once the
              // pulse animation fires so the param doesn't linger.
              highlight={highlightParam}
              clearHighlight={() => setSearchParams({ tab: 'connections' })}
            />
          )}
          {activeTab === 'defaults' && <StageDefaultsTab />}
        </div>
      </div>
    </div>
  );
}
