# Changelog

## [0.6.10] — 2026-06-19

### Changed
- Fix xVideos thumbnail proxy allowlist (xvideos-cdn.com) so cards no longer 403.
- Fix XNXX HTTP thumbnail URLs in search suggestions/sidebar by proxying concrete CDN URLs and using mozaique_listing for UUID rows.

## [0.6.9] — 2026-06-19

### Changed
- Fix KVS playback media proxy allow-list for HeavyFetish, PunishBang, and SunPorno assets.
- Stop stale metadata-only failures from persisting in retry queue and clear them on successful skip.
- Prune permanent xnxx.gold teaser failures immediately and clean old stale failure rows on startup.
- Trim KVS 404 log noise so pagination misses do not dump HTML into journald.

## [0.6.8] — 2026-06-19

### Changed
- Clean up stale KVS crawl lock on restart so BDSM source crawls cannot get wedged after deploy restarts.

## [0.6.7] — 2026-06-19

### Changed
- Remove dead PunishBang BDSM category seed that returned 404; keep working videos and bondage seeds.

## [0.6.6] — 2026-06-19

### Changed
- Add KVS-style BDSM playable sources: HeavyFetish, PunishBang, and SunPorno BDSM.
- Wire new sources into automated crawl loop, backfill refresh, status UI, source filters, and badges.
- Add /api/crawl-kvs endpoint and live-smoke verified MP4 extraction from all three sources.

## [0.6.5] — 2026-06-19

### Changed
- Use highest available video quality consistently and resolve quality buttons to real streams.
- Serve higher-resolution/correct thumbnail assets for XNXX/xVideos instead of hard-coded low/wrong frames.
- Fix mobile density controls: all four sizes now produce distinct grids, with Compact starting at 2 columns.

## [0.6.4] — 2026-06-19

### Fixed
- **CrawlProgress lock-copy bug**: `getProgressJSON()` was copying `CrawlProgress` struct (which contains `sync.RWMutex`) — undefined behavior under concurrent access. Fixed by using a mutex-free `progressSnapshot` struct for JSON encoding. (go vet clean)
- **storeVideo overwriting fields with empty values**: `ON CONFLICT DO UPDATE` was blindly overwriting slug, thumb_uuid, url_360/720/1080, preview_url, hls_url, secure_token, and expires_at with whatever the detail scraper returned — even if empty. This cleared slugs and URLs on re-scrape. Fixed with `CASE WHEN excluded.X != '' THEN excluded.X ELSE videos.X END` for all nullable fields.
- **xhamster detail scraper losing slug**: `scrapeXhVideoDetail()` didn't look up the slug from DB, so `storeVideo()` overwrote the stub's slug with empty string. Fixed by querying the slug from DB (same pattern as xVideos scraper).
- **Stale crawl lock files after restart**: `/tmp/karaxxx-*-crawl.lock` files survived ungraceful exits (kill, crash, deploy), causing crawls to be silently skipped on the next process. Added `cleanupStaleLocks()` at startup that removes all 6 lock files.
- **Frontend sendBeacon Content-Type**: `navigator.sendBeacon()` with a plain string sends `text/plain` — the server's JSON handler couldn't parse it. Watch position was lost on page unload. Fixed by wrapping in `Blob` with `type: 'application/json'`.
- **Frontend missing xVideos in Status dashboard**: Source stats grid, crawl trigger buttons, and `triggerAll()` were missing xVideos. Added xVideos to all three.
- **Frontend missing xVideos in SearchDropdown**: `formatSourceLabel()` didn't handle 'xvideos'. Added.
- **Dead code removed**: `reXhVideoList` regex was defined but never used (superseded by inline regex in `scrapeXhListing`).

### Changed
- xHamster crawl completion log now includes diagnostic counters (listing found, filtered empty ID/Slug, filtered existing) for monitoring.

## [0.6.3] — 2026-06-19

### Changed
- Release deployed through deploy.sh.

## [0.6.2] — 2026-06-19

### Changed
- Release deployed through deploy.sh.

## [0.6.1] — 2026-06-19

### Changed
- Release deployed through deploy.sh.

## [0.6.0] — 2026-06-19

### Changed
- Release deployed through deploy.sh.

## [0.5.0] — 2026-06-19

### Production-Grade UI Overhaul

