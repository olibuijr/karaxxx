# EPIC-2: Infrastructure Hardening

**Tier**: P0 | **Effort**: Medium | **Dependencies**: None (do first)

## Overview
Fix existential risks (SQLite corruption, no backups, missing security headers), add observability, improve search, and enable HTTP caching.

## Subtasks

### 2.1 SQLite Maintenance & Health Endpoint
**File**: `main.go`
- [ ] Add `runDBMaintenance()` function called every 6 hours via goroutine:
  ```go
  db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
  db.Exec("PRAGMA optimize")
  db.Exec("REINDEX videos_fts")
  db.Exec("PRAGMA integrity_check")
  ```
- [ ] Create `GET /api/health` endpoint (`handleHealth`):
  - [ ] Query DB size: `SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()`
  - [ ] Query WAL size: check `karaxxx.db-wal` file size with `os.Stat`
  - [ ] Query video count per source (already done in getProgressJSON)
  - [ ] Query stale token count: `SELECT COUNT(*) FROM videos WHERE expires_at < unixepoch() AND expires_at > 0`
  - [ ] Query scrape failure count: `SELECT COUNT(*) FROM scrape_failures`
  - [ ] Return JSON: `{"db_size_bytes": N, "wal_size_bytes": N, "videos_by_source": {...}, "stale_tokens": N, "scrape_failures": N, "uptime_seconds": N, "goroutines": N}`
- [ ] Register route in both SPA and non-SPA paths
- [ ] Add `var startTime = time.Now()` for uptime tracking

### 2.2 Database Backup & Restore
**Files**: `deploy.sh`, new `scripts/backup.sh`
- [ ] Create `scripts/backup.sh`:
  ```bash
  #!/bin/bash
  ssh root@192.168.8.4 "sqlite3 /opt/karaxxx/karaxxx.db \".backup /opt/karaxxx/backups/karaxxx.db.$(date +%Y%m%d-%H%M)\""
  ssh root@192.168.8.4 "gzip /opt/karaxxx/backups/karaxxx.db.*"
  # Rotate: keep 7 daily, 4 weekly
  ```
- [ ] Add backup step to `deploy.sh` before restart: push binary → backup DB → restart
- [ ] Add `./deploy.sh backup` command
- [ ] Add cron instruction: `0 3 * * * /home/olafurbui/Projects/Karaxxx/scripts/backup.sh` (daily at 3 AM)
- [ ] Create `/opt/karaxxx/backups/` directory on server

### 2.3 Security Headers & Login Rate Limiting
**File**: `main.go`
- [ ] Add HTTP security headers to the `noindexMiddleware` (rename to `securityMiddleware`):
  ```
  Content-Security-Policy: default-src 'self'; media-src *; img-src *; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'
  X-Content-Type-Options: nosniff
  X-Frame-Options: DENY
  Referrer-Policy: no-referrer
  Strict-Transport-Security: max-age=31536000
  ```
- [ ] Add in-memory rate limiter for `/api/auth/login`:
  ```go
  var loginRateLimit = make(map[string]*rateLimiterEntry)  // IP → attempts
  // Max 5 attempts per 15 minutes per IP
  ```
- [ ] Return `429 Too Many Requests` with `Retry-After: 900` when exceeded
- [ ] Clean up expired entries every 5 minutes

### 2.4 Fix Browse SQL Injection Vector
**File**: `main.go` (handleAPIBrowse)
- [ ] Current code at ~line 1132 does `WHERE source = '` + param + `'` — replace with parameterized query
- [ ] Audit ALL SQL queries for string concatenation in WHERE/ORDER BY clauses
- [ ] Use `?` placeholders everywhere
- [ ] For ORDER BY (sort param), validate against whitelist BEFORE building query:
  ```go
  validSorts := map[string]string{"recent": "v.added_at DESC", "new": "v.upload_date DESC", "views": "v.views DESC", "duration": "v.duration DESC"}
  if orderBy, ok := validSorts[sort]; ok { ... }
  ```

### 2.5 Unified FTS5 Search with Filters
**File**: `main.go` (handleAPIBrowse / search)
- [ ] Merge the two separate search paths (FTS at ~line 1143, SQL at ~line 1170) into one query
- [ ] Add `?sort=`, `?source=`, `?cat=`, `?uploader=` filter support to FTS search
- [ ] Add `*` suffix to search terms for prefix matching: `SELECT * FROM videos_fts WHERE videos_fts MATCH ? || '*'`
- [ ] Fix pagination in FTS path — currently hardcodes `count = len(videos)` (max 100)
- [ ] Use `COUNT(*) OVER()` window function or a separate count query for proper `total_pages`
- [ ] Remove the `sanitizeFTSQuery` risk — use proper FTS5 query escaping

### 2.6 HTTP Caching for Thumbnails & Media
**File**: `main.go` (proxyCDN, handleMediaProxy, handleThumbProxy)
- [ ] Add cache headers to proxy responses:
  ```go
  w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
  w.Header().Set("ETag", generateETag(targetURL))
  ```
- [ ] Handle `If-None-Match` / `304 Not Modified` in proxyCDN for repeated requests
- [ ] Consider adding `Expires` header based on token expiry for `/vid/` routes

### 2.7 Observability — Request Logging
**File**: `main.go`
- [ ] Add request duration middleware (wrap http.DefaultServeMux or add to securityMiddleware):
  ```go
  func loggingMiddleware(next http.Handler) http.Handler {
      return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          start := time.Now()
          wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}
          next.ServeHTTP(wrapped, r)
          log.Printf("[%s] %s %s %d %s", r.Method, r.URL.Path, r.RemoteAddr, wrapped.statusCode, time.Since(start))
      })
  }
  ```
- [ ] Add `responseWriter` wrapper that captures status code
- [ ] Skip logging for SSE connections (Content-Type: text/event-stream)
- [ ] Log scrape errors with source and video ID context

## Verification
1. `curl -s https://adult.olibuijr.com/api/health | jq` returns all fields
2. `curl -sI https://adult.olibuijr.com/ | grep -i 'content-security-policy\|x-frame-options\|referrer-policy'` shows headers
3. Search with `?q=test&source=eporner&sort=views` returns filtered results with proper pagination
4. Thumbnails return `Cache-Control: public, max-age=86400`
5. `./deploy.sh backup` creates compressed backup on server
