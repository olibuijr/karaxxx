# EPIC-1: Watch History & Continue Watching

**Tier**: P0 | **Effort**: Medium | **Dependencies**: None

## Overview
Track every video a user watches with playback position, surface "Continue Watching" on the homepage, and enable resuming videos at the last position.

## Schema

```sql
CREATE TABLE watch_history (
    user_id INTEGER NOT NULL,
    video_id TEXT NOT NULL,
    position INTEGER DEFAULT 0,        -- seconds watched
    duration INTEGER DEFAULT 0,         -- total video duration at time of save
    watched_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, video_id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (video_id) REFERENCES videos(id)
);
CREATE INDEX idx_watch_history_user ON watch_history(user_id, updated_at DESC);
CREATE INDEX idx_watch_history_video ON watch_history(video_id);
```

## Subtasks

### 1.1 Backend — Watch History Table & Migration
**File**: `main.go` (initDB section)
- [ ] Add `CREATE TABLE IF NOT EXISTS watch_history` to `initDB()`
- [ ] Add indexes as above
- [ ] Add `watch_history` to the schema documentation in AGENTS.md
- [ ] Verify table exists after server start: `sqlite3 karaxxx.db ".schema watch_history"`

### 1.2 Backend — Save Watch Position API
**Endpoint**: `POST /api/watch/{videoId}` — body: `{"position": 123}` (seconds)
- [ ] Create `handleWatchSave(w, r)` handler
- [ ] Parse video ID from URL path
- [ ] Validate JWT from Authorization header (reuse auth middleware pattern)
- [ ] Parse `position` from JSON body
- [ ] `INSERT OR REPLACE INTO watch_history (user_id, video_id, position) VALUES (?, ?, ?)`
- [ ] Also store `duration` if the body includes it
- [ ] Return `{"ok": true}`
- [ ] Register route in `initRoutes()` and `routeAPI` switch (both SPA and non-SPA paths)

### 1.3 Backend — Get Watch History API
**Endpoint**: `GET /api/watch/history?limit=20`
- [ ] Create `handleWatchHistory(w, r)` handler
- [ ] Validate JWT, extract user_id
- [ ] Query: `SELECT v.*, wh.position, wh.updated_at FROM watch_history wh JOIN videos v ON wh.video_id = v.id WHERE wh.user_id = ? ORDER BY wh.updated_at DESC LIMIT ?`
- [ ] Return JSON array of videos with `watched_position` and `watched_at` fields
- [ ] Register route

### 1.4 Backend — Delete Watch History Entry
**Endpoint**: `DELETE /api/watch/{videoId}`
- [ ] Create `handleWatchDelete(w, r)` handler
- [ ] Validate JWT
- [ ] `DELETE FROM watch_history WHERE user_id = ? AND video_id = ?`
- [ ] Return `{"ok": true}`
- [ ] Register route

### 1.5 Frontend — Position Tracking in Player
**File**: `web/src/pages/Play.tsx`
- [ ] Add `useEffect` with 5-second interval: if video is playing, POST current position to `/api/watch/{videoId}`
- [ ] Also save on pause, seek, and beforeunload (use `navigator.sendBeacon` for beforeunload)
- [ ] Read `watch_history.position` from the initial video API response (if available) and seek to that position on load
- [ ] Show a "Resume from 12:34?" toast/banner if position > 5 seconds (use the existing sonner toast from `sonner.tsx`)

### 1.6 Frontend — Continue Watching Row
**File**: `web/src/pages/Browse.tsx`
- [ ] Add a "Continue Watching" section at the top of the browse grid (above the main grid)
- [ ] Fetch from `GET /api/watch/history?limit=8` when user is logged in
- [ ] Display as a horizontal scrollable row of video cards
- [ ] Each card shows: thumbnail, title, source badge, progress bar overlay (% watched)
- [ ] Clicking navigates to `/play/{videoId}` (resume from position handled in 1.5)
- [ ] Add "X" button on each card to remove from history (calls DELETE endpoint)
- [ ] Only show section if there are history entries and user is logged in

### 1.7 Frontend — Progress Bar on Video Cards
**File**: `web/src/components/VideoCard.tsx`
- [ ] If `video.watched_position` and `video.duration` exist, render a thin progress bar at the bottom of the thumbnail area
- [ ] Bar color: `bg-orange`, height: 3px, width: `(position/duration)*100%`
- [ ] Only applies to videos in watch history / continue watching context
- [ ] Should NOT show on regular browse grid cards (only in continue watching row)

### 1.8 Backend — Progress in Browse Response
**File**: `main.go` (handleAPIBrowse)
- [ ] When user is authenticated (JWT present in query or header), LEFT JOIN watch_history in the browse query
- [ ] Add `COALESCE(wh.position, 0) as watched_position` to SELECT
- [ ] Add to the Video struct or response JSON
- [ ] Update `Video` struct to include `WatchedPosition int` field if needed

## Verification
1. Watch a video for 30 seconds, close the tab, reopen site → see it in "Continue Watching"
2. Click a continue-watching card → player seeks to saved position
3. Delete from history → card disappears
4. Watch another video → history reorders with most recent first
