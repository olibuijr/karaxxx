# EPIC-5: UI/UX Polish

**Tier**: P2 | **Effort**: Medium | **Dependencies**: None (wraps everything)

## Overview
Keyboard shortcuts, theater mode, grid density, loading skeletons, mobile gestures, thumbnail scrubbing, continuous auto-play. These are pure frontend features requiring no backend changes.

## Subtasks

### 5.1 Keyboard Shortcuts
**File**: New file `web/src/hooks/useKeyboardShortcuts.ts`
- [ ] Create a `useKeyboardShortcuts` hook:
  ```typescript
  // Browse page shortcuts (when not in player):
  // Arrow keys → navigate video grid (focus ring + scroll into view)
  // Enter → open focused video
  
  // Player shortcuts (when video is playing/paused):
  // Space → toggle play/pause
  // F → toggle fullscreen  
  // ←/→ → seek ±10 seconds
  // ↑/↓ → adjust volume ±10%
  // 1/2/3 → switch quality (360p/720p/1080p)
  // Esc → exit player, navigate back to browse
  // M → mute/unmute
  ```
- [ ] Add keyboard shortcut overlay: press `?` to show a modal with all shortcuts listed
- [ ] Hook into Browse page and Play page
- [ ] Prevent default browser behavior for used keys (Space scrolls, etc.)
- [ ] Only active when no input/textarea is focused

### 5.2 Theater Mode
**File**: `web/src/pages/Play.tsx`
- [ ] Add "Theater Mode" toggle button in player controls (next to fullscreen, or a dedicated icon)
- [ ] When activated:
  - [ ] Fade sidebar to opacity-0 (or slide out)
  - [ ] Header collapses to minimal height with just logo and close button
  - [ ] All UI except video fades to near-black after 3 seconds of mouse inactivity
  - [ ] Minimal floating control bar: play/pause, seek bar, theater mode exit, fullscreen
  - [ ] Any mouse movement or tap restores full UI temporarily
- [ ] Toggle back restores original layout
- [ ] Persist preference in localStorage so theater mode stays between page loads
- [ ] Add smooth CSS transitions (300ms ease) for all fades/slides

### 5.3 Grid Density Toggle
**File**: `web/src/pages/Browse.tsx`, `web/src/App.css`
- [ ] Add density toggle button next to sort/source filters (3 icons: □ □□ □□□)
- [ ] Three modes persisted in localStorage:
  - **Compact**: 6 columns on desktop, 3 on mobile. Smaller cards, less padding. For speed-scanning.
  - **Comfortable** (default): 4 columns desktop, 2 mobile. Current default sizing.
  - **Large**: 2-3 columns desktop, 1 mobile. Bigger previews, more metadata visible. For careful browsing.
- [ ] Implement via CSS grid classes:
  ```css
  .grid-density-compact { grid-template-columns: repeat(6, 1fr) }
  .grid-density-comfortable { grid-template-columns: repeat(4, 1fr) }
  .grid-density-large { grid-template-columns: repeat(3, 1fr) }
  ```
- [ ] Card component should adapt: compact shows less metadata (just title + duration), large shows full metadata

### 5.4 Loading Skeletons
**Files**: `web/src/components/VideoCard.tsx`, `web/src/pages/Browse.tsx`
- [ ] Create `<VideoCardSkeleton />` component:
  - [ ] Animated placeholder matching card layout (thumbnail area, title lines, metadata)
  - [ ] Use `animate-pulse` Tailwind class with `bg-card-hover`
  - [ ] Stagger animation delays: each skeleton card gets `animation-delay: ${index * 50}ms`
- [ ] Replace the spinner/loading state in Browse with skeleton grid:
  - [ ] Initial load: show 8-12 skeleton cards
  - [ ] Infinite scroll load: show 4-8 skeleton cards below existing content
- [ ] Add skeleton variants for:
  - [ ] Browse grid (card skeleton)
  - [ ] Play page (video player skeleton + sidebar skeleton)
  - [ ] Status page (card skeleton for each section)

### 5.5 Mobile Gesture Controls
**File**: `web/src/pages/Play.tsx`
- [ ] Add touch gesture handling to the video player (iOS/Android):
  - [ ] **Double-tap left/right**: skip ±10 seconds
  - [ ] **Swipe left/right**: seek ±15 seconds (show seek indicator overlay)
  - [ ] **Swipe up/down on left half**: adjust brightness (show brightness overlay with %)
  - [ ] **Swipe up/down on right half**: adjust volume (show volume overlay with %)
  - [ ] **Pinch**: toggle fullscreen
- [ ] Show translucent overlay during gestures (e.g., brightness icon + percentage bar)
- [ ] Auto-hide overlays after 1 second of no touch
- [ ] Only activate on the `<video>` element or player container, not on page-level touches
- [ ] Use `touchstart`, `touchmove`, `touchend` events

### 5.6 Thumbnail Scrubbing (Hover Preview with Seek)
**File**: `web/src/components/VideoCard.tsx`
- [ ] Enhance the existing hover preview mechanism:
  - [ ] On `mouseenter`, load the preview MP4 (`video.preview_url`)
  - [ ] On `mousemove`, calculate horizontal position within the card (0% to 100%)
  - [ ] Set `videoElement.currentTime = position * videoElement.duration`
  - [ ] Show a subtle timeline indicator (thin line following cursor)
- [ ] Fallback: if no `preview_url` available, show nothing (just static thumbnail)
- [ ] Debounce `mousemove` to every 100ms for performance
- [ ] On `mouseleave`, pause and reset the preview video

### 5.7 Continuous Auto-Play with Undo
**File**: `web/src/pages/Play.tsx`, `web/src/pages/Browse.tsx`
- [ ] When a video ends (`video.onended`):
  - [ ] Show a 5-second countdown overlay: "Playing next video in 5...4..." with a "Cancel" button
  - [ ] "Cancel" stops auto-play for the session
  - [ ] When countdown reaches 0, navigate to the next video
- [ ] Next video determination:
  - [ ] If watching from Watch Later queue: next in queue
  - [ ] If watching from a playlist: next in playlist
  - [ ] If watching from browse/related: next video in the browse order (grid position + 1)
  - [ ] If from random or direct link: no auto-play
- [ ] Show a subtle "Up next: {title}" info in the countdown overlay
- [ ] Store `autoPlayDisabled` flag in sessionStorage

## Verification
1. Press `?` on browse page → keyboard shortcut overlay appears
2. Press Space on play page → video pauses/plays
3. Toggle theater mode → sidebar and header fade away
4. Switch to Compact density → 6 columns of smaller cards
5. On mobile, swipe left on player → video seeks +15s with overlay indicator
6. Hover over a video card with preview → move mouse horizontally → preview scrubs through frames
7. Let a video play to the end → 5-second countdown appears
