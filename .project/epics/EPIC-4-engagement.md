# EPIC-4: User Engagement Core

**Tier**: P1 | **Effort**: High | **Dependencies**: EPIC-1 (watch history)

## Overview
Add playlists, thumbs up/down ratings, watch later queue, user profile with stats. These features require new DB tables and multiple API endpoints. Build order matters — each feature builds on the auth system and existing DB.

## Schema Additions

```sql
CREATE TABLE playlists (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    is_public INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE playlist_videos (
    playlist_id INTEGER NOT NULL,
    video_id TEXT NOT NULL,
    position INTEGER DEFAULT 0,       -- ordering within playlist
    added_at TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (playlist_id, video_id),
    FOREIGN KEY (playlist_id) REFERENCES playlists(id),
    FOREIGN KEY (video_id) REFERENCES videos(id)
);

CREATE TABLE ratings (
    user_id INTEGER NOT NULL,
    video_id TEXT NOT NULL,
    rating INTEGER NOT NULL CHECK (rating IN (-1, 1)),  -- -1 = thumbs down, 1 = thumbs up
    created_at TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, video_id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (video_id) REFERENCES videos(id)
);

CREATE TABLE watch_later (
    user_id INTEGER NOT NULL,
    video_id TEXT NOT NULL,
    position INTEGER DEFAULT 0,
    added_at TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, video_id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (video_id) REFERENCES videos(id)
);
```

## Subtasks

### 4.1 Playlists — Backend CRUD
**Files**: `main.go`
- [ ] Add playlist tables to `initDB()` (schema above)
- [ ] `POST /api/playlists` — body: `{"name": "My Playlist"}` → create playlist, return `{"id": N}`
- [ ] `GET /api/playlists` — return user's playlists: `[{"id": 1, "name": "...", "video_count": 5, "created_at": "..."}]`
- [ ] `PUT /api/playlists/{id}` — body: `{"name": "New Name"}` → rename playlist
- [ ] `DELETE /api/playlists/{id}` — delete playlist and all its video entries
- [ ] `POST /api/playlists/{id}/videos` — body: `{"video_id": "abc"}` → add video to playlist
- [ ] `GET /api/playlists/{id}/videos` — return videos in playlist with ordering
- [ ] `DELETE /api/playlists/{id}/videos/{videoId}` — remove video from playlist
- [ ] `PUT /api/playlists/{id}/reorder` — body: `{"video_ids": ["c", "a", "b"]}` → update positions
- [ ] Validate JWT for all endpoints, verify playlist ownership
- [ ] Register all routes in both SPA and non-SPA paths

### 4.2 Playlists — Frontend
**Files**: New page + modifications to existing components
- [ ] Create `web/src/pages/Playlists.tsx`:
  - [ ] List view showing user's playlists with name, video count, created date
  - [ ] "Create Playlist" button with inline name input
  - [ ] Rename/delete actions on each playlist (three-dot menu)
  - [ ] Click playlist → expand to show video grid
- [ ] Create `web/src/components/PlaylistButton.tsx`:
  - [ ] "+" button on every VideoCard and Play page
  - [ ] Clicking opens a dropdown with "Add to playlist..." listing user's playlists
  - [ ] Shows checkmark if video already in that playlist
  - [ ] "Create new playlist" option at bottom
- [ ] Add `/playlists` route to App.tsx
- [ ] Add "Playlists" link in user dropdown menu (next to Favorites) and/or sidebar

### 4.3 Thumbs Up/Down Ratings — Backend
**Files**: `main.go`
- [ ] Add ratings table to `initDB()` (schema above)
- [ ] `POST /api/rate/{videoId}` — body: `{"rating": 1}` or `{"rating": -1}`:
  - [ ] `INSERT OR REPLACE INTO ratings (user_id, video_id, rating) VALUES (?, ?, ?)`
  - [ ] If rating is 0 (or toggle), DELETE the row (remove rating)
  - [ ] Return `{"rating": 1, "up_count": 42, "down_count": 3}`
- [ ] `GET /api/rate/{videoId}` — return user's rating for this video + aggregate counts:
  - [ ] `SELECT rating FROM ratings WHERE user_id = ? AND video_id = ?` (for user's rating)
  - [ ] `SELECT SUM(CASE WHEN rating = 1 THEN 1 ELSE 0 END) as up, SUM(CASE WHEN rating = -1 THEN 1 ELSE 0 END) as down FROM ratings WHERE video_id = ?` (for aggregates)
- [ ] Add `sort=rating` to browse API (highest up/down ratio, minimum 5 votes)
- [ ] Register routes

### 4.4 Thumbs Up/Down — Frontend
**Files**: `web/src/components/VideoCard.tsx`, `web/src/pages/Play.tsx`
- [ ] Create reusable `<RatingButtons videoId={id} />` component:
  - [ ] Thumbs up icon + count, thumbs down icon + count
  - [ ] Active state: filled icon when user has voted
  - [ ] Clicking toggles: if already voted up, clicking up again removes vote; clicking down changes vote
  - [ ] Optimistic UI update: change state immediately, then confirm with API response
  - [ ] Greyed out / hidden for non-logged-in users
- [ ] Add to VideoCard: compact version (just icons + counts) in card footer area
- [ ] Add to Play page: larger version below video title

### 4.5 Watch Later Queue
**Files**: `main.go`, `web/src/components/VideoCard.tsx`, `web/src/pages/Browse.tsx`
- [ ] Backend:
  - [ ] Add watch_later table to `initDB()` (schema above)
  - [ ] `POST /api/watch-later/{videoId}` → add to queue, return `{"ok": true}`
  - [ ] `DELETE /api/watch-later/{videoId}` → remove from queue
  - [ ] `GET /api/watch-later?limit=100` → return queued videos with ordering
  - [ ] Register routes
- [ ] Frontend:
  - [ ] Add clock/queue icon button on every VideoCard ("Watch Later" on hover)
  - [ ] Add badge count in header nav showing queue size (only if > 0)
  - [ ] Add "Watch Later" page at `/watch-later` — grid of queued videos
  - [ ] Remove button (X) on each card
  - [ ] "Play All" button at top of page → navigates to first video, player auto-advances through queue

## Verification
1. Create a playlist, add 3 videos → playlist page shows them in order
2. Thumbs up a video → icon fills, count increments, persists across page reloads
3. Thumbs down another video → icon fills, both counts visible
4. Add video to watch later → header badge shows count, `/watch-later` shows the video
5. Remove from watch later → badge decrements
