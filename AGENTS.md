# KaraXXX — Private Adult Video Browser

## Overview

Go + SQLite + React/Tailwind v4 + shadcn/ui web app that crawls 6 adult video sources,
stores metadata/URLs, and serves a clean playback UI with user accounts and favorites.
Zero ads, zero JS tracking, direct MP4 streams.

## Architecture

```
karaXXX (Go binary, multi-file package main)
  ├── main.go      — routes, xnxx scraper, SSE, auth, DB, categories
  ├── tnaflix.go   — TNAFlix scraper (JSON-LD VideoObject)
  ├── drtuber.go   — DrTuber scraper (applicationData + og meta)
  └── web/dist/    — React SPA (embedded via http.FileServer)
  └── SQLite DB (karaxxx.db)
      ├── videos table (id, slug, title, description, duration, views, mp4 URLs, hls, preview, source)
      ├── users table (id, username, password_hash)
      ├── favorites table (user_id, video_id)
      ├── fav_categories table (user_id, category)
      ├── crawl_seeds table (seed, type)
      ├── scrape_failures table (id, error, attempts, last_attempt)
      └── background detail-scrape for unscraped/expired videos
```

## Providers (6 sources, threaded crawling)

| # | Provider | Source Key | ID Format | Metadata |
|---|----------|-----------|-----------|----------|
| 1 | **XNXX** | `xnxx` | hash (e.g. `abc123d`) | JSON-LD + player JS |
| 2 | **xVideos** | `xvideos` | alphanumeric (e.g. `iopkmuaedc3`) | JSON-LD + `setVideoUrl*` JS |
| 3 | **xHamster** | `xhamster` | short hash (7-8 chars) | `window.initials` JSON |
| 4 | **EPorner** | `eporner` | hash (~10 chars) | Structured HTML + meta desc |
| 5 | **TNAFlix** | `tnaflix` | numeric (e.g. `3086656`) | JSON-LD VideoObject |
| 6 | **DrTuber** | `drtuber` | numeric (e.g. `8620574`) | `applicationData` + og meta |

**Playable sources** (server-side URL extraction): xnxx, xvideos, xhamster
**Metadata-only** (JS-delivered video, no server-side MP4/HLS): eporner, drtuber, tnaflix

**Crawl All** (`/api/crawl`) fires all 6 providers as parallel goroutines.
Each provider has its own rate limiter (per-domain, ~400ms between requests to same host),
so crawling 6 providers simultaneously yields ~6x throughput without hitting rate limits.

## Per-Provider Rate Limiters

Each provider has a dedicated `chan time.Time` rate limiter:
- `rateLimiter` — XNXX (global, also used by background detail scraper)
- `rateLimitXv` — xVideos
- `rateLimitXh` — xHamster
- `rateLimitEp` — EPorner
- `rateLimitTf` — TNAFlix
- `rateLimitDt` — DrTuber

All 6 can run simultaneously at full speed without blocking each other.

## Branding

