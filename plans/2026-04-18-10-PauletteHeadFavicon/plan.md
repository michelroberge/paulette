# Paulette Head Favicon

## Goal
Replace the current generic (Vite-style) favicon with a custom SVG that visually represents the "paulette head" mascot from the ASCII art in `frontend/src/components/project/ProjectList.tsx`.

## Reference (ASCII mascot)
```
        ♥
       ╱│╲
    ┌──────────┐
    │  ◠    ◠  │
    │    ▽     │
    │  ╰────╯  │
    └─────┬────┘
```

## Steps
1. Replace `frontend/public/favicon.svg` with an SVG rendering of the paulette head:
   - Rounded rectangular head body
   - Heart antenna on top
   - Two crescent eyes
   - Small triangle nose
   - Curved smile
   - Short neck stub at bottom
   - Use brand colors: cyan `#7dd3fc`, purple accent `#a78bfa`, pink heart `#f472b6`, on a dark background for contrast at small sizes
2. Verify `frontend/index.html` already has `<link rel="icon" type="image/svg+xml" href="/favicon.svg">`.
3. Confirm the file is served statically by Vite from `/public`.

## Acceptance
- Favicon renders cleanly at 16x16 and 32x32 (browser tab + bookmarks).
- Recognizable as the paulette mascot head.
- No changes required to the index.html link (reuse the existing entry).
