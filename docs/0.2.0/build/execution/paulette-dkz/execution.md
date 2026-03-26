The routing is already set up. Let me check the `ProjectList` component to see if the "Configure Paulette" button is there:The existing code already has:
1. `ProjectListPage` passing `onConfigure={() => navigate('/configure')}` to `ProjectList`
2. A gear icon in the banner header for configure

However, the task requires a "Configure Paulette" button **alongside** the "New Project" and "Import Project" action cards in the project grid. Currently there's only a cog icon in the banner — the button should be a proper card alongside the others. Let me add it:Now let me verify the change looks right and check if there's any CSS for `configure-card` or if we need to rely on the existing `new-project` styles:The existing `.project-card.new-project` style already provides the right look (dashed border, centered flex column layout). The "Configure Paulette" card will inherit those styles. No additional CSS is needed since the `configure-card` class is optional — the card works fine with just `new-project`.

Let me verify the final state of the relevant section in the file:The implementation is complete. Here's what was done:

**Changes made to `frontend/src/components/project/ProjectList.tsx`:**

Added a "Configure Paulette" card in the project grid, rendered conditionally when `onConfigure` is provided. It appears directly after "New Project" and "Import Repo" action cards and:

- Uses the same `project-card new-project` CSS classes as the other action cards (dashed border, centered flex column layout)
- Displays a gear/cog SVG icon (same icon already used in the banner cog button) alongside the "Configure Paulette" label
- Calls `onConfigure` on click, which in `App.tsx` navigates to `/configure`
- Is conditionally rendered (`{onConfigure && (...)}`) so it only shows when the prop is provided — preserving backward compatibility

The existing wiring in `App.tsx` (`ProjectListPage` → `onConfigure={() => navigate('/configure')}`) already handles the navigation correctly, so no changes were needed there.