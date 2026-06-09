# EPIC-3: Content Discovery

**Tier**: P1 | **Effort**: Medium | **Dependencies**: None (standalone)

## Overview
Add related videos on play page, trending/popular sort, pornstar/uploader pages, random video, and tag exploration. These features use existing DB data (categories, tags, uploader, views) and require no new tables.

## Subtasks

### 3.1 Related Videos on Play Page
**Files**: `main.go`, `web/src/pages/Play.tsx`
- [ ] Backend — Add `GET /api/related/{videoId}?limit=12` endpoint:
  - [ ] Fetch the current video's categories and tags
  - [ ] Query: `SELECT * FROM videos WHERE id != ? AND source = ? AND (categories LIKE ? OR categories LIKE ? ...) ORDER BY views DESC LIMIT ?`
  - [ ] Match by shared categories first (at least 1 overlap), then by shared tags
  - [ ] Fallback: same source, ordered by popularity if no category matches
  - [ ] Return JSON array of Video objects
- [ ] Register route
- [ ] Frontend — Below the player on play page, render a "Related Videos" horizontal scroll row
- [ ] Each card: compact version (thumbnail, title, duration, source badge)
- [ ] Lazy load: fetch only when user scrolls near the section

### 3.2 Trending / Popular This Week
**Files**: `main.go` (handleAPIBrowse), `web/src/pages/Browse.tsx`
- [ ] Backend — Add `sort=trending` to browse API:
  - [ ] Score formula: `views / MAX(1, julianday('now') - julianday(added_at))` — views per day
  - [ ] Query: `SELECT v.*, (CAST(v.views AS REAL) / MAX(1.0, julianday('now') - julianday(v.added_at))) AS trend_score FROM videos v WHERE ... ORDER BY trend_score DESC LIMIT ? OFFSET ?`
  - [ ] Respect existing filters (source, cat, uploader)
- [ ] Frontend — Add "Trending" to the sort filter bar:
  ```typescript
  { label: 'Trending', value: 'trending' }
  ```
  Insert after "Popular" in the sorts array (line ~134 in Browse.tsx)

### 3.3 Pornstar / Uploader Pages
**Files**: `main.go`, `web/src/pages/Browse.tsx`
- [ ] Backend — Add `?uploader=` support to browse API (may already partially work):
  - [ ] Query: `WHERE v.uploader = ?` (parameterized)
  - [ ] Return uploader name, video count, total views in response metadata
  - [ ] Add `/api/uploader/{name}` endpoint for direct access
- [ ] Frontend — Make uploader names clickable on video cards and play page:
  - [ ] In `VideoCard.tsx`: wrap uploader name in `<Link to={`/?uploader=${encodeURIComponent(video.uploader)}`}>`
  - [ ] In `Play.tsx`: same for uploader name display
- [ ] Frontend — Uploader page header: show uploader name, video count, total views
- [ ] Add uploader page route: not strictly needed (uses browse with `?uploader=` param)

### 3.4 Random Video Button
**Files**: `main.go`, `web/src/components/Header.tsx`
- [ ] Backend — Add `GET /api/random?source=&cat=` endpoint:
  - [ ] Build query: `SELECT id FROM videos` with optional WHERE clauses
  - [ ] `ORDER BY RANDOM() LIMIT 1` (SQLite, fast enough for 40K rows)
  - [ ] Return `{"id": "abc123"}` — redirect to play page
- [ ] Frontend — Add dice/random icon button in header:
  ```tsx
  <button onClick={async () => {
    const res = await fetch('/api/random')
    const { id } = await res.json()
    navigate(`/play/${id}`)
  }} title="Random video">
    <svg>...</svg> {/* dice or shuffle icon */}
  </button>
  ```
- [ ] Respect active source filter and category filter from URL params

### 3.5 Tag Cloud / Tag Exploration
**Files**: `main.go`, `web/src/components/Sidebar.tsx`
- [ ] Backend — Add `GET /api/tags?limit=100` endpoint:
  - [ ] Query distinct tags from videos table: parse comma-separated `tags` column
  - [ ] Can't easily do in SQL alone — query raw tags, split, count in Go:
    ```go
    rows, _ := db.Query("SELECT tags FROM videos WHERE tags != '' LIMIT 5000")
    tagCounts := map[string]int{}
    for rows.Next() { ... }
    // Return top N tags by frequency
    ```
  - [ ] Return `[{"name": "blowjob", "count": 8423}, ...]`
- [ ] Frontend — Add tag cloud section in sidebar (below categories or as collapsible section)
  - [ ] Render tags as clickable pills, sized by frequency (larger = more common)
  - [ ] Clicking a tag navigates to `/?q={tag}` (uses existing search)
  - [ ] Show top 30-50 tags

### 3.6 Video Card Source Badge for All Providers
**File**: `web/src/components/VideoCard.tsx`
- [ ] Already partially done — verify all 5 providers have badges:
  - XNXX: no badge (default, most content)
  - xHamster: orange "XH" ✓
  - EPorner: blue "EP" ✓
  - TNAFlix: green "TF" ✓
  - DrTuber: purple "DT" ✓
- [ ] Add badge also to the play page (Play.tsx) — currently no source badge on player

## Verification
1. Watch any video → see 6-12 related videos below player with matching categories
2. Select "Trending" sort → videos ordered by views/day
3. Click uploader name → see all their videos with count header
4. Click random button → immediately navigates to a random video
5. Scroll sidebar → see tag cloud with sized pills