- **Design System**: OLED cinema theme with true black (`#000000`), deep midnight cards (`#0d0d1a`), rose red accent (`#e11d48`), ambient crimson glow background
- **Custom Video Player**: HTML5 controls replaced with custom overlay — progress bar with glow thumb, play/pause/volume/fullscreen, quality selector, theater mode with auto-hide
- **Keyboard Shortcuts**: Full HUD — Space (play/pause), arrows (seek/volume), F (fullscreen), T (theater), M (mute), J/L (seek -10s/+10s), ? (shortcuts)
- **VideoCard**: Play button overlay on hover, scale-in animation, gradient source badges, polished category pills
- **Browse**: Refined filter chips with X icon, proper empty/error state components with SVG illustrations, animated loading spinner
- **Login/Setup**: Password show/hide toggle, spinner loading states, glass-morphism auth card with specular edge, value prop section
- **CSS**: Custom scrollbars, selection colors, `prefers-reduced-motion` support, view transitions animation, toast overrides
- **Typography**: Inter body + Barlow Condensed display, font-feature-settings for OpenType

## [0.4.2] — 2026-06-19

### Changed
- **Parallel crawl loop**: all 5 providers now launch concurrently via WaitGroup instead of running sequentially
- **Per-page progress logging** added to XNXX, EPorner, TNAFlix, and DrTuber crawls (was silent before)

## [0.4.1] — 2026-06-19

### Added
- **Automated crawl service** (`crawlLoop` goroutine): runs all 5 providers sequentially every 6 hours
  - Initial 60s sleep on startup, then cycles xnxx → xhamster → eporner → tnaflix → drtuber
  - Respects existing per-provider crawl locks, no overlap
- **Traffic camo & browser simulation** (safe scraping)
  - Expanded user agent pool to 10 modern browsers (Chrome 124-127, Firefox 127 across Win/Mac/Linux)
  - Random user agent rotation per request
  - Best-effort `<source>`, `contentUrl`, `embedUrl` extraction in all detail scrapers
- **Smart source filtering**: `isPlayableSource()` prevents infinite retry loops for non-playable sources (eporner, drtuber, tnaflix). Their metadata stays in DB but no URL extraction is retried.
- **xHamster crawl seeds expanded**: 7 seeds (up from 4), 10 pages per seed (up from 5)

### Changed
- `ensureFreshVideo()` skips non-playable sources for token refresh
- `scrapeNewVideoDetails()` clears failures and skips non-playable sources
- User agents updated to latest major browser versions

## [0.4.0] — 2026-06-19

### Changed
- Release deployed through deploy.sh.

## [0.3.23] — 2026-06-17

### Changed
- Release deployed through deploy.sh.

## [0.3.22] — 2026-06-17

### Changed
- Always pick best available quality in video proxy: fallback order changed from 360→720→1080 to 1080→720→360, so videos without lower-quality variants stream at their best available resolution instead of worst quality.
- Frontend: fix removeFromHistory using wrong API endpoint (/api/watch vs /api/watch/history)
- Frontend: fix Playlists context menu closing only on last playlist row
- Frontend: fix Wall page infinite loading when logged out
- Frontend: add ErrorBoundary to all routes preventing blank-screen crashes
- Frontend: fix fetchRandom/fetchPlaylists/fetchProfile missing error guards
- Frontend: remove unused Pagination component and useKeyboardShortcuts hook
- Frontend: watched_position properly typed (no more as-any casts)
- Frontend: sourceBadge moved to module scope (perf: no per-render alloc)

## [0.3.20] — 2026-06-11

### Changed
- Fix multi-category filtering on the main browse API (cat=a,b now correctly returns the intersection)

## [0.3.19] — 2026-06-11

### Changed
- Multi-category filtering with active-filter pills; favorites sort; watch-history clear; filter persistence
- Structured request-ID logging; better empty/loading/error states; mobile + a11y polish
- CI pipeline added (build + race-tested)

## [0.3.18] — 2026-06-11

### Changed
- Security hardening: removed secret-leaking debug endpoint, SSRF/path-traversal guards, JWT cookie auth + env secret, bcrypt passwords, register rate-limiting, FK cascades, FTS bounds
- New: full-text search dropdown (categories + videos + open-all), category filter in browse toolbar
- Perf: count cache, connection-pool tuning, graceful shutdown
- 16 Go tests added

## [0.3.17] — 2026-06-11

### Changed
- Category system overhaul: fixed the 'only two categories filter' bug — all scraped categories now preserved, tags unified as categories, exact-match junction-table filtering, full metadata indexing
- P0 security/correctness fixes from 10-agent audit

## [0.3.16] — 2026-06-11

### Changed
- Sexy satin login background (draped silk folds + warm bokeh) replacing smoke effect
- Removed logo hover wobble
- Sitewide glass UI: header, sidebar, video cards, dialogs, filters, pagination

## [0.3.15] — 2026-06-11

### Changed
- Liquid-glass login: three.js shader background, frosted glass auth card, View Transitions navigation, optimistic session restore + browse prefetch
- Hero logo no longer overlaps tagline

## [0.3.14] — 2026-06-11

### Changed
- Mobile logo moved to header; sort+source as side-by-side dropdowns; fixed continue watching X button persistence

## [0.3.13] — 2026-06-11

