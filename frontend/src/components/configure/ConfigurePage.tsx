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

import { useState, useEffect, CSSProperties } from 'react';
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

function TabButton({ label, active, onClick }: TabButtonProps) {
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
    <button style={tabStyle} onClick={onClick} type="button">
      {label}
    </button>
  );
}

// ---------------------------------------------------------------------------
// ConfigurePage
// ---------------------------------------------------------------------------

export function ConfigurePage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  // Parse ?tab= query param to determine the initial active tab.
  // Falls back to 'connections' for any unknown value.
  const tabParam = searchParams.get('tab');
  const highlightParam = searchParams.get('highlight') ?? undefined;

  const resolveTab = (param: string | null): TabId =>
    param === 'defaults' ? 'defaults' : 'connections';

  const [activeTab, setActiveTab] = useState<TabId>(() => resolveTab(tabParam));

  // Keep the active tab in sync when the URL query param changes (e.g. the user
  // navigates back/forward or a deep-link replaces the URL).
  useEffect(() => {
    setActiveTab(resolveTab(tabParam));
  }, [tabParam]);

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
            onClick={() => setActiveTab('connections')}
          />
          <TabButton
            id="defaults"
            label="Stage Defaults"
            active={activeTab === 'defaults'}
            onClick={() => setActiveTab('defaults')}
          />
        </div>

        {/* Tab content */}
        <div style={styles.tabContent}>
          {activeTab === 'connections' && (
            <ConnectionsTab
              // Pass the ?highlight= param so ConnectionsTab can pulse the matching card
              // (IACT-011 deep-link behaviour from connection error banners).
              highlight={highlightParam}
            />
          )}
          {activeTab === 'defaults' && <StageDefaultsTab />}
        </div>
      </div>
    </div>
  );
}
