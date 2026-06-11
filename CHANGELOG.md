# Changelog

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
