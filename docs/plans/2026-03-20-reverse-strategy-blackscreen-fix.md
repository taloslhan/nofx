# Reverse Strategy "Black Overlay" Bug Fix Plan

## Date: 2026-03-20
## Status: Fixed

## Symptom
Clicking the enable toggle in reverse strategy section causes a visual "black overlay/mask" on part of the page. Not a crash — a portion turns visually darker.

## Root Cause

Three factors combine to produce the visual artifact:

### 1. Global CSS `transition: all` (Primary)
`web/src/index.css:443-451`:
```css
input, select, textarea {
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}
```
Also `button { transition: all 0.2s; }` at line 360.

When toggle is clicked, 4 sub-controls switch from disabled to enabled. `transition: all` animates ALL CSS properties (including opacity, background, border-width, etc.) during this state change, creating a perceptible dark flash on the extreme dark theme.

### 2. DeepVoidBackground CRT Overlay Positioning
`web/src/components/common/DeepVoidBackground.tsx:30`:
```tsx
<div className="absolute inset-0 pointer-events-none fixed z-[9999] opacity-40">
```
- Conflicting `absolute` + `fixed` (fixed wins)
- z-[9999] above all content, opacity-40 amplifies dark flash beneath

### 3. Extreme Dark Theme
All backgrounds nearly indistinguishable: `#05070A`, `#0B0E11`, `#0E1217`, `rgba(14,18,23,0.6)`. Transition artifacts become very visible.

## Fix Plan

### Fix A: Restrict transition properties (Recommended)
`web/src/index.css`:
```css
/* Before */
input, select, textarea {
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}
button {
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}

/* After */
input, select, textarea {
  transition: border-color 0.2s cubic-bezier(0.4, 0, 0.2, 1),
              box-shadow 0.2s cubic-bezier(0.4, 0, 0.2, 1),
              background-color 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}
button {
  transition: background-color 0.2s, color 0.2s, border-color 0.2s,
              box-shadow 0.2s, transform 0.2s, filter 0.2s, opacity 0.2s;
}
```

### Fix B: Clean CRT overlay positioning
`web/src/components/common/DeepVoidBackground.tsx:30`:
```tsx
// Before
<div className="absolute inset-0 pointer-events-none fixed z-[9999] opacity-40">

// After
<div className="fixed inset-0 pointer-events-none z-[9999] opacity-40">
```

## Files
- `web/src/index.css` — global CSS transitions
- `web/src/components/common/DeepVoidBackground.tsx` — CRT overlay
- `web/src/components/strategy/ReverseStrategyEditor.tsx` — toggle component
- `web/src/pages/StrategyStudioPage.tsx` — parent page
