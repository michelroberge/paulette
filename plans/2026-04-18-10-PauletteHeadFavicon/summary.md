# Summary — Paulette Head Favicon

## What changed
- `frontend/public/favicon.svg` replaced with a custom SVG of the paulette mascot's head (heart antenna, rounded-rect face, crescent eyes, triangle nose, smile) on a dark rounded-square background using the app's brand palette (`#7dd3fc`, `#a78bfa`, `#f472b6`).

## Why
Previous favicon was a generic Vite lightning-bolt mark; project needed an on-brand tab icon matching the ASCII mascot shown on the home/login screen.

## Loading path
- Dev: Vite serves `public/favicon.svg` at `/favicon.svg`.
- Prod: Dockerfile copies `frontend/dist/` into `backend/static/`; Go `//go:embed all:static` ships it; server serves the root.
- `frontend/index.html:5` already has `<link rel="icon" type="image/svg+xml" href="/favicon.svg">` — no HTML changes required.

## Verification
- SVG parses as valid XML (Python `xml.etree` parse).
- No code or markup changes outside the single SVG file.

## Decisions
- DEC-001: Single SVG only (no PNG/ICO fallback).
- DEC-002: Head-only rendering (drop necklace/body) for 16×16 legibility.