### Changed
- Surprise me visible on mobile too

## [0.3.12] — 2026-06-11

### Changed
- Random+profile pushed right, Surprise me label

## [0.3.11] — 2026-06-11

### Changed
- Fixed mobile header logo visibility

## [0.3.10] — 2026-06-11

### Changed
- Moved search into mobile sidebar; cleaned up mobile header

## [0.3.9] — 2026-06-11

### Changed
- Flattened header logo 3D effect

## [0.3.8] — 2026-06-11

### Changed
- Redesigned login/setup page with hero layout, benefit bullets, collapsible privacy details

## [0.3.7] — 2026-06-11

### Changed
- Categories expanded, density options, sidebar collapsible, full-width header

## [0.3.6] — 2026-06-11

### Changed
- Show all categories collapsible with localStorage; Adult Playground in sidebar; more category mappings in backend; compact density smaller; theatre mode added; header full width

## [0.3.5] — 2026-06-11

### Changed
- Added missing sources to sidebar; collapsed source pills into dropdown on mobile; shared SOURCES constant

## [0.3.4] — 2026-06-11

### Changed
- Replaced sort/source filters with styled dropdowns; pin button now visible on mobile; category list no longer clipped

## [0.3.3] — 2026-06-11

### Changed
- Treat empty SSE progress as idle so the header never shows scraping 0/0 after restart
- Reset background backfill progress counters atomically and return to idle when it finishes
- Guard the React header and status page against zero-total scraping states

## [0.3.2] — 2026-06-11

### Changed
- Use SQLite WAL with a real connection pool instead of serializing all DB work through one connection
- Make /api/health a fast readiness endpoint with bounded optional DB metrics
- Cap background missing-media backfill to 12 videos every 30 minutes and delay it until after startup
- Slow failed-scrape retry churn to reduce service load

## [0.3.1] — 2026-06-11

### Changed
- Fix startup DB profile migration blocking before the HTTP listener opened
- Require deploy.sh to wait for systemd active plus /api/health readiness
- Keep service lifecycle routed through systemd from deploy.sh

## [0.3.0] — 2026-06-11

### Changed
- KaraXXX - Adult Playground invite-only setup screen with privacy and GitHub transparency copy
- Hashed invite-key CLI for controlled signup
- Provider-aware stale-token refresh and missing-media cleanup
- Anonymous social layer with comments, reactions, watch counts, profiles, and walls
- User-facing changelog page backed by deploy.sh release metadata
- Hardened deploy.sh systemd validation, stop, restart, and status checks

## [0.2.0] — 2026-06-09

### Added
- **React + Vite + TypeScript + Tailwind v4 frontend** (`web/`) replacing server-rendered Go templates
  - `VideoCard` — responsive card with hover zoom, gradient overlay, pill tags
  - `Header` — glass-morphism sticky bar with search, live crawl progress
  - `CategoryBar` — auto-fetching category pills with sort toggles
  - `Pagination` — page navigation with ellipsis
  - `Browse` page — auto-fill grid, IntersectionObserver preview on desktop, scroll-based on mobile
  - `Play` page — video player, quality selector, tags, uploader link
- JSON API endpoints on Go backend:
  - `GET /api/browse` — paginated video list with sort, category, search, uploader filters
  - `GET /api/video/{id}` — full video detail with auto-refresh on expired tokens
- Go server detects `web/dist/` and serves React SPA at root (falls back to templates otherwise)
- `deploy.sh` — build, push, restart, version management, status
- `CHANGELOG.md` + `VERSION` file

### Changed
- Frontend migrated from Go html/template to React TSX with Tailwind v4
- CDN media proxy (`/vid/`, `/thumb/`) preserved for both frontend modes
- SSE crawl progress wired through React state

## [0.1.0] — 2026-06-09

### Added
- Rotating User-Agent pool (7 UAs: Chrome 124-126, Firefox 126 on Linux/Mac/Win)
- Cookie jar (`net/http/cookiejar`) on scrape HTTP client for session persistence
- Exponential backoff in `httpGetWithRetry`: 5s → 10s → 20s with up to 1s jitter
- Token-bucket rate limiter: 5-request burst, then 1 req / 400ms to xnxx.com
- Dead-letter queue: `scrape_failures` table + `retryFailedLoop` (every 5min) with doubling backoff (5min → 6h max)
- `recordScrapeFailure` / `clearScrapeFailure` wired into all batch scrape error/success paths

### Changed
- Retry logic: was 3 attempts with linear 1s/2s backoff; now exponential with jitter
- Rate limiting: was unbounded 5 concurrent workers; now globally throttled at HTTP level

### Fixed
- Dead scrape failures were fire-and-forget; now persisted and retried
- Single hardcoded UA fingerprint risk; now rotated per request
