## Decision: Use SVG (only) for the favicon
ID: DEC-001

- Date: 2026-04-18
- Status: Accepted

- Context:
  The default `frontend/public/favicon.svg` was a generic lightning-bolt style Vite mark. The user asked for a favicon matching the "paulette head" — the ASCII mascot rendered on the home/login screen.

- Options Considered:
  - Option A: Ship a single SVG favicon, reuse existing `<link rel="icon" type="image/svg+xml">`.
  - Option B: Ship SVG + multi-size PNG/ICO set and multiple `<link>` tags.

- Decision:
  Option A. Replace the existing `favicon.svg` with a head-shaped SVG; keep the existing `<link>` in `index.html`.

- Rationale:
  All evergreen browsers (the app targets React 19 + modern stack) support SVG favicons. A single file avoids adding a PNG generation step to the Vite build and keeps the asset resolution-independent.

- Consequences:
  - Positive:
    - One file to maintain; crisp at any pixel density.
    - Zero changes to `index.html` or the Go static-embed pipeline.
  - Negative:
    - Legacy browsers without SVG favicon support will show no icon (acceptable for this app's audience).

- Impact:
  - Affects: `frontend/public/favicon.svg`
  - Related Plans: 2026-04-18-10-PauletteHeadFavicon

## Decision: Show just the head (omit beads/necklace/body)
ID: DEC-002

- Date: 2026-04-18
- Status: Accepted

- Context:
  The full ASCII mascot includes a heart antenna, head, necklace of coloured beads, and a body/legs. At 16×16 the whole mascot becomes illegible.

- Options Considered:
  - Option A: Render the full mascot (head + beads + body) scaled down.
  - Option B: Render only the heart antenna + head + short neck stub.

- Decision:
  Option B.

- Rationale:
  The user asked specifically for "the paulette head". Limiting the glyph to the head preserves recognizability at browser-tab sizes while still quoting the mascot's signature heart antenna and face features (crescent eyes, triangle nose, smile).

- Consequences:
  - Positive:
    - Clean silhouette at 16×16 and 32×32.
    - Stays on-brand with the mascot.
  - Negative:
    - Loses the colourful bead necklace that gives the full mascot its character.

- Impact:
  - Affects: `frontend/public/favicon.svg`
  - Related Plans: 2026-04-18-10-PauletteHeadFavicon
