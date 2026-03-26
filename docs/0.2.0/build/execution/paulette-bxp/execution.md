Now I have a thorough understanding of the codebase. Let me create the `ProjectStageSettings.tsx` component:Now let me verify the file compiles by running a TypeScript check:No TypeScript errors. Let me also verify the component file itself is well-formed and review the key logic:Clean compile. Let me do a quick review of what was implemented:The component is complete and clean. Here's a summary of what was implemented:

## `ProjectStageSettings.tsx` — Complete

**File:** `frontend/src/components/configure/ProjectStageSettings.tsx`

### What it implements

**SCR-011 / ARCH-v0.2.0-026 / JRN-v0.2.0-007:**

1. **Slide-over panel** — Fixed to the right edge (`width: 420px`), dark-themed (`#1e293b`), with a semi-transparent backdrop that closes on click.

2. **Header** — Shows "Stage Connection Settings" + project name subtitle, with an × close button.

3. **Five stage rows** — One per pipeline stage (Vision → Complete), each with:
   - **Stage pip** — Color-coded dot (blue, purple, amber, green, teal)
   - **Stage label** — e.g. "UX Design"
   - **Save status** — Inline `Saved ✓` / `Saving…` / `Save failed` per row (auto-fades after 2s)
   - **Inherit toggle** — `role="switch"` button; blue when ON, grey when OFF; animated thumb slide

4. **Inherit ON** (default) — Row shows global default connection + model in muted italic text (read-only). No yellow tint.

5. **Inherit OFF** (override) — Row gets a yellow `rgba(234,179,8,0.07)` tint + amber border. Shows editable:
   - **Connection dropdown** — Lists all saved connections + "Claude CLI (default)" at top
   - **Model field** — Dropdown if the selected connection has `discoveredModels`; free-text input otherwise

6. **Auto-save logic** — IACT-010:
   - Toggle ON→OFF: immediately saves pre-populated global defaults as an override
   - Toggle OFF→ON: sends `null` to delete the override
   - Connection change: saves immediately (also updates model to `defaultModel`)
   - Model dropdown change: saves immediately
   - Model free-text: saves on blur only (not every keystroke)

7. **"Reset all to global defaults"** — Red underlined link in the footer (only shown when any override exists), triggers a confirmation dialog with the project name, Cancel/Reset All buttons.

8. **TypeScript** — Compiles with zero errors (`tsc --noEmit` clean).