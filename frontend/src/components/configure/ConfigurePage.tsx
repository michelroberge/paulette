import { useNavigate, useSearchParams } from 'react-router-dom';

/**
 * Configure Paulette page — full implementation delivered in task 5.1.
 * This placeholder establishes the route and parses ?tab= / ?highlight= query params
 * so that deep-links from connection error banners resolve correctly.
 */
export function ConfigurePage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  // Query params used by deep-linking (IACT-011):
  // ?tab=connections|defaults  — selects which tab to show on load
  // ?highlight=<connection-id> — highlights a specific connection card
  const _tab = searchParams.get('tab') ?? 'connections';
  const _highlight = searchParams.get('highlight');

  // Suppress unused-variable lint warnings in the placeholder; these will be
  // consumed by the real implementation in task 5.1.
  void _tab;
  void _highlight;

  return (
    <div style={{ minHeight: '100vh', background: '#0f172a', color: '#e2e8f0', padding: '2rem' }}>
      <div style={{ maxWidth: '56rem', margin: '0 auto' }}>
        {/* Breadcrumb */}
        <button
          onClick={() => navigate('/')}
          style={{
            background: 'none',
            border: 'none',
            color: '#60a5fa',
            cursor: 'pointer',
            fontSize: '0.875rem',
            padding: 0,
            marginBottom: '1.5rem',
            display: 'flex',
            alignItems: 'center',
            gap: '0.25rem',
          }}
        >
          ← Home
        </button>

        <h1 style={{ fontSize: '1.5rem', fontWeight: 700, marginBottom: '0.5rem' }}>
          Configure Paulette
        </h1>
        <p style={{ color: '#94a3b8', fontSize: '0.875rem' }}>
          LLM provider connections and stage defaults — full configuration UI coming soon.
        </p>
      </div>
    </div>
  );
}
