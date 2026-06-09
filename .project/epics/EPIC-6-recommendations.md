# EPIC-6: Smart Recommendations

**Tier**: P2 | **Effort**: Medium | **Dependencies**: EPIC-1 (watch history), EPIC-4 (ratings)

## Overview
Add a personalized "For You" feed, smart recommendations on play page, category-based suggestions, and a user profile page with stats. These features synthesize data from watch history, favorites, and ratings.

## Subtasks

### 6.1 "For You" Personalized Feed
**Files**: `main.go`, `web/src/pages/Browse.tsx`
- [ ] Backend — Add `GET /api/for-you?limit=50` endpoint:
  - [ ] Requires JWT auth
  - [ ] Algorithm (simple but effective):
    1. Get user's favorited categories from `fav_categories`
    2. Get user's top-watched categories from `watch_history` JOINed with videos
    3. Get user's upvoted video categories
    4. Merge categories with weights: favorites (3x), watch history (2x), upvotes (1x)
    5. Query: `SELECT v.* FROM videos v WHERE v.categories LIKE '%cat1%' OR v.categories LIKE '%cat2%' ... ORDER BY v.views DESC LIMIT 50`
    6. Exclude already-watched and favorited videos
  - [ ] If user has no data (new user), return trending videos (same as EPIC-3.2)
  - [ ] Return JSON with `videos` array and `reason` field per video ("Because you liked Big Tits", "Popular in Homemade", etc.)
- [ ] Frontend — Add "For You" tab at the top of browse page:
  - [ ] Tab bar: "For You" | "Browse" (default: Browse)
  - [ ] "For You" only visible when logged in
  - [ ] Fetches from `/api/for-you` instead of `/api/browse`
  - [ ] Shows "reason" as a small label on each card ("Because you liked...")
- [ ] Register route

### 6.2 Smart Related Videos (Enhanced)
**Files**: `main.go` (extends EPIC-3.1 endpoint)
- [ ] Enhance the `/api/related/{videoId}` endpoint:
  - [ ] When user is authenticated, personalize the related videos:
    1. First priority: same categories as current video, weighted by user's category preferences
    2. Second priority: popular in current video's categories
    3. Third priority: same source, popular
  - [ ] When user is not authenticated, use the basic algorithm from EPIC-3.1
- [ ] Add "mood" diversity: ensure related videos aren't all identical categories — mix in 1-2 from adjacent categories

### 6.3 Category-Based Suggestions
**Files**: `main.go`, `web/src/components/Sidebar.tsx`
- [ ] Backend — Add `GET /api/suggestions` endpoint:
  - [ ] For each of the user's favorited categories, return the top 3 most-viewed recent videos
  - [ ] Structure: `{"Homemade": [{video...}, ...], "Big Tits": [{video...}, ...], ...}`
- [ ] Frontend — In the sidebar, below favorited categories (which are pinned to top), show:
  - [ ] "Suggested for you" section with category headers
  - [ ] Each header is expandable, showing 3 video thumbnails
  - [ ] "View all in {category}" link at bottom
- [ ] Only visible when user is logged in and has favorite categories

### 6.4 User Profile Page with Stats
**Files**: `main.go`, New page `web/src/pages/Profile.tsx`
- [ ] Backend — Add `GET /api/profile` endpoint:
  - [ ] Requires JWT
  - [ ] Return:
    ```json
    {
      "username": "...",
      "account_age_days": 42,
      "total_watched": 156,
      "total_watch_time_seconds": 42840,
      "favorite_categories": ["homemade", "milf"],
      "top_categories": [{"name": "homemade", "count": 89}, {"name": "blowjob", "count": 34}],
      "playlist_count": 3,
      "favorite_count": 12,
      "ratings_given": 45,
      "rating_ratio": 0.82,
      "recently_watched": [video...],
      "top_rated": [video...]
    }
    ```
  - [ ] Queries:
    - `SELECT COUNT(*) FROM watch_history WHERE user_id = ?`
    - `SELECT SUM(position) FROM watch_history WHERE user_id = ?`
    - `SELECT COUNT(*) FROM playlists WHERE user_id = ?`
    - `SELECT COUNT(*) FROM favorites WHERE user_id = ?`
    - `SELECT COUNT(*), SUM(CASE WHEN rating=1 THEN 1 ELSE 0 END) ...`
    - Category breakdown from favorites + watch history
    - Last 5 watched videos
- [ ] Frontend — Create `/profile` page:
  - [ ] Stats cards in a grid: Total Watched, Watch Time, Playlists, Favorites
  - [ ] Category breakdown as a horizontal bar chart (simple div-based, no chart library)
  - [ ] "Recently Watched" horizontal scroll row (reuse VideoCard)
  - [ ] "Top Rated" horizontal scroll row
  - [ ] Account info: username, member since date
- [ ] Add `/profile` route to App.tsx
- [ ] Link from user dropdown menu: "Profile" item before Favorites
- [ ] Register route

## Verification
1. Log in, favorite a category → "For You" tab shows videos in that category
2. Watch several videos in one category → "For You" prioritizes that category
3. On play page, related videos are personalized (when logged in)
4. Sidebar shows category suggestions below pinned favorites
5. Profile page shows accurate stats matching user's activity