- **Name**: KaraXXX
- **Logo**: "Kara" in red (#e50914) + "XXX" in orange (#f97316)
- **Colors**: `--color-bg: #22222a`, `--color-card: #2a2a35`, `--color-text: #fff`, `--color-red: #e50914`, `--color-orange: #f97316`
- **Scale**: `html { font-size: 120% }` — 20% larger than default
- **Favicon**: SVG play-triangle (orange) with K-cutout on dark gradient background
- **SEO**: `robots.txt: Disallow: /`, `<meta name="robots" content="noindex, nofollow, ...">`,
  `X-Robots-Tag: noindex, nofollow, noarchive, nosnippet, noimageindex` on every response

## XNXX Reverse-Engineered API

| Endpoint | Data |
|----------|------|
| `xnxx.com/search/best` | 50-100 video links (`/video-{id}/{slug}`) |
| `xnxx.com/video-{id}/{slug}` | JSON-LD: name, duration, views, contentUrl |
| `mp4-{cdn}.xnxx-cdn.com/{uuid}/{n}/video_{q}p.mp4` or `/{n}/mp4_{sd\|hq\|hd\|fhd}.mp4` `?secure={token},{expiry}` | Direct MP4 — **each quality has its OWN token**; filename format changed mid-2026 to `mp4_{label}.mp4`, path digit varies |
| `hls-cdn77.xnxx-cdn.com/{token},{expiry}/{uuid}/0/hls.m3u8` | HLS stream (own token) |
| `thumb-cdn77.xnxx-cdn.com/{uuid}/0/preview.mp4` | 10s preview (used for hover) |

Tokens are **per-quality**. Real URLs in player JS — `setVideoUrlLow`, `setVideoUrlHigh`, `setVideoHLS`.
Each carries a different token; swapping filename → 403. CDN host varies (`mp4-cdn77`, `mp4-gcore`, …).
Expires ~hours.

## xHamster Reverse-Engineered API

| Endpoint | Data |
|----------|------|
| `xhamster.com/newest`, `/best/weekly`, `/channels` | `window.initials` JSON with video list |
| `xhamster.com/videos/{slug}-{shortId}` | `window.initials` JSON: title, duration, tags, uploader |
| `video{N}.xhcdn.com/key=.../{res}p.h264.mp4` | Direct MP4 with token in URL path |
| `ic-vt-nss.xhcdn.com/a/...` | Thumbnails (webp/jpg) |
| `<link rel="preload" href="...m3u8">` | HLS manifest with `key=TOKEN,end=TIMESTAMP` |

Token: `key=TOKEN,end=TIMESTAMP` in URL path. IP binding: `data=IP-dvp`. No JSON-LD.

## EPorner Reverse-Engineered API

| Endpoint | Data |
|----------|------|
| `eporner.com/`, `/2/`, `/category/{cat}/` | Server-rendered HTML with `data-id`, structured classes |
| `eporner.com/video-{HASH}/{slug}/` | `og:title`, meta description, category/pornstar links |
| `static-eu-cdn.eporner.com/thumbs/static4/{d}/{2d}/{3d}/{id}/{frame}_240.jpg` | Thumbnails |
| `<span class="mbtim">` | Duration (MM:SS) |
| `<span class="mbvie">` | Views (formatted like "1,059,110") |
| `<span class="mb-uploader">` | Uploader name |
| `<span class="mvhdico">` | Quality (4K, 1080p, 720p) |
| `<span class="mbrate">` | Rating (%) |

## TNAFlix Reverse-Engineered API

| Endpoint | Data |
|----------|------|
| `tnaflix.com/`, `/popular`, `/hd-videos`, `/top-rated` | Server-rendered HTML, paginated |
| `tnaflix.com/{category}/{slug}/video{NUMERIC_ID}` | Full JSON-LD VideoObject |
| `<script type="application/ld+json">` | name, description, uploadDate, duration (ISO), embedUrl, thumbnailUrl |
| `<a class="video-thumb" data-trailer="...">` | Trailer/preview MP4 URL with secure token |
| `<img data-src="...">` in `video-thumb` | Thumbnail (lazyloaded) |
| `<div class="video-duration">` | Duration (MM:SS) |
| `<a href="/pornstar/...">` | Pornstar name |
| `<a data-category="...">` | Categories |

JSON-LD duration format: `PT00H11M15S` — parsed to seconds.

## DrTuber Reverse-Engineered API

| Endpoint | Data |
|----------|------|
| `drtuber.com/`, `/most-popular`, `/top-rated` | Server-rendered HTML, paginated |
| `drtuber.com/video/{NUMERIC_ID}/{slug}` | `og:title`, `og:image`, meta description, view counts |
| `<a class="th ch-video" href="/video/{id}/{slug}">` | Video link with embedded `<img src>` and `alt` |
| `g{N}.drtst.com/media/videos/tmb/{id}/240_180/{frame}.jpg` | Thumbnail CDN (load-balanced across g1-g5) |
| `<a href="/tags/...">` | Tag links |
| `<a href="/categories/...">` | Category links |

## Database Schema

```sql
CREATE TABLE videos (
    id TEXT PRIMARY KEY, slug TEXT, title TEXT, description TEXT,
    categories TEXT, tags TEXT, uploader TEXT, upload_date TEXT,
    duration INTEGER, views INTEGER, added_at TEXT, source TEXT DEFAULT 'xnxx',
    thumb_uuid TEXT, url_360 TEXT, url_720 TEXT, url_1080 TEXT,
    preview_url TEXT, hls_url TEXT, secure_token TEXT, expires_at INTEGER,
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE favorites (
    user_id INTEGER NOT NULL, video_id TEXT NOT NULL,
    created_at TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, video_id)
);

CREATE TABLE fav_categories (
    user_id INTEGER NOT NULL, category TEXT NOT NULL,
    created_at TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, category)
);
```

## Categorization

~50 categories via two-tier matching:
1. **Tag-based** (`tagToCat` map): maps known tags to canonical categories (e.g. `"blowjob"` → `blowjob`, `"shemale"` → `transgender`)
2. **Keyword-based** (`categoryKeywords`): matches title + description against keyword lists with priority scoring

Categories include: anal, teen, milf, blowjob, big-tits, big-ass, homemade, creampie, cumshot, compilation, pov, lesbian, bbc, latina, asian, indian, rough, group, outdoor, handjob, squirting, cartoon, bdsm, fetish, cosplay, massage, vintage, european, transgender, casting, hidden-cam, tattooed, hairy, toy, shower, party, uniform, fantasy, parody, solo, doggystyle, cowgirl, sixty-nine, strip, pornstar, reality, and more.

## Deployment

```bash
# Build Go binary (requires sqlite_fts5 build tag for FTS search)
cd ~/.hermes/workspaces/karaxxx
go build -tags "sqlite_fts5" -buildvcs=false -o karaxxx .

# Build React frontend
cd web && bun run build && cd ..

# Full deploy (build Go + web, push to server, restart service)
./deploy.sh deploy

# Service config
systemd: /etc/systemd/system/karaxxx.service
nginx:  /etc/nginx/sites-available/adult.olibuijr.com
URL:    https://adult.olibuijr.com
DB:     /opt/karaxxx/karaxxx.db (SQLite)
Port:   8799 (internal, behind nginx reverse proxy)
Server: root@192.168.8.4

# Auto-crawl for new videos (every 3h — all providers)
0 */3 * * * curl -s http://127.0.0.1:8799/api/crawl > /dev/null

# Individual provider crawls (every 3h, staggered)
15 */3 * * * curl -s http://127.0.0.1:8799/api/crawl-xh > /dev/null
30 */3 * * * curl -s http://127.0.0.1:8799/api/crawl-ep > /dev/null
45 */3 * * * curl -s http://127.0.0.1:8799/api/crawl-tf > /dev/null
0  */3 * * * curl -s http://127.0.0.1:8799/api/crawl-dt > /dev/null

# Local DB auto-sync (every 5 min) — copies live DB for local dev
systemctl --user enable --now karaxxx-db-sync.timer
```

## Endpoints

### Pages
| Path | Purpose |
|------|---------|
| `/` | Browse grid with hover previews, infinite scroll, sort/source filters |
| `/search?q=` | Full-text search from local DB |
| `/play/{id}` | Full player with quality selector + favorite button |
| `/favorites` | User's favorited videos (requires login) |
| `/status` | Real-time scraping dashboard with per-source counts, progress bars, crawl controls |

### API
| Path | Purpose |
|------|---------|
| `/api/browse` | Paginated video list (supports `?sort=`, `?cat=`, `?q=`, `?uploader=`, `?source=`) |
| `/api/video/{id}` | Single video detail with MP4/HLS URLs |
| `/api/crawl` | Trigger ALL crawlers (xnxx + xvideos + xhamster + eporner + tnaflix + drtuber) in parallel |
| `/api/crawl-xv` | Trigger xVideos crawl |
| `/api/crawl-xh` | Trigger xHamster crawl |
| `/api/crawl-ep` | Trigger EPorner crawl |
| `/api/crawl-tf` | Trigger TNAFlix crawl |
| `/api/crawl-dt` | Trigger DrTuber crawl |
| `/api/categories` | List all categories |
| `/api/search?q=` | Scrape xnxx search with query |
| `/api/refresh?id=` | Re-scrape a specific video for fresh tokens |
| `/api/status` | SSE stream of crawl progress (includes `source_counts`, `total_count`, progress fields) |
| `/api/auth/register` | POST {username, password} → {token, user} |
| `/api/auth/login` | POST {username, password} → {token, user} |
| `/api/auth/me` | GET current user from Bearer token |
| `/api/fav/video/{id}` | GET/POST/DELETE — check/toggle favorite |
| `/api/fav/videos` | GET — list favorited video IDs |
| `/api/fav/category?cat=` | POST/DELETE — toggle favorite category |
| `/api/fav/categories` | GET — list favorited categories |
| `/vid/{id}/{quality}` | Proxy xnxx MP4 streams |
| `/thumb/{uuid}/...` | Proxy xnxx thumbnail CDN |
| `/media?url=` | Generic URL proxy (xhamster/eporner/tnaflix/drtuber thumbnails) |

## SSE Status Format

```json
{
  "status": "scraping|searching|idle",
  "source": "xnxx|xhamster|eporner|tnaflix|drtuber",
  "scanned": 1234,
  "new_videos": 56,
  "cached": 1178,
  "detail_done": 45,
  "detail_total": 7000,
  "page": 3,
  "total_count": 38200,
  "source_counts": {
    "xnxx": 37337,
    "xvideos": 296,
    "eporner": 240,
    "tnaflix": 235,
    "drtuber": 201,
    "xhamster": 50
  }
}
```

## Frontend

- **Stack**: React 19 + TypeScript 6 + Tailwind v4 + shadcn/ui
- **Routing**: react-router-dom v7 (SPA with `/`, `/search`, `/play/:id`, `/favorites`, `/status`)
- **State**: AuthContext with JWT in localStorage
- **Preview**: Hover on desktop (mouseenter/mouseleave), scroll-based autoplay on mobile
- **Infinite scroll**: IntersectionObserver with 600px root margin for preloading
- **Quality**: 360p/720p/1080p from direct MP4 CDN URLs
- **Sorting**: Recent (added_at), New (upload_date), Popular (views), Longest (duration)
- **Source filter**: All / XNXX / xVideos / xHamster / EPorner / TNAFlix / DrTuber
- **Categories**: Sidebar with pin-to-top favorites for logged-in users
- **Auth**: Login/Register dialog (shadcn Dialog), user dropdown with avatar
- **Favorites**: Heart button on every card and play page, `/favorites` page
- **Source badge**: Colored badge on cards indicating source (red XNXX, orange XH, blue EP, green TF, purple DT)
- **Relative dates**: "Today", "Yesterday", "Xd ago" from upload_date
- **Header**: Clickable video count + real-time progress → links to `/status` dashboard
- **Status page**: Live SSE progress with per-source counts, progress bars, crawl controls (All/individual)
- **No external scripts, no ads, no tracking**

## Key Implementation Details

### JWT Auth
- HMAC-SHA256 signing with random 32-byte server secret (generated on startup)
- Token format: `base64(header).base64(claims).base64(sig)`
- Claims: `{"uid":N,"un":"username","exp":unix_timestamp}`
- Parsed via `json.Unmarshal` (NOT fmt.Sscanf — `%[^"]` silently fails)
- 30-day token expiry

### Rate Limiting
- `rateLimitInterval = 400ms` between HTTP requests
- **Per-provider rate limiters** — 6 independent channels so all crawlers run simultaneously
- 3 retries with exponential backoff (5s → 10s → 20s)
- Max 5 concurrent scrape workers per provider (`scrapeWorkers = 5`)
- Per-provider lock files prevent duplicate crawls (`/tmp/karaxxx-{source}-crawl.lock`)

### CDN Proxy
- Detects `xhcdn.com`/`xnxx-cdn.com`/`eporner.com` domain and sets correct Referer header
- `/media?url=` generic proxy for all non-xnxx thumbnails
- `/thumb/` proxy for xnxx thumbnails (UUID-based path)
- Supports Range requests for video seeking

### Scraping Details

**XNXX**: `scrapeVideoDetail` parses JSON-LD + `setVideoUrl*` player JS. goquery for listing.
**xHamster**: `scrapeXhListing` parses regex from window.initials. `scrapeXhVideoDetail` extracts thumbURL, HLS, MP4 links.
**EPorner**: `scrapeEpListing` regex parses server-rendered HTML blocks. `scrapeEpVideoDetail` extracts og:title, meta desc, categories, pornstars.
**TNAFlix**: `scrapeTfListing` regex with DOTALL flag parses video-thumb blocks. `scrapeTfVideoDetail` JSON-unmarshals JSON-LD VideoObject for structured metadata.
**DrTuber**: `scrapeDtListing` regex parses video link blocks. `scrapeDtVideoDetail` extracts og:image, meta desc, tags, categories, view counts.

### Anti-Bot / No-Index
- `robots.txt`: `Disallow: /`
- Meta: `<meta name="robots" content="noindex, nofollow, noarchive, nosnippet, noimageindex">`
- HTTP header: `X-Robots-Tag: noindex, nofollow, noarchive, nosnippet, noimageindex` (middleware on every response)
