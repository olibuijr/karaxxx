# KaraXXX — Private Adult Video Browser

Zero-ads, zero-tracking, self-hosted adult video aggregator. Crawls 5 providers (XNXX, xHamster, EPorner, TNAFlix, DrTuber), stores metadata in SQLite, and serves a clean React SPA with direct MP4 streaming.

## Stack

- **Backend**: Go 1.23+ (stdlib, goquery, sqlite3 via FTS5)
- **Frontend**: React 19 + TypeScript 6 + Tailwind v4 + shadcn/ui
- **Database**: SQLite with FTS5 full-text search
- **Deploy**: systemd + nginx reverse proxy

## Features

- **5 providers** crawled in parallel with per-domain rate limiters
- **Full-text search** via SQLite FTS5 with prefix matching
- **User accounts** with JWT auth (persisted across restarts)
- **Favorites, playlists, ratings** (thumbs up/down)
- **Watch history** with continue-watching and position resume
- **Watch later** queue
- **Personalized "For You" feed** based on watch history, favorites, and ratings
- **Tag cloud** with frequency-sized pills
- **Related videos** by shared categories
- **Trending sort** (views per day)
- **Random video** button
- **Grid density** toggle (compact/comfortable/large)
- **Theater mode** with auto-hide controls
- **Mobile gesture controls** (swipe seek, double-tap, volume swipe)
- **Keyboard shortcuts** (Space, arrows, F, M, 1-3, ?)
- **Continuous autoplay** with cancel countdown
- **Video hover previews** with thumbnail scrubbing
- **Loading skeletons**
- **Real-time crawl status** dashboard via SSE
- **Health endpoint** with DB size, stale tokens, goroutine count
- **Security headers** (CSP, HSTS, X-Frame-Options, etc.)
- **Login rate limiting** (5 attempts per 15 min per IP)
- **DB maintenance** (auto VACUUM, reindex, integrity check every 6h)

## Quick Start

```bash
# Prerequisites: Go 1.23+, Bun

# Clone
git clone https://github.com/olibuijr/karaxxx.git
cd karaxxx

# Build backend
go build -tags "sqlite_fts5" -buildvcs=false -o karaxxx .

# Build frontend
cd web && bun install && bun run build && cd ..

# Run
./karaxxx
# → http://localhost:8799
```

## API Endpoints

### Browse & Search
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/browse?page=&sort=&cat=&source=&uploader=&q=` | Paginated video list |
| GET | `/api/video/{id}` | Single video detail |
| GET | `/api/related/{id}?limit=12` | Related videos |
| GET | `/api/random?source=&cat=` | Random video ID |
| GET | `/api/search?q=` | Trigger XNXX search |
| GET | `/api/categories` | Top 30 categories |
| GET | `/api/tags?limit=100` | Tag frequency cloud |

### Crawling
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/crawl` | Trigger all 5 providers (parallel) |
| GET | `/api/crawl-xh` | xHamster only |
| GET | `/api/crawl-ep` | EPorner only |
| GET | `/api/crawl-tf` | TNAFlix only |
| GET | `/api/crawl-dt` | DrTuber only |
| GET | `/api/status` | SSE real-time progress stream |
| GET | `/api/refresh?id=` | Re-scrape single video for fresh tokens |

### Auth
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/auth/register` | Create account |
| POST | `/api/auth/login` | Login (rate-limited) |
| GET | `/api/auth/me` | Verify token |

### Watch History
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/watch/{id}` | Save watch position |
| GET | `/api/watch/history?limit=` | Get watch history |
| DELETE | `/api/watch/{id}` | Remove from history |

### Favorites
| Method | Path | Description |
|--------|------|-------------|
| GET/POST/DELETE | `/api/fav/video/{id}` | Toggle favorite |
| GET | `/api/fav/videos` | List favorited IDs |
| POST/DELETE | `/api/fav/category?cat=` | Pin/unpin category |
| GET | `/api/fav/categories` | Pinned categories |

### Playlists
| Method | Path | Description |
|--------|------|-------------|
| GET/POST | `/api/playlists` | List / create |
| GET/PUT/DELETE | `/api/playlists/{id}` | Get / rename / delete |
| POST | `/api/playlists/{id}/videos` | Add video |
| DELETE | `/api/playlists/{id}/videos/{vid}` | Remove video |
| PUT | `/api/playlists/{id}/reorder` | Reorder videos |

### Ratings
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/rate/{id}` | Rate (-1/0/1) |
| GET | `/api/rate/{id}` | Get user rating + aggregates |

### Watch Later
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/watch-later` | List queue |
| POST | `/api/watch-later/{id}` | Add to queue |
| DELETE | `/api/watch-later/{id}` | Remove from queue |

### Recommendations
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/for-you` | Personalized feed |
| GET | `/api/suggestions` | Per-category suggestions |
| GET | `/api/profile` | User stats and activity |

### System
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | DB size, stale tokens, goroutines, uptime |

## Deployment

```bash
# Build
go build -tags "sqlite_fts5" -buildvcs=false -ldflags="-s -w" -o karaxxx .
cd web && bun run build && cd ..

# Deploy to server (configurable in deploy.sh)
./deploy.sh deploy [version]
```

## License

MIT
