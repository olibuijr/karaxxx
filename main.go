package main

import (
	"context"
	"crypto/hmac"
	crand "crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/PuerkitoBio/goquery"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

//go:embed templates/*
var templateFS embed.FS

var (
	db            *sql.DB
	tmpl          *template.Template
	httpClient    *http.Client
	mediaClient   *http.Client
	scrapeSem     chan struct{}
	videoCache    sync.Map
	bgWg          sync.WaitGroup
	progress      CrawlProgress
	crawlMu       sync.Mutex
	crawlMuXv     sync.Mutex
	crawlMuXh     sync.Mutex
	crawlMuEp     sync.Mutex
	crawlMuTf     sync.Mutex
	crawlMuDt     sync.Mutex
	crawlMuKVS    sync.Mutex
	catCache      catCacheT
	rateLimiter   chan time.Time
	rateLimitXv   chan time.Time
	rateLimitXh   chan time.Time
	rateLimitEp   chan time.Time
	rateLimitTf   chan time.Time
	rateLimitDt   chan time.Time
	rateLimitKVS  chan time.Time
	refreshLocks  sync.Map
	routeAPI      func(w http.ResponseWriter, r *http.Request)
	startTime     = time.Now()
	loginAttempts = make(map[string]*loginEntry)
	loginMu       sync.Mutex
	registerMu    sync.Mutex
	registerIPs   = make(map[string]*loginEntry)
	countCacheMu  sync.RWMutex
	countCache    = map[string]struct {
		n   int
		exp time.Time
	}{}
)

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:127.0) Gecko/20100101 Firefox/127.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:127.0) Gecko/20100101 Firefox/127.0",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64; rv:127.0) Gecko/20100101 Firefox/127.0",
}

type catCacheT struct {
	mu   sync.RWMutex
	cats []string
	last time.Time
}

type CrawlProgress struct {
	mu           sync.RWMutex
	Status       string         `json:"status"`
	Source       string         `json:"source"`
	Scanned      int            `json:"scanned"`
	NewVideos    int            `json:"new_videos"`
	Cached       int            `json:"cached"`
	DetailDone   int            `json:"detail_done"`
	DetailTotal  int            `json:"detail_total"`
	Page         int            `json:"page"`
	TotalCount   int            `json:"total_count"`
	SourceCounts map[string]int `json:"source_counts"`
}

const (
	xnxxBase                      = "https://www.xnxx.com"
	thumbCDN                      = "https://thumb-cdn77.xnxx-cdn.com"
	thumbsCDN                     = "https://thumbs-gcore.xnxx-cdn.com"
	mp4CDN                        = "https://mp4-cdn77.xnxx-cdn.com"
	hlsCDN                        = "https://hls-cdn77.xnxx-cdn.com"
	xhBase                        = "https://xhamster.com"
	xhCDN                         = "https://video3.xhcdn.com"
	epBase                        = "https://www.eporner.com"
	epCDN                         = "https://static-eu-cdn.eporner.com"
	dbPath                        = "karaxxx.db"
	port                          = "8799"
	scrapeWorkers                 = 5
	cacheTTL                      = 5 * time.Minute
	refreshEvery                  = 20 * time.Minute
	tokenRefreshLead              = 45 * time.Minute
	expiringRefreshBatch          = 50
	backfillBatchSize             = 12
	backfillEvery                 = 30 * time.Minute
	retryFailedEvery              = 15 * time.Minute
	crawlEvery                    = 6 * time.Hour
	dbMaxOpenConns                = 8
	dbBusyTimeout                 = 5 * time.Second
	crawlLockPath                 = "/tmp/karaxxx-crawl.lock"
	maxHTTPRetries                = 3
	retryBaseDelay                = 5 * time.Second
	retryMaxDelay                 = 30 * time.Second
	rateLimitInterval             = 400 * time.Millisecond
	countCacheTTL                 = 45 * time.Second
	failureBaseDelay              = 5 * time.Minute
	failureMaxDelay               = 6 * time.Hour
	maxFailuresPerBatch           = 20
	maxScrapeFailuresBeforeDelete = 8
	maxProxyBytes                 = 2 << 30
	playableMediaSQL              = "(COALESCE(url_360,'') <> '' OR COALESCE(url_720,'') <> '' OR COALESCE(url_1080,'') <> '' OR COALESCE(hls_url,'') <> '')"
	playableMediaSQLV             = "(COALESCE(v.url_360,'') <> '' OR COALESCE(v.url_720,'') <> '' OR COALESCE(v.url_1080,'') <> '' OR COALESCE(v.hls_url,'') <> '')"
	playableSources               = "xnxx,xhamster,xvideos,heavyfetish,punishbang,sunporno" // sources that provide playable media URLs via server-side scraping
)

var jwtSecret string

type Video struct {
	ID          string   `json:"id"`
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
	Tags        []string `json:"tags"`
	Uploader    string   `json:"uploader"`
	UploadDate  string   `json:"upload_date"`
	Duration    int      `json:"duration"`
	Views       int      `json:"views"`
	AddedAt     string   `json:"added_at"`
	Source      string   `json:"source"`
	ThumbUUID   string   `json:"thumb_uuid"`
	URL360      string   `json:"url_360"`
	URL720      string   `json:"url_720,omitempty"`
	URL1080     string   `json:"url_1080,omitempty"`
	PreviewURL  string   `json:"preview_url"`
	HLSURL      string   `json:"hls_url,omitempty"`
	SecureToken string   `json:"secure_token"`
	ExpiresAt   int64    `json:"expires_at"`
	WatchCount  int      `json:"watch_count"`
}

type cacheEntry struct {
	video     Video
	expiresAt time.Time
}

type loginEntry struct {
	attempts int
	until    time.Time
}

type fkTableMigration struct {
	name              string
	createSQL         string
	legacyCreateSQL   string
	indexes           []string
	requiredFragments []string
}

type fkOrphanCleanup struct {
	tableName string
	label     string
	sql       string
}

type jwtPayload struct {
	UID int    `json:"uid"`
	UN  string `json:"un"`
	Exp int64  `json:"exp"`
	Iat int64  `json:"iat"`
}

type requestIDContextKey struct{}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func parseRemoteIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	return net.ParseIP(strings.Trim(host, "[]"))
}

func clientIP(r *http.Request) string {
	remoteHost := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		remoteHost = host
	}
	remoteHost = strings.Trim(strings.TrimSpace(remoteHost), "[]")
	peerIP := parseRemoteIP(r.RemoteAddr)
	if peerIP == nil {
		return remoteHost
	}
	if peerIP.IsLoopback() || peerIP.IsPrivate() {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
			if forwardedIP := net.ParseIP(strings.Trim(first, "[]")); forwardedIP != nil {
				return forwardedIP.String()
			}
		}
	}
	return peerIP.String()
}

func isRateLimited(mu *sync.Mutex, attempts map[string]*loginEntry, ip string, limit int) bool {
	mu.Lock()
	defer mu.Unlock()
	entry := attempts[ip]
	return entry != nil && time.Now().Before(entry.until) && entry.attempts >= limit
}

func recordAttempt(mu *sync.Mutex, attempts map[string]*loginEntry, ip string, window time.Duration) {
	mu.Lock()
	defer mu.Unlock()
	now := time.Now()
	entry := attempts[ip]
	if entry == nil || now.After(entry.until) {
		attempts[ip] = &loginEntry{attempts: 1, until: now.Add(window)}
		return
	}
	entry.attempts++
}

func clearAttempts(mu *sync.Mutex, attempts map[string]*loginEntry, ip string) {
	mu.Lock()
	delete(attempts, ip)
	mu.Unlock()
}

func loadOrCreateJWTSecret() {
	if envSecret, ok := os.LookupEnv("KARAXXX_JWT_SECRET"); ok {
		if len(envSecret) >= 32 {
			jwtSecret = envSecret
			log.Println("Loaded JWT secret from env")
			return
		}
		log.Printf("Ignoring KARAXXX_JWT_SECRET: too short (%d bytes)", len(envSecret))
	}
	secretFile := dbPath + ".jwt_secret"
	if data, err := os.ReadFile(secretFile); err == nil && len(data) == 64 {
		jwtSecret = string(data)
		log.Println("Loaded JWT secret from file")
		return
	}
	jwtSecret = randomHex(32)
	if err := os.WriteFile(secretFile, []byte(jwtSecret), 0400); err != nil {
		log.Printf("Warning: could not persist JWT secret: %v", err)
	} else {
		log.Println("Created new JWT secret")
	}
}

// --- Init ---

// cleanupStaleLocks removes leftover crawl lock files from a previous process.
// On ungraceful exit (kill, crash, deploy restart), /tmp/karaxxx-*-crawl.lock
// files remain and cause the next process to skip crawls indefinitely.
func cleanupStaleLocks() {
	patterns := []string{
		"/tmp/karaxxx-crawl.lock",
		"/tmp/karaxxx-xh-crawl.lock",
		"/tmp/karaxxx-xv-crawl.lock",
		"/tmp/karaxxx-ep-crawl.lock",
		"/tmp/karaxxx-tf-crawl.lock",
		"/tmp/karaxxx-dt-crawl.lock",
		"/tmp/karaxxx-kvs-crawl.lock",
	}
	for _, p := range patterns {
		if err := os.Remove(p); err == nil {
			log.Printf("Removed stale lock file: %s", p)
		}
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "invite" {
		initInviteDB()
		runInviteCLI(os.Args[2:])
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	loadOrCreateJWTSecret()

	cleanupStaleLocks()

	initHTTPClient()
	initDB()
	cleanupStaleScrapeFailures()
	initTemplates()
	initRoutes()

	go refreshLoop(ctx)
	go backgroundBackfillLoop(ctx)
	go retryFailedLoop(ctx)
	go crawlLoop(ctx)
	go func() {
		// Prime xnxx session cookies for scraping and CDN proxy
		time.Sleep(2 * time.Second)
		httpGetWithRetry(xnxxBase + "/")
		log.Println("Primed xnxx session")
	}()
	go func() {
		time.Sleep(3 * time.Second)
		refreshCatCache()
		for range time.NewTicker(5 * time.Minute).C {
			refreshCatCache()
		}
	}()
	go func() {
		time.Sleep(10 * time.Second)
		refreshExpiring()
	}()
	go runDBMaintenance()
	go func() {
		for range time.NewTicker(5 * time.Minute).C {
			now := time.Now()
			loginMu.Lock()
			for ip, entry := range loginAttempts {
				if now.After(entry.until) {
					delete(loginAttempts, ip)
				}
			}
			loginMu.Unlock()
			registerMu.Lock()
			for ip, entry := range registerIPs {
				if now.After(entry.until) {
					delete(registerIPs, ip)
				}
			}
			registerMu.Unlock()
		}
	}()

	srv := &http.Server{Addr: ":" + port, Handler: securityMiddleware(loggingMiddleware(http.DefaultServeMux))}

	go func() {
		<-ctx.Done()
		log.Println("Shutting down gracefully...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Printf("KaraXXX listening on http://localhost:%s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}

	log.Println("Goodbye")
}

func newRateLimiter(interval time.Duration) chan time.Time {
	ch := make(chan time.Time, 1)
	go func() {
		for t := range time.NewTicker(interval).C {
			select {
			case ch <- t:
			default:
			}
		}
	}()
	return ch
}

func initHTTPClient() {
	jar, _ := cookiejar.New(nil)

	tr := &http.Transport{
		MaxIdleConns:       20,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: false,
	}
	httpClient = &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
		Jar:       jar,
	}
	mediaTr := &http.Transport{
		MaxIdleConns:          5,
		IdleConnTimeout:       90 * time.Second,
		DisableCompression:    false,
		ResponseHeaderTimeout: 15 * time.Second,
	}
	mediaClient = &http.Client{
		Transport: mediaTr,
		Timeout:   15 * time.Second,
		Jar:       jar,
	}
	scrapeSem = make(chan struct{}, scrapeWorkers)

	rateLimiter = make(chan time.Time, scrapeWorkers)
	go func() {
		for t := range time.NewTicker(rateLimitInterval).C {
			select {
			case rateLimiter <- t:
			default:
			}
		}
	}()
	// Per-provider rate limiters for parallel crawling
	rateLimitXv = newRateLimiter(rateLimitInterval)
	rateLimitXh = newRateLimiter(rateLimitInterval)
	rateLimitEp = newRateLimiter(2 * time.Second) // EPorner is aggressive with 429s
	rateLimitTf = newRateLimiter(rateLimitInterval)
	rateLimitDt = newRateLimiter(rateLimitInterval)
	rateLimitKVS = newRateLimiter(rateLimitInterval)
}

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", sqliteDSN())
	if err != nil {
		log.Fatal(err)
	}
	configureSQLitePool(db, dbMaxOpenConns)

	db.Exec(`CREATE TABLE IF NOT EXISTS videos (
		id TEXT PRIMARY KEY, slug TEXT, title TEXT, description TEXT,
		categories TEXT, tags TEXT, uploader TEXT, upload_date TEXT,
		duration INTEGER, views INTEGER, added_at TEXT, source TEXT DEFAULT 'xnxx',
		thumb_uuid TEXT,
		url_360 TEXT, url_720 TEXT, url_1080 TEXT, preview_url TEXT,
		hls_url TEXT, secure_token TEXT, expires_at INTEGER,
		created_at TEXT DEFAULT (datetime('now'))
	)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_added ON videos(added_at DESC)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_views ON videos(views DESC)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_title ON videos(title COLLATE NOCASE)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_expires ON videos(expires_at)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_source ON videos(source)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_uploader ON videos(uploader)`)

	var colCount int
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('videos') WHERE name='categories'").Scan(&colCount)
	if colCount == 0 {
		db.Exec(`ALTER TABLE videos ADD COLUMN categories TEXT DEFAULT ''`)
	}
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('videos') WHERE name='tags'").Scan(&colCount)
	if colCount == 0 {
		db.Exec(`ALTER TABLE videos ADD COLUMN tags TEXT DEFAULT ''`)
	}
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('videos') WHERE name='uploader'").Scan(&colCount)
	if colCount == 0 {
		db.Exec(`ALTER TABLE videos ADD COLUMN uploader TEXT DEFAULT ''`)
	}
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('videos') WHERE name='upload_date'").Scan(&colCount)
	if colCount == 0 {
		db.Exec(`ALTER TABLE videos ADD COLUMN upload_date TEXT DEFAULT ''`)
	}
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('videos') WHERE name='source'").Scan(&colCount)
	if colCount == 0 {
		db.Exec(`ALTER TABLE videos ADD COLUMN source TEXT DEFAULT 'xnxx'`)
		db.Exec(`CREATE INDEX IF NOT EXISTS idx_source ON videos(source)`)
	}

	db.Exec(`CREATE TABLE IF NOT EXISTS crawl_seeds (
		seed TEXT PRIMARY KEY,
		type TEXT DEFAULT 'tag',
		consumed INTEGER DEFAULT 0,
		created_at TEXT DEFAULT (datetime('now'))
	)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS video_categories (
		video_id TEXT NOT NULL,
		category TEXT NOT NULL,
		PRIMARY KEY (video_id, category)
	)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_vc_category ON video_categories(category)`)

	initCrawlSeeds()
	ensureVideosFTSWithUploader()
	backfillVideoCategoriesIfNeeded()

	// Auth tables
	db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		created_at TEXT DEFAULT (datetime('now'))
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS invite_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		key_hash TEXT UNIQUE NOT NULL,
		label TEXT DEFAULT '',
		max_uses INTEGER DEFAULT 1,
		uses INTEGER DEFAULT 0,
		expires_at INTEGER DEFAULT 0,
		revoked_at INTEGER DEFAULT 0,
		last_used_at INTEGER DEFAULT 0,
		last_used_by TEXT DEFAULT '',
		created_at TEXT DEFAULT (datetime('now'))
	)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_invite_keys_hash ON invite_keys(key_hash)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS favorites (
		user_id INTEGER NOT NULL,
		video_id TEXT NOT NULL,
		created_at TEXT DEFAULT (datetime('now')),
		PRIMARY KEY (user_id, video_id),
		FOREIGN KEY (user_id) REFERENCES users(id)
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS fav_categories (
		user_id INTEGER NOT NULL,
		category TEXT NOT NULL,
		created_at TEXT DEFAULT (datetime('now')),
		PRIMARY KEY (user_id, category),
		FOREIGN KEY (user_id) REFERENCES users(id)
	)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS scrape_failures (
		video_id TEXT PRIMARY KEY,
		retry_count INTEGER DEFAULT 0,
		last_error TEXT,
		next_retry_at INTEGER,
		created_at TEXT DEFAULT (datetime('now'))
	)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_fail_next ON scrape_failures(next_retry_at)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS watch_history (
		user_id INTEGER NOT NULL,
		video_id TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		duration INTEGER DEFAULT 0,
		play_count INTEGER DEFAULT 0,
		watched_at TEXT DEFAULT (datetime('now')),
		updated_at TEXT DEFAULT (datetime('now')),
		PRIMARY KEY (user_id, video_id),
		FOREIGN KEY (user_id) REFERENCES users(id),
		FOREIGN KEY (video_id) REFERENCES videos(id)
	)`)
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('watch_history') WHERE name='play_count'").Scan(&colCount)
	if colCount == 0 {
		db.Exec(`ALTER TABLE watch_history ADD COLUMN play_count INTEGER DEFAULT 0`)
	}
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_watch_history_user ON watch_history(user_id, updated_at DESC)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_watch_history_video ON watch_history(video_id)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS user_profiles (
		user_id INTEGER PRIMARY KEY,
		display_name TEXT DEFAULT '',
		anonymous_name TEXT NOT NULL,
		comment_anonymously INTEGER DEFAULT 1,
		created_at TEXT DEFAULT (datetime('now')),
		updated_at TEXT DEFAULT (datetime('now')),
		FOREIGN KEY (user_id) REFERENCES users(id)
	)`)
	profileRows, err := db.Query(`SELECT u.id FROM users u LEFT JOIN user_profiles p ON p.user_id = u.id WHERE p.user_id IS NULL`)
	if err == nil {
		missingProfileIDs := []int{}
		for profileRows.Next() {
			var userID int
			if profileRows.Scan(&userID) == nil {
				missingProfileIDs = append(missingProfileIDs, userID)
			}
		}
		profileRows.Close()
		for _, userID := range missingProfileIDs {
			db.Exec("INSERT OR IGNORE INTO user_profiles (user_id, anonymous_name) VALUES (?, ?)", userID, createAnonymousName())
		}
	}

	db.Exec(`CREATE TABLE IF NOT EXISTS video_watch_counts (
		video_id TEXT PRIMARY KEY,
		watch_count INTEGER DEFAULT 0,
		updated_at TEXT DEFAULT (datetime('now')),
		FOREIGN KEY (video_id) REFERENCES videos(id)
	)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS video_comments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		video_id TEXT NOT NULL,
		user_id INTEGER NOT NULL,
		body TEXT NOT NULL,
		display_name TEXT NOT NULL,
		anonymous INTEGER DEFAULT 1,
		created_at TEXT DEFAULT (datetime('now')),
		FOREIGN KEY (video_id) REFERENCES videos(id),
		FOREIGN KEY (user_id) REFERENCES users(id)
	)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_video_comments_video ON video_comments(video_id, created_at DESC)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS video_reactions (
		video_id TEXT NOT NULL,
		user_id INTEGER NOT NULL,
		reaction TEXT NOT NULL,
		created_at TEXT DEFAULT (datetime('now')),
		PRIMARY KEY (video_id, user_id, reaction),
		FOREIGN KEY (video_id) REFERENCES videos(id),
		FOREIGN KEY (user_id) REFERENCES users(id)
	)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_video_reactions_video ON video_reactions(video_id)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS wall_comments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		wall_user_id INTEGER NOT NULL,
		author_id INTEGER NOT NULL,
		body TEXT NOT NULL,
		display_name TEXT NOT NULL,
		anonymous INTEGER DEFAULT 1,
		created_at TEXT DEFAULT (datetime('now')),
		FOREIGN KEY (wall_user_id) REFERENCES users(id),
		FOREIGN KEY (author_id) REFERENCES users(id)
	)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_wall_comments_wall ON wall_comments(wall_user_id, created_at DESC)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS playlists (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		is_public INTEGER DEFAULT 0,
		created_at TEXT DEFAULT (datetime('now')),
		FOREIGN KEY (user_id) REFERENCES users(id)
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS playlist_videos (
		playlist_id INTEGER NOT NULL,
		video_id TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		added_at TEXT DEFAULT (datetime('now')),
		PRIMARY KEY (playlist_id, video_id),
		FOREIGN KEY (playlist_id) REFERENCES playlists(id),
		FOREIGN KEY (video_id) REFERENCES videos(id)
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS ratings (
		user_id INTEGER NOT NULL,
		video_id TEXT NOT NULL,
		rating INTEGER NOT NULL CHECK (rating IN (-1, 1)),
		created_at TEXT DEFAULT (datetime('now')),
		PRIMARY KEY (user_id, video_id),
		FOREIGN KEY (user_id) REFERENCES users(id),
		FOREIGN KEY (video_id) REFERENCES videos(id)
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS watch_later (
		user_id INTEGER NOT NULL,
		video_id TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		added_at TEXT DEFAULT (datetime('now')),
		PRIMARY KEY (user_id, video_id),
		FOREIGN KEY (user_id) REFERENCES users(id),
		FOREIGN KEY (video_id) REFERENCES videos(id)
	)`)

	migrateForeignKeyCascades()
}

func migrateForeignKeyCascades() {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		log.Printf("FK migration connection failed: %v", err)
		return
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		log.Printf("FK migration could not disable foreign keys: %v", err)
		return
	}
	restoreFKs := true
	defer func() {
		if restoreFKs {
			if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
				log.Printf("FK migration could not restore foreign keys: %v", err)
			}
		}
	}()

	for _, migration := range foreignKeyCascadeMigrations() {
		schemaSQL, err := loadTableSchemaSQL(ctx, conn, migration.name)
		if err != nil {
			log.Printf("FK migration schema lookup for %s failed: %v", migration.name, err)
			continue
		}
		if schemaSQL == "" {
			continue
		}
		if strings.Contains(strings.ToUpper(schemaSQL), "ON DELETE CASCADE") {
			continue
		}

		createSQL := migration.createSQL
		if migration.legacyCreateSQL != "" && schemaContainsAll(schemaSQL, migration.requiredFragments) {
			createSQL = migration.legacyCreateSQL
		}

		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			log.Printf("FK migration for %s failed: %v", migration.name, err)
			continue
		}
		if err := rebuildTableWithCascade(ctx, tx, migration.name, createSQL, migration.indexes); err != nil {
			tx.Rollback()
			log.Printf("FK migration for %s failed: %v", migration.name, err)
			continue
		}
		if err := tx.Commit(); err != nil {
			tx.Rollback()
			log.Printf("FK migration for %s failed: %v", migration.name, err)
			continue
		}
	}

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		log.Printf("FK migration could not restore foreign keys: %v", err)
		return
	}
	restoreFKs = false

	runForeignKeyOrphanCleanup(ctx, conn)
}

func foreignKeyCascadeMigrations() []fkTableMigration {
	return []fkTableMigration{
		{
			name: "video_categories",
			createSQL: `CREATE TABLE video_categories__new (
		video_id TEXT NOT NULL,
		category TEXT NOT NULL,
		PRIMARY KEY (video_id, category),
		FOREIGN KEY (video_id) REFERENCES videos(id) ON DELETE CASCADE
	)`,
			indexes: []string{
				`CREATE INDEX IF NOT EXISTS idx_vc_category ON video_categories(category)`,
			},
		},
		{
			name: "favorites",
			createSQL: `CREATE TABLE favorites__new (
		user_id INTEGER NOT NULL,
		video_id TEXT NOT NULL,
		created_at TEXT DEFAULT (datetime('now')),
		PRIMARY KEY (user_id, video_id),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	)`,
		},
		{
			name: "fav_categories",
			createSQL: `CREATE TABLE fav_categories__new (
		user_id INTEGER NOT NULL,
		category TEXT NOT NULL,
		created_at TEXT DEFAULT (datetime('now')),
		PRIMARY KEY (user_id, category),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	)`,
		},
		{
			name: "scrape_failures",
			createSQL: `CREATE TABLE scrape_failures__new (
		video_id TEXT PRIMARY KEY,
		retry_count INTEGER DEFAULT 0,
		last_error TEXT,
		next_retry_at INTEGER,
		created_at TEXT DEFAULT (datetime('now')),
		FOREIGN KEY (video_id) REFERENCES videos(id) ON DELETE CASCADE
	)`,
			indexes: []string{
				`CREATE INDEX IF NOT EXISTS idx_fail_next ON scrape_failures(next_retry_at)`,
			},
		},
		{
			name: "watch_history",
			createSQL: `CREATE TABLE watch_history__new (
		user_id INTEGER NOT NULL,
		video_id TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		duration INTEGER DEFAULT 0,
		play_count INTEGER DEFAULT 0,
		watched_at TEXT DEFAULT (datetime('now')),
		updated_at TEXT DEFAULT (datetime('now')),
		PRIMARY KEY (user_id, video_id),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (video_id) REFERENCES videos(id) ON DELETE CASCADE
	)`,
			legacyCreateSQL: `CREATE TABLE watch_history__new (
		user_id INTEGER NOT NULL,
		video_id TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		duration INTEGER DEFAULT 0,
		watched_at TEXT DEFAULT (datetime('now')),
		updated_at TEXT DEFAULT (datetime('now')),
		play_count INTEGER DEFAULT 0,
		PRIMARY KEY (user_id, video_id),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (video_id) REFERENCES videos(id) ON DELETE CASCADE
	)`,
			requiredFragments: []string{
				"UPDATED_AT TEXT DEFAULT (DATETIME('NOW')), PLAY_COUNT INTEGER DEFAULT 0",
			},
			indexes: []string{
				`CREATE INDEX IF NOT EXISTS idx_watch_history_user ON watch_history(user_id, updated_at DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_watch_history_video ON watch_history(video_id)`,
			},
		},
		{
			name: "user_profiles",
			createSQL: `CREATE TABLE user_profiles__new (
		user_id INTEGER PRIMARY KEY,
		display_name TEXT DEFAULT '',
		anonymous_name TEXT NOT NULL,
		comment_anonymously INTEGER DEFAULT 1,
		created_at TEXT DEFAULT (datetime('now')),
		updated_at TEXT DEFAULT (datetime('now')),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	)`,
		},
		{
			name: "video_watch_counts",
			createSQL: `CREATE TABLE video_watch_counts__new (
		video_id TEXT PRIMARY KEY,
		watch_count INTEGER DEFAULT 0,
		updated_at TEXT DEFAULT (datetime('now')),
		FOREIGN KEY (video_id) REFERENCES videos(id) ON DELETE CASCADE
	)`,
		},
		{
			name: "video_comments",
			createSQL: `CREATE TABLE video_comments__new (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		video_id TEXT NOT NULL,
		user_id INTEGER NOT NULL,
		body TEXT NOT NULL,
		display_name TEXT NOT NULL,
		anonymous INTEGER DEFAULT 1,
		created_at TEXT DEFAULT (datetime('now')),
		FOREIGN KEY (video_id) REFERENCES videos(id) ON DELETE CASCADE,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	)`,
			indexes: []string{
				`CREATE INDEX IF NOT EXISTS idx_video_comments_video ON video_comments(video_id, created_at DESC)`,
			},
		},
		{
			name: "video_reactions",
			createSQL: `CREATE TABLE video_reactions__new (
		video_id TEXT NOT NULL,
		user_id INTEGER NOT NULL,
		reaction TEXT NOT NULL,
		created_at TEXT DEFAULT (datetime('now')),
		PRIMARY KEY (video_id, user_id, reaction),
		FOREIGN KEY (video_id) REFERENCES videos(id) ON DELETE CASCADE,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	)`,
			indexes: []string{
				`CREATE INDEX IF NOT EXISTS idx_video_reactions_video ON video_reactions(video_id)`,
			},
		},
		{
			name: "wall_comments",
			createSQL: `CREATE TABLE wall_comments__new (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		wall_user_id INTEGER NOT NULL,
		author_id INTEGER NOT NULL,
		body TEXT NOT NULL,
		display_name TEXT NOT NULL,
		anonymous INTEGER DEFAULT 1,
		created_at TEXT DEFAULT (datetime('now')),
		FOREIGN KEY (wall_user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE CASCADE
	)`,
			indexes: []string{
				`CREATE INDEX IF NOT EXISTS idx_wall_comments_wall ON wall_comments(wall_user_id, created_at DESC)`,
			},
		},
		{
			name: "playlists",
			createSQL: `CREATE TABLE playlists__new (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		is_public INTEGER DEFAULT 0,
		created_at TEXT DEFAULT (datetime('now')),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	)`,
		},
		{
			name: "playlist_videos",
			createSQL: `CREATE TABLE playlist_videos__new (
		playlist_id INTEGER NOT NULL,
		video_id TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		added_at TEXT DEFAULT (datetime('now')),
		PRIMARY KEY (playlist_id, video_id),
		FOREIGN KEY (playlist_id) REFERENCES playlists(id) ON DELETE CASCADE,
		FOREIGN KEY (video_id) REFERENCES videos(id) ON DELETE CASCADE
	)`,
		},
		{
			name: "ratings",
			createSQL: `CREATE TABLE ratings__new (
		user_id INTEGER NOT NULL,
		video_id TEXT NOT NULL,
		rating INTEGER NOT NULL CHECK (rating IN (-1, 1)),
		created_at TEXT DEFAULT (datetime('now')),
		PRIMARY KEY (user_id, video_id),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (video_id) REFERENCES videos(id) ON DELETE CASCADE
	)`,
		},
		{
			name: "watch_later",
			createSQL: `CREATE TABLE watch_later__new (
		user_id INTEGER NOT NULL,
		video_id TEXT NOT NULL,
		position INTEGER DEFAULT 0,
		added_at TEXT DEFAULT (datetime('now')),
		PRIMARY KEY (user_id, video_id),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (video_id) REFERENCES videos(id) ON DELETE CASCADE
	)`,
		},
	}
}

func rebuildTableWithCascade(ctx context.Context, tx *sql.Tx, tableName, createSQL string, indexes []string) error {
	if _, err := tx.ExecContext(ctx, createSQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s__new SELECT * FROM %s", tableName, tableName)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("DROP TABLE %s", tableName)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s__new RENAME TO %s", tableName, tableName)); err != nil {
		return err
	}
	for _, indexSQL := range indexes {
		if _, err := tx.ExecContext(ctx, indexSQL); err != nil {
			return err
		}
	}
	return nil
}

func runForeignKeyOrphanCleanup(ctx context.Context, conn *sql.Conn) {
	for _, cleanup := range foreignKeyOrphanCleanups() {
		schemaSQL, err := loadTableSchemaSQL(ctx, conn, cleanup.tableName)
		if err != nil {
			log.Printf("FK orphan cleanup schema lookup for %s failed: %v", cleanup.tableName, err)
			continue
		}
		if schemaSQL == "" {
			continue
		}
		res, err := conn.ExecContext(ctx, cleanup.sql)
		if err != nil {
			log.Printf("FK orphan cleanup for %s failed: %v", cleanup.label, err)
			continue
		}
		rowsDeleted, err := res.RowsAffected()
		if err == nil && rowsDeleted > 0 {
			log.Printf("FK orphan cleanup removed %d rows from %s", rowsDeleted, cleanup.label)
		}
	}
}

func foreignKeyOrphanCleanups() []fkOrphanCleanup {
	return []fkOrphanCleanup{
		{tableName: "video_categories", label: "video_categories.video_id", sql: `DELETE FROM video_categories WHERE video_id NOT IN (SELECT id FROM videos)`},
		{tableName: "scrape_failures", label: "scrape_failures.video_id", sql: `DELETE FROM scrape_failures WHERE video_id NOT IN (SELECT id FROM videos)`},
		{tableName: "playlist_videos", label: "playlist_videos.video_id", sql: `DELETE FROM playlist_videos WHERE video_id NOT IN (SELECT id FROM videos)`},
		{tableName: "playlist_videos", label: "playlist_videos.playlist_id", sql: `DELETE FROM playlist_videos WHERE playlist_id NOT IN (SELECT id FROM playlists)`},
		{tableName: "watch_history", label: "watch_history.video_id", sql: `DELETE FROM watch_history WHERE video_id NOT IN (SELECT id FROM videos)`},
		{tableName: "video_reactions", label: "video_reactions.video_id", sql: `DELETE FROM video_reactions WHERE video_id NOT IN (SELECT id FROM videos)`},
		{tableName: "video_comments", label: "video_comments.video_id", sql: `DELETE FROM video_comments WHERE video_id NOT IN (SELECT id FROM videos)`},
		{tableName: "video_watch_counts", label: "video_watch_counts.video_id", sql: `DELETE FROM video_watch_counts WHERE video_id NOT IN (SELECT id FROM videos)`},
		{tableName: "ratings", label: "ratings.video_id", sql: `DELETE FROM ratings WHERE video_id NOT IN (SELECT id FROM videos)`},
		{tableName: "watch_later", label: "watch_later.video_id", sql: `DELETE FROM watch_later WHERE video_id NOT IN (SELECT id FROM videos)`},
		{tableName: "favorites", label: "favorites.video_id", sql: `DELETE FROM favorites WHERE video_id NOT IN (SELECT id FROM videos)`},
		{tableName: "favorites", label: "favorites.user_id", sql: `DELETE FROM favorites WHERE user_id NOT IN (SELECT id FROM users)`},
		{tableName: "fav_categories", label: "fav_categories.user_id", sql: `DELETE FROM fav_categories WHERE user_id NOT IN (SELECT id FROM users)`},
		{tableName: "watch_history", label: "watch_history.user_id", sql: `DELETE FROM watch_history WHERE user_id NOT IN (SELECT id FROM users)`},
		{tableName: "playlists", label: "playlists.user_id", sql: `DELETE FROM playlists WHERE user_id NOT IN (SELECT id FROM users)`},
		{tableName: "video_comments", label: "video_comments.user_id", sql: `DELETE FROM video_comments WHERE user_id NOT IN (SELECT id FROM users)`},
		{tableName: "video_reactions", label: "video_reactions.user_id", sql: `DELETE FROM video_reactions WHERE user_id NOT IN (SELECT id FROM users)`},
		{tableName: "ratings", label: "ratings.user_id", sql: `DELETE FROM ratings WHERE user_id NOT IN (SELECT id FROM users)`},
		{tableName: "watch_later", label: "watch_later.user_id", sql: `DELETE FROM watch_later WHERE user_id NOT IN (SELECT id FROM users)`},
		{tableName: "user_profiles", label: "user_profiles.user_id", sql: `DELETE FROM user_profiles WHERE user_id NOT IN (SELECT id FROM users)`},
		{tableName: "wall_comments", label: "wall_comments.author_id", sql: `DELETE FROM wall_comments WHERE author_id NOT IN (SELECT id FROM users)`},
		{tableName: "wall_comments", label: "wall_comments.wall_user_id", sql: `DELETE FROM wall_comments WHERE wall_user_id NOT IN (SELECT id FROM users)`},
	}
}

func loadTableSchemaSQL(ctx context.Context, conn *sql.Conn, tableName string) (string, error) {
	var schemaSQL sql.NullString
	err := conn.QueryRowContext(ctx, "SELECT sql FROM sqlite_master WHERE type='table' AND name = ?", tableName).Scan(&schemaSQL)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return schemaSQL.String, nil
}

func schemaContainsAll(schemaSQL string, fragments []string) bool {
	upperSchema := strings.ToUpper(schemaSQL)
	for _, fragment := range fragments {
		if !strings.Contains(upperSchema, strings.ToUpper(fragment)) {
			return false
		}
	}
	return true
}

func ensureVideosFTSWithUploader() {
	var uploaderColCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_xinfo('videos_fts') WHERE name = 'uploader'").Scan(&uploaderColCount); err != nil {
		log.Printf("videos_fts schema check failed: %v", err)
		return
	}
	if uploaderColCount == 0 {
		if _, err := db.Exec(`DROP TRIGGER IF EXISTS videos_ai`); err != nil {
			log.Printf("drop videos_ai failed: %v", err)
		}
		if _, err := db.Exec(`DROP TRIGGER IF EXISTS videos_ad`); err != nil {
			log.Printf("drop videos_ad failed: %v", err)
		}
		if _, err := db.Exec(`DROP TRIGGER IF EXISTS videos_au`); err != nil {
			log.Printf("drop videos_au failed: %v", err)
		}
		if _, err := db.Exec(`DROP TABLE IF EXISTS videos_fts`); err != nil {
			log.Printf("drop videos_fts failed: %v", err)
		}
	}
	if err := createVideosFTSTable(); err != nil {
		log.Printf("create videos_fts failed: %v", err)
		return
	}
	if err := recreateVideosFTSTriggers(); err != nil {
		log.Printf("create videos_fts triggers failed: %v", err)
		return
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO videos_fts(rowid, title, description, categories, tags, uploader)
		SELECT rowid, title, description, categories, tags, uploader FROM videos`); err != nil {
		log.Printf("videos_fts backfill failed: %v", err)
	}
}

func createVideosFTSTable() error {
	_, err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS videos_fts USING fts5(
		title, description, categories, tags, uploader,
		content='videos', content_rowid='rowid'
	)`)
	return err
}

func recreateVideosFTSTriggers() error {
	for _, stmt := range []string{
		`DROP TRIGGER IF EXISTS videos_ai`,
		`DROP TRIGGER IF EXISTS videos_ad`,
		`DROP TRIGGER IF EXISTS videos_au`,
		`CREATE TRIGGER videos_ai AFTER INSERT ON videos BEGIN
			INSERT INTO videos_fts(rowid, title, description, categories, tags, uploader)
			VALUES (new.rowid, new.title, new.description, new.categories, new.tags, new.uploader);
		END`,
		`CREATE TRIGGER videos_ad AFTER DELETE ON videos BEGIN
			INSERT INTO videos_fts(videos_fts, rowid, title, description, categories, tags, uploader)
			VALUES ('delete', old.rowid, old.title, old.description, old.categories, old.tags, old.uploader);
		END`,
		`CREATE TRIGGER videos_au AFTER UPDATE ON videos BEGIN
			INSERT INTO videos_fts(videos_fts, rowid, title, description, categories, tags, uploader)
			VALUES ('delete', old.rowid, old.title, old.description, old.categories, old.tags, old.uploader);
			INSERT INTO videos_fts(rowid, title, description, categories, tags, uploader)
			VALUES (new.rowid, new.title, new.description, new.categories, new.tags, new.uploader);
		END`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func backfillVideoCategoriesIfNeeded() {
	var existing int
	if err := db.QueryRow(`SELECT COUNT(*) FROM video_categories`).Scan(&existing); err != nil {
		log.Printf("video_categories count failed: %v", err)
		return
	}
	if existing > 0 {
		return
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("video_categories backfill begin failed: %v", err)
		return
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id, categories FROM videos WHERE categories != '' AND categories IS NOT NULL`)
	if err != nil {
		log.Printf("video_categories backfill query failed: %v", err)
		return
	}
	defer rows.Close()

	insertStmt, err := tx.Prepare(`INSERT OR IGNORE INTO video_categories(video_id, category) VALUES (?, ?)`)
	if err != nil {
		log.Printf("video_categories backfill prepare failed: %v", err)
		return
	}
	defer insertStmt.Close()

	for rows.Next() {
		var videoID string
		var categories string
		if err := rows.Scan(&videoID, &categories); err != nil {
			log.Printf("video_categories backfill scan failed: %v", err)
			return
		}
		for _, category := range normalizeCategoryList(strings.Split(categories, ",")) {
			if _, err := insertStmt.Exec(videoID, category); err != nil {
				log.Printf("video_categories backfill insert failed for %s: %v", videoID, err)
				return
			}
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("video_categories backfill rows failed: %v", err)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("video_categories backfill commit failed: %v", err)
	}
}

func normalizeCategoryTerm(raw string) string {
	category := strings.ToLower(strings.TrimSpace(raw))
	if category == "" || category == "uncategorized" {
		return ""
	}
	return category
}

func normalizeCategoryList(values []string) []string {
	seen := make(map[string]bool, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		category := normalizeCategoryTerm(value)
		if category == "" || seen[category] {
			continue
		}
		seen[category] = true
		normalized = append(normalized, category)
	}
	return normalized
}

func parseCategoryFilter(raw string) []string {
	return normalizeCategoryList(strings.Split(raw, ","))
}

func mergeCategoryLists(lists ...[]string) []string {
	merged := []string{}
	seen := map[string]bool{}
	for _, list := range lists {
		for _, value := range list {
			category := normalizeCategoryTerm(value)
			if category == "" || seen[category] {
				continue
			}
			seen[category] = true
			merged = append(merged, category)
		}
	}
	return merged
}

func joinStoredCategories(categories []string) string {
	normalized := normalizeCategoryList(categories)
	if len(normalized) == 0 {
		return "uncategorized"
	}
	return strings.Join(normalized, ",")
}

func replaceVideoCategories(videoID string, categories []string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := replaceVideoCategoriesTx(tx, videoID, categories); err != nil {
		return err
	}
	return tx.Commit()
}

func replaceVideoCategoriesTx(tx *sql.Tx, videoID string, categories []string) error {
	if _, err := tx.Exec(`DELETE FROM video_categories WHERE video_id = ?`, videoID); err != nil {
		return err
	}
	for _, category := range normalizeCategoryList(categories) {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO video_categories(video_id, category) VALUES (?, ?)`, videoID, category); err != nil {
			return err
		}
	}
	return nil
}

func loadVideoCategories(videoID string) []string {
	rows, err := db.Query(`SELECT category FROM video_categories WHERE video_id = ? ORDER BY category`, videoID)
	if err != nil {
		log.Printf("loadVideoCategories(%s) failed: %v", videoID, err)
		return nil
	}
	defer rows.Close()

	categories := []string{}
	for rows.Next() {
		var category string
		if err := rows.Scan(&category); err != nil {
			log.Printf("loadVideoCategories(%s) scan failed: %v", videoID, err)
			return categories
		}
		if category = normalizeCategoryTerm(category); category != "" {
			categories = append(categories, category)
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("loadVideoCategories(%s) rows failed: %v", videoID, err)
	}
	return categories
}

func cachedCount(key string, query string, args ...any) int {
	cacheKey := fmt.Sprintf("%s|%s|%v", key, query, args)
	now := time.Now()

	countCacheMu.RLock()
	entry, ok := countCache[cacheKey]
	countCacheMu.RUnlock()
	if ok && now.Before(entry.exp) {
		return entry.n
	}

	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		log.Printf("cachedCount query failed (key=%s): %v", key, err)
		return 0
	}

	countCacheMu.Lock()
	countCache[cacheKey] = struct {
		n   int
		exp time.Time
	}{
		n:   n,
		exp: now.Add(countCacheTTL),
	}
	countCacheMu.Unlock()
	return n
}

func newRequestID() string {
	var buf [4]byte
	if _, err := crand.Read(buf[:]); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(buf[:])
}

func reqLogf(r *http.Request, format string, args ...any) {
	reqID := "-"
	if r != nil {
		if value := r.Context().Value(requestIDContextKey{}); value != nil {
			if id, ok := value.(string); ok && id != "" {
				reqID = id
			}
		}
	}
	log.Printf("req_id=%s "+format, append([]any{reqID}, args...)...)
}

func initInviteDB() {
	var err error
	db, err = sql.Open("sqlite3", sqliteDSN())
	if err != nil {
		log.Fatal(err)
	}
	configureSQLitePool(db, 1)
	db.Exec(`CREATE TABLE IF NOT EXISTS invite_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		key_hash TEXT UNIQUE NOT NULL,
		label TEXT DEFAULT '',
		max_uses INTEGER DEFAULT 1,
		uses INTEGER DEFAULT 0,
		expires_at INTEGER DEFAULT 0,
		revoked_at INTEGER DEFAULT 0,
		last_used_at INTEGER DEFAULT 0,
		last_used_by TEXT DEFAULT '',
		created_at TEXT DEFAULT (datetime('now'))
	)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_invite_keys_hash ON invite_keys(key_hash)`)
}

func sqliteDSN() string {
	return dbPath + "?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000&_foreign_keys=on"
}

func configureSQLitePool(conn *sql.DB, maxOpen int) {
	conn.SetMaxOpenConns(maxOpen)
	conn.SetMaxIdleConns(maxOpen)
	conn.SetConnMaxLifetime(0)
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		log.Printf("sqlite WAL pragma failed: %v", err)
	}
	if _, err := conn.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		log.Printf("sqlite synchronous pragma failed: %v", err)
	}
	if _, err := conn.Exec(fmt.Sprintf("PRAGMA busy_timeout=%d", dbBusyTimeout.Milliseconds())); err != nil {
		log.Printf("sqlite busy_timeout pragma failed: %v", err)
	}
}

func cleanupStaleScrapeFailures() {
	if db == nil {
		return
	}
	if res, err := db.Exec(`DELETE FROM scrape_failures
		WHERE video_id IN (SELECT id FROM videos WHERE source IN ('eporner','drtuber','tnaflix'))`); err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			log.Printf("Cleared %d stale metadata-only scrape failures", n)
		}
	} else {
		log.Printf("metadata-only scrape failure cleanup failed: %v", err)
	}

	rows, err := db.Query(`SELECT f.video_id
		FROM scrape_failures f
		JOIN videos v ON v.id = f.video_id
		WHERE lower(f.last_error) LIKE '%redirected off-site to www.xnxx.gold%'
		AND COALESCE(v.url_360,'') = ''
		AND COALESCE(v.url_720,'') = ''
		AND COALESCE(v.url_1080,'') = ''
		AND COALESCE(v.hls_url,'') = ''`)
	if err != nil {
		log.Printf("permanent scrape failure cleanup query failed: %v", err)
		return
	}
	defer rows.Close()
	deleted := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			deleteVideoEverywhere(id, "permanent xnxx.gold redirect")
			deleted++
		}
	}
	if deleted > 0 {
		log.Printf("Deleted %d unplayable xnxx.gold teaser rows", deleted)
	}
}

func runDBMaintenance() {
	for range time.NewTicker(6 * time.Hour).C {
		log.Println("Running DB maintenance...")
		db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		db.Exec("PRAGMA optimize")
		db.Exec("REINDEX videos_fts")
		var integrity string
		db.QueryRow("PRAGMA integrity_check").Scan(&integrity)
		if integrity != "ok" {
			log.Printf("DB integrity check failed: %s", integrity)
		} else {
			log.Println("DB maintenance complete (integrity: ok)")
		}
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	dbSize := int64(0)
	if fi, err := os.Stat(dbPath); err == nil {
		dbSize = fi.Size()
	}
	walSize := int64(0)
	if fi, err := os.Stat(dbPath + "-wal"); err == nil {
		walSize = fi.Size()
	}

	stats := db.Stats()
	resp := map[string]interface{}{
		"status":              "ok",
		"db_size_bytes":       dbSize,
		"wal_size_bytes":      walSize,
		"uptime_seconds":      int(time.Since(startTime).Seconds()),
		"goroutines":          runtime.NumGoroutine(),
		"db_open_connections": stats.OpenConnections,
		"db_in_use":           stats.InUse,
		"db_idle":             stats.Idle,
		"db_wait_count":       stats.WaitCount,
		"videos_total":        cachedCount("videos_total", "SELECT COUNT(*) FROM videos"),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
	defer cancel()

	var staleTokens int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM videos WHERE expires_at < unixepoch() AND expires_at > 0").Scan(&staleTokens); err == nil {
		resp["stale_tokens"] = staleTokens
	} else {
		resp["db_metrics_error"] = err.Error()
	}

	var failCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scrape_failures").Scan(&failCount); err == nil {
		resp["scrape_failures"] = failCount
	} else if resp["db_metrics_error"] == nil {
		resp["db_metrics_error"] = err.Error()
	}

	videosBySource := map[string]int{}
	vrows, err := db.QueryContext(ctx, "SELECT source, COUNT(*) FROM videos GROUP BY source")
	if err == nil {
		defer vrows.Close()
		for vrows.Next() {
			var src string
			var cnt int
			vrows.Scan(&src, &cnt)
			videosBySource[src] = cnt
		}
		resp["videos_by_source"] = videosBySource
	} else if resp["db_metrics_error"] == nil {
		resp["db_metrics_error"] = err.Error()
	}

	json.NewEncoder(w).Encode(resp)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		reqID := newRequestID()
		r = r.WithContext(context.WithValue(r.Context(), requestIDContextKey{}, reqID))
		rw := &responseWriter{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(rw, r)
		log.Printf("req_id=%s method=%s path=%s remote=%s status=%d dur_ms=%.3f", reqID, r.Method, r.URL.Path, r.RemoteAddr, rw.statusCode, float64(time.Since(start))/float64(time.Millisecond))
	})
}

func initTemplates() {
	funcMap := template.FuncMap{
		"formatDuration": func(secs int) string {
			m := secs / 60
			s := secs % 60
			return fmt.Sprintf("%d:%02d", m, s)
		},
		"formatViews": func(n int) string {
			if n >= 1000000 {
				return fmt.Sprintf("%.1fM", float64(n)/1000000)
			}
			if n >= 1000 {
				return fmt.Sprintf("%.1fK", float64(n)/1000)
			}
			return fmt.Sprintf("%d", n)
		},
		"hasPrefix": strings.HasPrefix,
		"add":       func(a, b int) int { return a + b },
		"sub":       func(a, b int) int { return a - b },
		"gt":        func(a, b int) bool { return a > b },
		"paginate": func(page, totalPages int) []int {
			start := page - 3
			if start < 1 {
				start = 1
			}
			end := page + 3
			if end > totalPages {
				end = totalPages
			}
			var pages []int
			for i := start; i <= end; i++ {
				pages = append(pages, i)
			}
			return pages
		},
	}
	tmpl = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))
}

// --- Auth ---

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := crand.Read(b); err != nil {
		log.Printf("crand.Read failed: %v", err)
		for i := range b {
			b[i] = byte(time.Now().UnixNano() & 0xff)
		}
	}
	return hex.EncodeToString(b)
}

func randomInviteKey() string {
	b := make([]byte, 24)
	if _, err := crand.Read(b); err != nil {
		return "kxxx_" + randomHex(24)
	}
	return "kxxx_" + base64.RawURLEncoding.EncodeToString(b)
}

func inviteKeyHash(key string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:])
}

func printInviteUsage() {
	fmt.Println(`Usage:
  karaxxx invite create [label] [--uses N] [--days N]
  karaxxx invite list
  karaxxx invite revoke <key-or-hash>

Examples:
  karaxxx invite create alice --days 14
  karaxxx invite create beta --uses 5 --days 30`)
}

func runInviteCLI(args []string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		printInviteUsage()
		return
	}
	switch args[0] {
	case "create":
		labelParts := []string{}
		maxUses := 1
		days := 30
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--uses":
				if i+1 >= len(args) {
					log.Fatal("--uses requires a number")
				}
				i++
				if _, err := fmt.Sscanf(args[i], "%d", &maxUses); err != nil || maxUses < 1 {
					log.Fatal("--uses must be a positive number")
				}
			case "--days":
				if i+1 >= len(args) {
					log.Fatal("--days requires a number")
				}
				i++
				if _, err := fmt.Sscanf(args[i], "%d", &days); err != nil || days < 1 {
					log.Fatal("--days must be a positive number")
				}
			default:
				labelParts = append(labelParts, args[i])
			}
		}
		key := randomInviteKey()
		expiresAt := time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
		label := strings.Join(labelParts, " ")
		if _, err := db.Exec(`INSERT INTO invite_keys (key_hash, label, max_uses, expires_at) VALUES (?, ?, ?, ?)`,
			inviteKeyHash(key), label, maxUses, expiresAt); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Invite key: %s\n", key)
		fmt.Printf("Uses: %d\n", maxUses)
		fmt.Printf("Expires: %s\n", time.Unix(expiresAt, 0).Format(time.RFC3339))
		if label != "" {
			fmt.Printf("Label: %s\n", label)
		}
	case "list":
		rows, err := db.Query(`SELECT id, label, max_uses, uses, expires_at, revoked_at, last_used_by
			FROM invite_keys ORDER BY id DESC LIMIT 100`)
		if err != nil {
			log.Fatal(err)
		}
		defer rows.Close()
		fmt.Println("ID\tUSES\tEXPIRES\tSTATUS\tLABEL\tLAST_USED_BY")
		now := time.Now().Unix()
		for rows.Next() {
			var id, maxUses, uses int
			var expiresAt, revokedAt int64
			var label, lastUsedBy string
			rows.Scan(&id, &label, &maxUses, &uses, &expiresAt, &revokedAt, &lastUsedBy)
			status := "active"
			if revokedAt > 0 {
				status = "revoked"
			} else if expiresAt > 0 && expiresAt <= now {
				status = "expired"
			} else if maxUses > 0 && uses >= maxUses {
				status = "used"
			}
			expires := "never"
			if expiresAt > 0 {
				expires = time.Unix(expiresAt, 0).Format("2006-01-02")
			}
			fmt.Printf("%d\t%d/%d\t%s\t%s\t%s\t%s\n", id, uses, maxUses, expires, status, label, lastUsedBy)
		}
	case "revoke":
		if len(args) < 2 {
			log.Fatal("revoke requires a key or hash")
		}
		hash := strings.TrimSpace(args[1])
		if strings.HasPrefix(hash, "kxxx_") {
			hash = inviteKeyHash(hash)
		}
		res, err := db.Exec(`UPDATE invite_keys SET revoked_at = ? WHERE key_hash = ?`, time.Now().Unix(), hash)
		if err != nil {
			log.Fatal(err)
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			log.Fatal("invite key not found")
		}
		fmt.Println("Invite revoked")
	default:
		printInviteUsage()
	}
}

func hashPassword(password string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("bcrypt hash failed: %v", err)
		return ""
	}
	return string(hash)
}

func checkPassword(password, stored string) bool {
	if stored == "" {
		return false
	}
	if strings.HasPrefix(stored, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)) == nil
	}
	parts := strings.SplitN(stored, ":", 2)
	if len(parts) != 2 {
		return false
	}
	salt, hash := parts[0], parts[1]
	h := sha256.Sum256([]byte(salt + password))
	return hex.EncodeToString(h[:]) == hash
}

func createToken(userID int, username string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	now := time.Now()
	payloadJSON, err := json.Marshal(jwtPayload{
		UID: userID,
		UN:  username,
		Exp: now.Add(30 * 24 * time.Hour).Unix(),
		Iat: now.Unix(),
	})
	if err != nil {
		log.Printf("JWT: payload marshal error: %v", err)
		return ""
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := header + "." + payloadB64
	mac := hmac.New(sha256.New, []byte(jwtSecret))
	mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sig
}

const authCookieName = "kxxx_token"

func setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func writeAuthResponse(w http.ResponseWriter, token string, userID int, username string) {
	setAuthCookie(w, token)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token": token,
		"user": map[string]interface{}{
			"id":       userID,
			"username": username,
		},
	})
}

func parseToken(token string) (int, string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		log.Printf("JWT: wrong parts count: %d", len(parts))
		return 0, "", false
	}
	header, payload, sig := parts[0], parts[1], parts[2]
	signingInput := header + "." + payload
	mac := hmac.New(sha256.New, []byte(jwtSecret))
	mac.Write([]byte(signingInput))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if sig != expectedSig {
		gotPreview := sig
		if len(gotPreview) > 10 {
			gotPreview = gotPreview[:10]
		}
		wantPreview := expectedSig
		if len(wantPreview) > 10 {
			wantPreview = wantPreview[:10]
		}
		log.Printf("JWT: signature mismatch: got=%q want=%q", gotPreview, wantPreview)
		return 0, "", false
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		log.Printf("JWT: payload decode error: %v", err)
		return 0, "", false
	}
	var claims jwtPayload
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		log.Printf("JWT: claims parse error: %v, payload=%s", err, string(payloadJSON))
		return 0, "", false
	}
	if time.Now().Unix() > claims.Exp {
		return 0, "", false
	}
	return claims.UID, claims.UN, true
}

func authFromRequest(r *http.Request) (int, string, bool) {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return parseToken(strings.TrimPrefix(auth, "Bearer "))
	}
	if cookie, err := r.Cookie(authCookieName); err == nil && cookie.Value != "" {
		return parseToken(cookie.Value)
	}
	return 0, "", false
}

func authMiddleware(w http.ResponseWriter, r *http.Request) (int, string, bool) {
	return authFromRequest(r)
}

func normalizeUsername(username string) string {
	return strings.TrimSpace(username)
}

func validUsername(username string) bool {
	if len(username) < 3 || len(username) > 32 {
		return false
	}
	for _, r := range username {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func registerUserWithInvite(username, password, inviteKey string) (int64, error) {
	inviteKey = strings.TrimSpace(inviteKey)
	if inviteKey == "" {
		return 0, fmt.Errorf("invite key required")
	}
	passwordHash := hashPassword(password)
	if passwordHash == "" {
		return 0, fmt.Errorf("could not hash password")
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`UPDATE invite_keys
		SET uses = uses + 1, last_used_at = ?, last_used_by = ?
		WHERE key_hash = ?
		  AND COALESCE(revoked_at, 0) = 0
		  AND (COALESCE(expires_at, 0) = 0 OR expires_at > ?)
		  AND (COALESCE(max_uses, 1) = 0 OR uses < max_uses)`,
		time.Now().Unix(), username, inviteKeyHash(inviteKey), time.Now().Unix())
	if err != nil {
		return 0, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return 0, fmt.Errorf("invalid or expired invite key")
	}

	userRes, err := tx.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", username, passwordHash)
	if err != nil {
		return 0, fmt.Errorf("username taken")
	}
	id, _ := userRes.LastInsertId()
	if _, err := tx.Exec("INSERT OR IGNORE INTO user_profiles (user_id, anonymous_name) VALUES (?, ?)", id, createAnonymousName()); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSONError(w, 405, "POST only")
		return
	}
	ip := clientIP(r)
	if isRateLimited(&registerMu, registerIPs, ip, 5) {
		w.Header().Set("Retry-After", "900")
		writeJSONError(w, 429, "too many registrations, try again in 15 minutes")
		return
	}
	recordAttempt(&registerMu, registerIPs, ip, 15*time.Minute)
	var body struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		InviteKey string `json:"invite_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, 400, "invalid body")
		return
	}
	body.Username = normalizeUsername(body.Username)
	if body.Username == "" || body.Password == "" {
		writeJSONError(w, 400, "username and password required")
		return
	}
	if !validUsername(body.Username) {
		writeJSONError(w, 400, "username must be 3-32 letters, numbers, dashes, or underscores")
		return
	}
	if len(body.Password) < 4 {
		writeJSONError(w, 400, "password too short")
		return
	}
	id, err := registerUserWithInvite(body.Username, body.Password, body.InviteKey)
	if err != nil {
		writeJSONError(w, 400, err.Error())
		return
	}
	token := createToken(int(id), body.Username)
	writeAuthResponse(w, token, int(id), body.Username)
}

func handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSONError(w, 405, "POST only")
		return
	}
	ip := clientIP(r)
	if isRateLimited(&loginMu, loginAttempts, ip, 5) {
		w.Header().Set("Retry-After", "900")
		writeJSONError(w, 429, "too many attempts, try again in 15 minutes")
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, 400, "invalid body")
		return
	}
	body.Username = normalizeUsername(body.Username)
	var id int
	var hash string
	err := db.QueryRow("SELECT id, password_hash FROM users WHERE username = ?", body.Username).Scan(&id, &hash)
	if err != nil || !checkPassword(body.Password, hash) {
		recordAttempt(&loginMu, loginAttempts, ip, 15*time.Minute)
		writeJSONError(w, 401, "invalid credentials")
		return
	}
	if !strings.HasPrefix(hash, "$2") {
		if newHash := hashPassword(body.Password); newHash != "" {
			if _, err := db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", newHash, id); err != nil {
				log.Printf("password hash migration failed for user %d: %v", id, err)
			} else {
				log.Printf("migrated legacy password hash for user %d", id)
			}
		}
	}
	clearAttempts(&loginMu, loginAttempts, ip)
	token := createToken(id, body.Username)
	writeAuthResponse(w, token, id, body.Username)
}

func handleAuthMe(w http.ResponseWriter, r *http.Request) {
	uid, un, ok := authMiddleware(w, r)
	if !ok {
		writeJSONError(w, 401, "unauthorized")
		return
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		setAuthCookie(w, strings.TrimPrefix(auth, "Bearer "))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"id": uid, "username": un})
}

func handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	clearAuthCookie(w)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func handleFavVideo(w http.ResponseWriter, r *http.Request) {
	uid, _, ok := authMiddleware(w, r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	videoID := strings.TrimPrefix(r.URL.Path, "/api/fav/video/")
	if videoID == "" {
		http.Error(w, `{"error":"missing video id"}`, 400)
		return
	}
	if r.Method == "POST" {
		db.Exec("INSERT OR IGNORE INTO favorites (user_id, video_id) VALUES (?, ?)", uid, videoID)
		json.NewEncoder(w).Encode(map[string]bool{"favorited": true})
	} else if r.Method == "DELETE" {
		db.Exec("DELETE FROM favorites WHERE user_id = ? AND video_id = ?", uid, videoID)
		json.NewEncoder(w).Encode(map[string]bool{"favorited": false})
	} else {
		var count int
		db.QueryRow("SELECT COUNT(*) FROM favorites WHERE user_id = ? AND video_id = ?", uid, videoID).Scan(&count)
		json.NewEncoder(w).Encode(map[string]bool{"favorited": count > 0})
	}
}

func handleFavVideos(w http.ResponseWriter, r *http.Request) {
	uid, _, ok := authMiddleware(w, r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	sort := r.URL.Query().Get("sort")
	orderBy := favSortOrderBy(sort)
	rows, err := db.Query("SELECT f.video_id FROM favorites f JOIN videos v ON v.id = f.video_id WHERE f.user_id = ? ORDER BY "+orderBy, uid)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	json.NewEncoder(w).Encode(ids)
}

func favSortOrderBy(sort string) string {
	switch sort {
	case "views":
		return "v.views DESC"
	case "duration":
		return "v.duration DESC"
	case "title":
		return "v.title COLLATE NOCASE ASC"
	default:
		return "f.created_at DESC"
	}
}

func handleFavCategory(w http.ResponseWriter, r *http.Request) {
	uid, _, ok := authMiddleware(w, r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	cat := r.URL.Query().Get("cat")
	if cat == "" {
		http.Error(w, `{"error":"missing cat"}`, 400)
		return
	}
	if r.Method == "POST" {
		db.Exec("INSERT OR IGNORE INTO fav_categories (user_id, category) VALUES (?, ?)", uid, cat)
		json.NewEncoder(w).Encode(map[string]bool{"favorited": true})
	} else if r.Method == "DELETE" {
		db.Exec("DELETE FROM fav_categories WHERE user_id = ? AND category = ?", uid, cat)
		json.NewEncoder(w).Encode(map[string]bool{"favorited": false})
	}
}

func handleFavCategories(w http.ResponseWriter, r *http.Request) {
	uid, _, ok := authMiddleware(w, r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	rows, err := db.Query("SELECT category FROM fav_categories WHERE user_id = ?", uid)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	cats := []string{}
	for rows.Next() {
		var c string
		rows.Scan(&c)
		cats = append(cats, c)
	}
	json.NewEncoder(w).Encode(cats)
}

func publicRequestPath(path string) bool {
	switch path {
	case "/api/auth/login", "/api/auth/register", "/api/auth/me", "/api/auth/logout", "/api/health", "/api/reclassify":
		return true
	}
	return false
}

func protectedRequestPath(path string) bool {
	return strings.HasPrefix(path, "/api/") ||
		strings.HasPrefix(path, "/vid/") ||
		strings.HasPrefix(path, "/thumb/") ||
		path == "/media"
}

func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive, nosnippet, noimageindex")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; media-src *; img-src *; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		if protectedRequestPath(r.URL.Path) && !publicRequestPath(r.URL.Path) {
			if _, _, ok := authFromRequest(r); !ok {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func initRoutes() {
	http.HandleFunc("/thumb/", handleThumbProxy)
	http.HandleFunc("/media", handleMediaProxy)

	// If web/dist exists, serve the React frontend at root
	if _, err := os.Stat("web/dist/index.html"); err == nil {
		fs := http.FileServer(http.Dir("web/dist"))
		http.HandleFunc("/vid/", handleVidProxy)
		http.Handle("/assets/", fs)
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				routeAPI(w, r)
				return
			}
			// Serve static files from dist if they exist
			filePath := "web/dist" + r.URL.Path
			if r.URL.Path != "/" && r.URL.Path != "" {
				if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
					http.ServeFile(w, r, filePath)
					return
				}
			}
			// Fall through to SPA routing
			http.ServeFile(w, r, "web/dist/index.html")
		})
		routeAPI = func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/api/search":
				handleAPISearch(w, r)
			case r.URL.Path == "/api/search-suggest":
				handleSearchSuggest(w, r)
			case r.URL.Path == "/api/crawl":
				handleAPICrawl(w, r)
			case r.URL.Path == "/api/crawl-xv":
				handleAPICrawlXv(w, r)
			case r.URL.Path == "/api/crawl-xh":
				handleAPICrawlXh(w, r)
			case r.URL.Path == "/api/crawl-ep":
				handleAPICrawlEp(w, r)
			case r.URL.Path == "/api/crawl-tf":
				handleAPICrawlTf(w, r)
			case r.URL.Path == "/api/crawl-dt":
				handleAPICrawlDt(w, r)
			case r.URL.Path == "/api/crawl-kvs":
				handleAPICrawlKVS(w, r)
			case r.URL.Path == "/api/categories":
				handleAPICategories(w, r)
			case r.URL.Path == "/api/browse":
				handleAPIBrowse(w, r)
			case strings.HasPrefix(r.URL.Path, "/api/video/"):
				handleAPIVideo(w, r)
			case r.URL.Path == "/api/refresh":
				handleAPIRefresh(w, r)
			case r.URL.Path == "/api/reclassify":
				handleAPIRclassify(w, r)
			case r.URL.Path == "/api/status":
				handleStatusSSE(w, r)
			case r.URL.Path == "/api/changelog":
				handleAPIChangelog(w, r)
			case r.URL.Path == "/api/auth/register":
				handleAuthRegister(w, r)
			case r.URL.Path == "/api/auth/login":
				handleAuthLogin(w, r)
			case r.URL.Path == "/api/auth/me":
				handleAuthMe(w, r)
			case r.URL.Path == "/api/auth/logout":
				handleAuthLogout(w, r)
			case strings.HasPrefix(r.URL.Path, "/api/fav/video/"):
				handleFavVideo(w, r)
			case r.URL.Path == "/api/fav/videos":
				handleFavVideos(w, r)
			case r.URL.Path == "/api/health":
				handleHealth(w, r)
			case r.URL.Path == "/api/random":
				handleAPIRandom(w, r)
			case strings.HasPrefix(r.URL.Path, "/api/related/"):
				handleAPIRelated(w, r)
			case r.URL.Path == "/api/tags":
				handleAPITags(w, r)
			case strings.HasPrefix(r.URL.Path, "/api/watch/"):
				handleWatchRouter(w, r)
			case r.URL.Path == "/api/history/clear":
				handleHistoryClear(w, r)
			case r.URL.Path == "/api/watch-later":
				handleWatchLaterList(w, r)
			case strings.HasPrefix(r.URL.Path, "/api/watch-later/"):
				handleWatchLaterRouter(w, r)
			case r.URL.Path == "/api/playlists":
				handlePlaylistListCreate(w, r)
			case strings.HasPrefix(r.URL.Path, "/api/playlists/"):
				handlePlaylistRouter(w, r)
			case strings.HasPrefix(r.URL.Path, "/api/rate/"):
				handleRateVideo(w, r)
			case r.URL.Path == "/api/for-you":
				handleForYou(w, r)
			case r.URL.Path == "/api/suggestions":
				handleSuggestions(w, r)
			case r.URL.Path == "/api/profile":
				handleProfile(w, r)
			case r.URL.Path == "/api/profile/settings":
				handleProfileSettings(w, r)
			case strings.HasPrefix(r.URL.Path, "/api/social/video/"):
				handleVideoSocialRouter(w, r)
			case strings.HasPrefix(r.URL.Path, "/api/wall/"):
				handleWallRouter(w, r)
			case strings.HasPrefix(r.URL.Path, "/api/fav/category"):
				handleFavCategory(w, r)
			case r.URL.Path == "/api/fav/categories":
				handleFavCategories(w, r)
			default:
				http.NotFound(w, r)
			}
		}
		return
	}

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/page/", handleIndex)
	http.HandleFunc("/search", handleSearch)
	http.HandleFunc("/tag/", handleTagPage)
	http.HandleFunc("/uploader/", handleUploaderPage)
	http.HandleFunc("/play/", handlePlay)
	http.HandleFunc("/vid/", handleVidProxy)
	http.HandleFunc("/thumb/", handleThumbProxy)
	http.HandleFunc("/api/search", handleAPISearch)
	http.HandleFunc("/api/search-suggest", handleSearchSuggest)
	http.HandleFunc("/api/crawl", handleAPICrawl)
	http.HandleFunc("/api/crawl-xv", handleAPICrawlXv)
	http.HandleFunc("/api/crawl-xh", handleAPICrawlXh)
	http.HandleFunc("/api/crawl-ep", handleAPICrawlEp)
	http.HandleFunc("/api/crawl-tf", handleAPICrawlTf)
	http.HandleFunc("/api/crawl-dt", handleAPICrawlDt)
	http.HandleFunc("/api/crawl-kvs", handleAPICrawlKVS)
	http.HandleFunc("/api/categories", handleAPICategories)
	http.HandleFunc("/api/browse", handleAPIBrowse)
	http.HandleFunc("/api/video/", handleAPIVideo)
	http.HandleFunc("/api/refresh", handleAPIRefresh)
	http.HandleFunc("/api/reclassify", handleAPIRclassify)
	http.HandleFunc("/api/status", handleStatusSSE)
	http.HandleFunc("/api/changelog", handleAPIChangelog)
	http.HandleFunc("/api/auth/register", handleAuthRegister)
	http.HandleFunc("/api/auth/login", handleAuthLogin)
	http.HandleFunc("/api/auth/me", handleAuthMe)
	http.HandleFunc("/api/auth/logout", handleAuthLogout)
	http.HandleFunc("/api/fav/video/", handleFavVideo)
	http.HandleFunc("/api/fav/videos", handleFavVideos)
	http.HandleFunc("/api/fav/category", handleFavCategory)
	http.HandleFunc("/api/fav/categories", handleFavCategories)
	http.HandleFunc("/api/health", handleHealth)
	http.HandleFunc("/api/random", handleAPIRandom)
	http.HandleFunc("/api/related/", handleAPIRelated)
	http.HandleFunc("/api/tags", handleAPITags)
	http.HandleFunc("/api/watch/", handleWatchRouter)
	http.HandleFunc("/api/history/clear", handleHistoryClear)
	http.HandleFunc("/api/watch-later", handleWatchLaterList)
	http.HandleFunc("/api/watch-later/", handleWatchLaterRouter)
	http.HandleFunc("/api/playlists", handlePlaylistListCreate)
	http.HandleFunc("/api/playlists/", handlePlaylistRouter)
	http.HandleFunc("/api/rate/", handleRateVideo)
	http.HandleFunc("/api/for-you", handleForYou)
	http.HandleFunc("/api/suggestions", handleSuggestions)
	http.HandleFunc("/api/profile", handleProfile)
	http.HandleFunc("/api/profile/settings", handleProfileSettings)
	http.HandleFunc("/api/social/video/", handleVideoSocialRouter)
	http.HandleFunc("/api/wall/", handleWallRouter)
}

// --- Background refresh every 20 min ---

func refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(refreshEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			log.Println("Starting 20-minute refresh cycle for expiring tokens...")
			refreshExpiring()
		}
	}
}

func backgroundBackfillLoop(ctx context.Context) {
	ticker := time.NewTicker(backfillEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			scrapeNewVideoDetails()
		}
	}
}

func refreshExpiring() {
	now := time.Now().Unix()
	cutoff := time.Now().Add(tokenRefreshLead).Unix()
	rows, err := db.Query(`SELECT id FROM videos
		WHERE expires_at > ?
		  AND expires_at < ?
		  AND COALESCE(source, 'xnxx') IN ('xnxx', 'xhamster')
		  AND (COALESCE(url_360,'') <> '' OR COALESCE(url_720,'') <> '' OR COALESCE(url_1080,'') <> '' OR COALESCE(hls_url,'') <> '')
		ORDER BY expires_at ASC LIMIT ?`, now, cutoff, expiringRefreshBatch)
	if err != nil {
		log.Printf("Refresh query failed: %v", err)
		return
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		log.Println("No videos need token refresh")
		return
	}

	log.Printf("Refreshing %d expiring videos...", len(ids))
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		scrapeSem <- struct{}{}
		go func(vid string) {
			defer wg.Done()
			defer func() { <-scrapeSem }()
			time.Sleep(time.Duration(rand.Intn(1500)) * time.Millisecond)
			if v, ok := loadVideoFromDB(vid); ok {
				if refreshed, err := ensureFreshVideo(v, tokenRefreshLead); err == nil {
					log.Printf("Pre-warmed media token for %s (%s)", refreshed.ID, refreshed.Source)
				} else {
					log.Printf("Pre-warm failed for %s: %v", vid, err)
				}
			} else {
				clearScrapeFailure(vid)
			}
		}(id)
	}
	wg.Wait()
	log.Println("Refresh cycle complete")
}

func isPlayableSource(source string) bool {
	switch source {
	case "", "xnxx", "xhamster", "xvideos":
		return true
	default:
		return isKVSSource(source)
	}
}

func crawlLoop(ctx context.Context) {
	ticker := time.NewTicker(crawlEvery)
	defer ticker.Stop()

	// Initial sleep to let the app start up before crawling
	select {
	case <-time.After(60 * time.Second):
	case <-ctx.Done():
		return
	}

	for {
		log.Println("=== Starting automated crawl cycle (parallel) ===")

		var wg sync.WaitGroup
		wg.Add(7)
		go func() { defer wg.Done(); runFullCrawl() }()
		go func() { defer wg.Done(); runXvCrawl() }()
		go func() { defer wg.Done(); runXhCrawl() }()
		go func() { defer wg.Done(); runEpCrawl() }()
		go func() { defer wg.Done(); runTfCrawl() }()
		go func() { defer wg.Done(); runDtCrawl() }()
		go func() { defer wg.Done(); runKVSCrawl() }()
		wg.Wait()

		log.Println("=== Automated crawl cycle complete ===")

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// --- Progress / SSE ---

func setProgress(source, status string, scanned, newVideos, cached, detailDone, detailTotal, page int) {
	if status == "" || (status == "scraping" && detailTotal <= 0) {
		status = "idle"
		detailDone = 0
		detailTotal = 0
	}
	progress.mu.Lock()
	progress.Status = status
	progress.Source = source
	progress.Scanned = scanned
	progress.NewVideos = newVideos
	progress.Cached = cached
	progress.DetailDone = detailDone
	progress.DetailTotal = detailTotal
	progress.Page = page
	progress.mu.Unlock()
}

// progressSnapshot is a mutex-free copy of CrawlProgress fields for JSON encoding.
// Copying CrawlProgress directly copies the sync.RWMutex, which go vet flags and is undefined behavior.
type progressSnapshot struct {
	Status       string         `json:"status"`
	Source       string         `json:"source"`
	Scanned      int            `json:"scanned"`
	NewVideos    int            `json:"new_videos"`
	Cached       int            `json:"cached"`
	DetailDone   int            `json:"detail_done"`
	DetailTotal  int            `json:"detail_total"`
	Page         int            `json:"page"`
	TotalCount   int            `json:"total_count"`
	SourceCounts map[string]int `json:"source_counts"`
}

func getProgressJSON() []byte {
	progress.mu.RLock()
	p := progressSnapshot{
		Status:      progress.Status,
		Source:      progress.Source,
		Scanned:     progress.Scanned,
		NewVideos:   progress.NewVideos,
		Cached:      progress.Cached,
		DetailDone:  progress.DetailDone,
		DetailTotal: progress.DetailTotal,
		Page:        progress.Page,
	}
	progress.mu.RUnlock()
	if p.Status == "" || (p.Status == "scraping" && p.DetailTotal <= 0) {
		p.Status = "idle"
		p.DetailDone = 0
		p.DetailTotal = 0
	}
	p.TotalCount = cachedCount("videos_total", "SELECT COUNT(*) FROM videos")
	p.SourceCounts = map[string]int{}
	rows, err := db.Query("SELECT source, COUNT(*) FROM videos GROUP BY source")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var src string
			var count int
			if err := rows.Scan(&src, &count); err != nil {
				log.Printf("progress source count scan failed: %v", err)
				continue
			}
			p.SourceCounts[src] = count
		}
	}
	data, _ := json.Marshal(p)
	return data
}

func handleAPIRclassify(w http.ResponseWriter, r *http.Request) {
	go func() {
		log.Println("Reclassifying all videos...")
		rows, err := db.Query("SELECT id, title, description, categories, tags FROM videos")
		if err != nil {
			log.Printf("Reclassify query failed: %v", err)
			return
		}
		defer rows.Close()

		updated := 0
		for rows.Next() {
			var id, title, desc, catsStr, tagsStr string
			rows.Scan(&id, &title, &desc, &catsStr, &tagsStr)
			tags := strings.Split(tagsStr, ",")
			mergedCats := mergeCategoryLists(strings.Split(catsStr, ","), tags, extractCategories(title, desc, tags))
			newCats := joinStoredCategories(mergedCats)
			if newCats != catsStr {
				if _, err := db.Exec("UPDATE videos SET categories = ? WHERE id = ?", newCats, id); err != nil {
					log.Printf("Reclassify update failed for %s: %v", id, err)
					continue
				}
				if err := replaceVideoCategories(id, mergedCats); err != nil {
					log.Printf("Reclassify junction sync failed for %s: %v", id, err)
					continue
				}
				updated++
			}
		}
		log.Printf("Reclassification complete: %d videos updated", updated)
		refreshCatCache()
	}()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func handleStatusSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	// Send initial state
	fmt.Fprintf(w, "data: %s\n\n", getProgressJSON())
	flusher.Flush()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, "data: %s\n\n", getProgressJSON())
			flusher.Flush()
		}
	}
}

// --- Handlers ---

const perPage = 72

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")

	page := 1
	if p := strings.TrimPrefix(r.URL.Path, "/page/"); p != r.URL.Path {
		fmt.Sscanf(p, "%d", &page)
		if page < 1 {
			page = 1
		}
	}

	sort := r.URL.Query().Get("sort")
	cat := r.URL.Query().Get("cat")

	order := "added_at DESC"
	switch sort {
	case "new":
		order = "upload_date DESC"
	case "views":
		order = "views DESC"
	case "duration":
		order = "duration DESC"
	}

	var rows *sql.Rows
	var err error
	cats := parseCategoryFilter(cat)
	if len(cats) == 1 {
		rows, err = db.Query(
			"SELECT videos.id, videos.slug, videos.title, videos.description, videos.categories, videos.duration, videos.views, videos.thumb_uuid, videos.preview_url, videos.added_at, videos.upload_date, videos.source FROM videos JOIN video_categories vc ON vc.video_id = videos.id WHERE vc.category = ? ORDER BY "+order+" LIMIT ? OFFSET ?",
			cats[0], perPage, (page-1)*perPage)
	} else if len(cats) >= 2 {
		// Multiple categories use AND semantics: every returned video must match all requested categories.
		placeholders := make([]string, len(cats))
		args := make([]any, 0, len(cats)+3)
		for i, cat := range cats {
			placeholders[i] = "?"
			args = append(args, cat)
		}
		args = append(args, len(cats), perPage, (page-1)*perPage)
		rows, err = db.Query(
			"SELECT videos.id, videos.slug, videos.title, videos.description, videos.categories, videos.duration, videos.views, videos.thumb_uuid, videos.preview_url, videos.added_at, videos.upload_date, videos.source FROM videos JOIN video_categories vc ON vc.video_id = videos.id WHERE vc.category IN ("+strings.Join(placeholders, ",")+") GROUP BY videos.id HAVING COUNT(DISTINCT vc.category) = ? ORDER BY "+order+" LIMIT ? OFFSET ?",
			args...)
	} else {
		rows, err = db.Query(
			"SELECT id, slug, title, description, categories, duration, views, thumb_uuid, preview_url, added_at, upload_date, source FROM videos ORDER BY "+order+" LIMIT ? OFFSET ?",
			perPage, (page-1)*perPage)
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	videos := []Video{}
	for rows.Next() {
		v := Video{}
		var dur, views sql.NullInt64
		var cats, uploadDate sql.NullString
		if err := rows.Scan(&v.ID, &v.Slug, &v.Title, &v.Description, &cats, &dur, &views, &v.ThumbUUID, &v.PreviewURL, &v.AddedAt, &uploadDate, &v.Source); err != nil {
			log.Printf("uploader page row scan failed: %v", err)
			continue
		}
		v.Duration = int(dur.Int64)
		v.Views = int(views.Int64)
		if cats.Valid && cats.String != "" {
			v.Categories = strings.Split(cats.String, ",")
		}
		if uploadDate.Valid {
			v.UploadDate = uploadDate.String
		}
		videos = append(videos, v)
	}

	var count int
	if len(cats) == 1 {
		count = cachedCount("cat_count:"+strings.Join(cats, ","), "SELECT COUNT(*) FROM videos JOIN video_categories vc ON vc.video_id = videos.id WHERE vc.category = ?", cats[0])
	} else if len(cats) >= 2 {
		placeholders := make([]string, len(cats))
		args := make([]any, 0, len(cats)+1)
		for i, cat := range cats {
			placeholders[i] = "?"
			args = append(args, cat)
		}
		args = append(args, len(cats))
		count = cachedCount("cat_count:"+strings.Join(cats, ","), "SELECT COUNT(*) FROM (SELECT videos.id FROM videos JOIN video_categories vc ON vc.video_id = videos.id WHERE vc.category IN ("+strings.Join(placeholders, ",")+") GROUP BY videos.id HAVING COUNT(DISTINCT vc.category) = ?)", args...)
	} else {
		count = cachedCount("videos_total", "SELECT COUNT(*) FROM videos")
	}

	totalPages := (count + perPage - 1) / perPage

	data := map[string]interface{}{
		"Videos":     videos,
		"Count":      count,
		"Page":       page,
		"TotalPages": totalPages,
		"Sort":       sort,
		"Cat":        cat,
	}
	tmpl.ExecuteTemplate(w, "index.html", data)
}

func handleTagPage(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimPrefix(r.URL.Path, "/tag/")
	tag = strings.TrimSuffix(tag, "/")
	if tag == "" {
		http.Redirect(w, r, "/", 302)
		return
	}
	http.Redirect(w, r, "/search?q="+url.QueryEscape(tag), 302)
}

func handleUploaderPage(w http.ResponseWriter, r *http.Request) {
	uploader := strings.TrimPrefix(r.URL.Path, "/uploader/")
	uploader = strings.TrimSuffix(uploader, "/")
	if uploader == "" {
		http.Redirect(w, r, "/", 302)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=120")
	rows, err := db.Query(
		"SELECT id, slug, title, description, categories, duration, views, thumb_uuid, preview_url, added_at, upload_date, source FROM videos WHERE uploader = ? ORDER BY views DESC LIMIT 72",
		uploader)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	videos := []Video{}
	for rows.Next() {
		v := Video{}
		var dur, views sql.NullInt64
		var cats, uploadDate sql.NullString
		rows.Scan(&v.ID, &v.Slug, &v.Title, &v.Description, &cats, &dur, &views, &v.ThumbUUID, &v.PreviewURL, &v.AddedAt, &uploadDate, &v.Source)
		v.Duration = int(dur.Int64)
		v.Views = int(views.Int64)
		if cats.Valid && cats.String != "" {
			v.Categories = strings.Split(cats.String, ",")
		}
		if uploadDate.Valid {
			v.UploadDate = uploadDate.String
		}
		videos = append(videos, v)
	}

	data := map[string]interface{}{
		"Videos":   videos,
		"Count":    len(videos),
		"Uploader": uploader,
	}
	tmpl.ExecuteTemplate(w, "index.html", data)
}

func refreshCatCache() {
	catCache.mu.Lock()
	defer catCache.mu.Unlock()
	if time.Since(catCache.last) < 30*time.Second {
		return
	}
	rows, err := db.Query(`SELECT vc.category FROM video_categories vc JOIN videos v ON v.id = vc.video_id WHERE ` + playableMediaSQLV + ` GROUP BY vc.category ORDER BY COUNT(*) DESC`)
	if err != nil {
		return
	}
	defer rows.Close()
	catCache.cats = nil
	for rows.Next() {
		var category string
		if err := rows.Scan(&category); err != nil {
			continue
		}
		category = normalizeCategoryTerm(category)
		if category != "" {
			catCache.cats = append(catCache.cats, category)
		}
	}
	catCache.last = time.Now()
}

func handleAPICategories(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	catCache.mu.RLock()
	cats := catCache.cats
	catCache.mu.RUnlock()
	if cats == nil {
		refreshCatCache()
		catCache.mu.RLock()
		cats = catCache.cats
		catCache.mu.RUnlock()
	}
	if cats == nil {
		cats = []string{}
	}
	json.NewEncoder(w).Encode(cats)
}

func handleAPIBrowse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")

	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		fmt.Sscanf(p, "%d", &page)
		if page < 1 {
			page = 1
		}
	}

	sort := r.URL.Query().Get("sort")
	cat := r.URL.Query().Get("cat")
	q := r.URL.Query().Get("q")
	uploader := r.URL.Query().Get("uploader")
	source := r.URL.Query().Get("source")

	validSorts := map[string]string{
		"recent":   "v.added_at DESC",
		"new":      "v.upload_date DESC",
		"views":    "v.views DESC",
		"duration": "v.duration DESC",
		"trending": "(CAST(v.views AS REAL) / MAX(1.0, julianday('now') - julianday(v.added_at))) DESC",
	}
	orderBy := "v.added_at DESC"
	if o, ok := validSorts[sort]; ok {
		orderBy = o
	}
	whereClauses := []string{playableMediaSQLV}
	var args []interface{}
	// Categories support comma-separated AND/intersection: a video must match
	// every requested category (e.g. cat=anal,milf → tagged both).
	catList := parseCategoryFilter(cat)
	if len(catList) == 1 {
		whereClauses = append(whereClauses, "v.id IN (SELECT video_id FROM video_categories WHERE category = ?)")
		args = append(args, catList[0])
	} else if len(catList) >= 2 {
		ph := make([]string, len(catList))
		for i, c := range catList {
			ph[i] = "?"
			args = append(args, c)
		}
		whereClauses = append(whereClauses, "v.id IN (SELECT video_id FROM video_categories WHERE category IN ("+strings.Join(ph, ",")+") GROUP BY video_id HAVING COUNT(DISTINCT category) = ?)")
		args = append(args, len(catList))
	}
	if uploader != "" {
		whereClauses = append(whereClauses, "v.uploader = ?")
		args = append(args, uploader)
	}
	if source != "" {
		whereClauses = append(whereClauses, "v.source = ?")
		args = append(args, source)
	}
	where := ""
	if len(whereClauses) > 0 {
		where = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	videos := []Video{}

	if q != "" {
		sanitized := sanitizeFTSQuery(q)
		if sanitized != "" {
			ftsWhere := where
			if ftsWhere == "" {
				ftsWhere = " WHERE videos_fts MATCH ?"
			} else {
				ftsWhere = " WHERE videos_fts MATCH ? AND " + strings.Join(whereClauses, " AND ")
			}
			ftsArgs := []interface{}{sanitized}
			ftsArgs = append(ftsArgs, args...)
			rows, err := db.Query(
				`SELECT v.id, COALESCE(v.slug,''), COALESCE(v.title,''), COALESCE(v.description,''), v.categories, v.duration, v.views, COALESCE(v.thumb_uuid,''), COALESCE(v.preview_url,''), COALESCE(v.added_at,''), v.upload_date, COALESCE(v.source,'xnxx'), COALESCE(wc.watch_count, 0)
					 FROM videos_fts f JOIN videos v ON v.rowid = f.rowid LEFT JOIN video_watch_counts wc ON wc.video_id = v.id`+ftsWhere+` ORDER BY rank LIMIT ? OFFSET ?`,
				append(ftsArgs, perPage, (page-1)*perPage)...)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					vv := Video{}
					var dur, views sql.NullInt64
					var cats, uploadDate sql.NullString
					rows.Scan(&vv.ID, &vv.Slug, &vv.Title, &vv.Description, &cats, &dur, &views, &vv.ThumbUUID, &vv.PreviewURL, &vv.AddedAt, &uploadDate, &vv.Source, &vv.WatchCount)
					vv.Duration = int(dur.Int64)
					vv.Views = int(views.Int64)
					if cats.Valid && cats.String != "" {
						vv.Categories = strings.Split(cats.String, ",")
					}
					if uploadDate.Valid {
						vv.UploadDate = uploadDate.String
					}
					videos = append(videos, vv)
				}
			}
		}
	} else {
		// COALESCE everything nullable: stub rows (tnaflix/drtuber) leave columns
		// NULL, and a NULL aborts rows.Scan mid-row → blank cards in the UI.
		query := `SELECT v.id, COALESCE(v.slug,''), COALESCE(v.title,''), COALESCE(v.description,''), v.categories, v.duration, v.views, COALESCE(v.thumb_uuid,''), COALESCE(v.preview_url,''), COALESCE(v.added_at,''), v.upload_date, COALESCE(v.source,'xnxx'), COALESCE(wc.watch_count, 0) FROM videos v LEFT JOIN video_watch_counts wc ON wc.video_id = v.id` + where + ` ORDER BY ` + orderBy + ` LIMIT ? OFFSET ?`
		rows, err := db.Query(query, append(args, perPage, (page-1)*perPage)...)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		for rows.Next() {
			vv := Video{}
			var dur, views sql.NullInt64
			var cats, uploadDate sql.NullString
			rows.Scan(&vv.ID, &vv.Slug, &vv.Title, &vv.Description, &cats, &dur, &views, &vv.ThumbUUID, &vv.PreviewURL, &vv.AddedAt, &uploadDate, &vv.Source, &vv.WatchCount)
			vv.Duration = int(dur.Int64)
			vv.Views = int(views.Int64)
			if cats.Valid && cats.String != "" {
				vv.Categories = strings.Split(cats.String, ",")
			}
			if uploadDate.Valid {
				vv.UploadDate = uploadDate.String
			}
			videos = append(videos, vv)
		}
	}

	var count int
	if q != "" {
		countQuery := `SELECT COUNT(*) FROM videos_fts f JOIN videos v ON v.rowid = f.rowid`
		ftsWhere := " WHERE videos_fts MATCH ?"
		if len(whereClauses) > 0 {
			ftsWhere += " AND " + strings.Join(whereClauses, " AND ")
		}
		var cnt int
		if err := db.QueryRow(countQuery+ftsWhere, append([]interface{}{sanitizeFTSQuery(q)}, args...)...).Scan(&cnt); err != nil {
			log.Printf("browse FTS count scan failed for %q: %v", q, err)
		}
		count = cnt
		if count == 0 {
			count = len(videos)
		}
	} else {
		count = cachedCount("browse_count:"+where, "SELECT COUNT(*) FROM videos v"+where, args...)
	}

	totalPages := 1
	if count > 0 {
		totalPages = (count + perPage - 1) / perPage
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"videos":      videos,
		"count":       count,
		"page":        page,
		"total_pages": totalPages,
	})
}

func handleAPIVideo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")

	id := strings.TrimPrefix(r.URL.Path, "/api/video/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		http.Error(w, "missing id", 400)
		return
	}

	v, ok := loadFreshVideoByID(id, tokenRefreshLead)
	if !ok {
		http.NotFound(w, r)
		return
	}
	type videoResponse struct {
		Video
		WatchedPosition int `json:"watched_position,omitempty"`
	}
	resp := videoResponse{Video: v}
	if uid, _, ok := authMiddleware(w, r); ok {
		db.QueryRow("SELECT COALESCE(position, 0) FROM watch_history WHERE user_id = ? AND video_id = ?", uid, id).Scan(&resp.WatchedPosition)
	}
	json.NewEncoder(w).Encode(resp)
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=60")

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Redirect(w, r, "/", 302)
		return
	}

	sanitized := sanitizeFTSQuery(q)
	videos := []Video{}

	if sanitized != "" {
		rows, err := db.Query(
			`SELECT v.id, v.slug, v.title, v.description, v.categories, v.duration, v.views, v.thumb_uuid, v.preview_url, v.added_at, v.upload_date, v.source
			 FROM videos_fts f JOIN videos v ON v.rowid = f.rowid
			 WHERE videos_fts MATCH ?
			 ORDER BY rank LIMIT 100`, sanitized)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()

		for rows.Next() {
			v := Video{}
			var dur, views sql.NullInt64
			var cats, uploadDate sql.NullString
			rows.Scan(&v.ID, &v.Slug, &v.Title, &v.Description, &cats, &dur, &views, &v.ThumbUUID, &v.PreviewURL, &v.AddedAt, &uploadDate, &v.Source)
			v.Duration = int(dur.Int64)
			v.Views = int(views.Int64)
			if cats.Valid && cats.String != "" {
				v.Categories = strings.Split(cats.String, ",")
			}
			if uploadDate.Valid {
				v.UploadDate = uploadDate.String
			}
			videos = append(videos, v)
		}
	}

	data := map[string]interface{}{
		"Videos": videos,
		"Query":  q,
		"Count":  len(videos),
	}
	tmpl.ExecuteTemplate(w, "index.html", data)
}

func handlePlay(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")

	id := strings.TrimPrefix(r.URL.Path, "/play/")
	id = strings.TrimSuffix(id, "/")

	v, ok := loadFreshVideoByID(id, tokenRefreshLead)
	if !ok {
		http.NotFound(w, r)
		return
	}
	renderPlayPage(w, r, v)
}

func handleVidProxy(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/vid/")
	parts := strings.SplitN(path, "/", 2)
	id := strings.TrimSuffix(parts[0], "/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	quality := ""
	if len(parts) > 1 {
		quality = strings.TrimSuffix(parts[1], "/")
	}

	v, ok := loadOrRefreshVideo(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	var targetURL string
	switch quality {
	case "360":
		targetURL = v.URL360
	case "720":
		targetURL = v.URL720
	case "1080":
		targetURL = v.URL1080
	}
	if targetURL == "" {
		for _, u := range []string{v.URL1080, v.URL720, v.URL360} {
			if u != "" {
				targetURL = u
				break
			}
		}
	}
	if targetURL == "" {
		http.NotFound(w, r)
		return
	}

	proxyCDN(w, r, targetURL)
}

func previewURLFromThumbnail(rawURL string) string {
	if rawURL == "" || !strings.HasPrefix(rawURL, "http") {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	idx := strings.LastIndex(u.Path, "/")
	if idx < 0 {
		return ""
	}
	u.Path = u.Path[:idx+1] + "preview.mp4"
	return u.String()
}

func handleThumbProxy(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/thumb/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	if strings.Contains(path, "..") || strings.Contains(path, "\x00") || strings.HasPrefix(path, "/") {
		http.NotFound(w, r)
		return
	}
	targetURL := fmt.Sprintf("%s/%s", thumbCDN, path)
	proxyCDN(w, r, targetURL)
}

func handleMediaProxy(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		http.NotFound(w, r)
		return
	}
	target, err := url.Parse(targetURL)
	if err != nil || target.Hostname() == "" {
		http.Error(w, "bad url", http.StatusBadRequest)
		return
	}
	if target.Scheme != "https" {
		http.Error(w, "forbidden scheme", http.StatusForbidden)
		return
	}
	if !isAllowedCDNHost(target.Hostname()) {
		http.Error(w, "forbidden host", http.StatusForbidden)
		return
	}
	if !hasSafeResolvedIPs(target.Hostname()) {
		http.Error(w, "forbidden target", http.StatusForbidden)
		return
	}
	proxyCDN(w, r, targetURL)
}

func hasPlayableMedia(v Video) bool {
	return v.URL360 != "" || v.URL720 != "" || v.URL1080 != "" || v.HLSURL != ""
}

func tokenedSource(source string) bool {
	switch source {
	case "", "xnxx", "xhamster":
		return true
	default:
		return false
	}
}

func expiryFromMediaURL(rawURL string) int64 {
	if rawURL == "" {
		return 0
	}
	if u, err := url.Parse(rawURL); err == nil {
		if token := u.Query().Get("secure"); token != "" {
			if expiry := parseTokenExpiry(token); expiry > 0 {
				return expiry
			}
		}
	}
	if m := regexp.MustCompile(`[?&]secure=([^&"'\s]+)`).FindStringSubmatch(rawURL); len(m) > 1 {
		if expiry := parseTokenExpiry(m[1]); expiry > 0 {
			return expiry
		}
	}
	if m := reXhTokenExpiry.FindStringSubmatch(rawURL); len(m) > 1 {
		var expiry int64
		fmt.Sscanf(m[1], "%d", &expiry)
		return expiry
	}
	return 0
}

func mediaExpiresAt(v Video) int64 {
	var minExpiry int64
	for _, rawURL := range []string{v.URL360, v.URL720, v.URL1080, v.HLSURL} {
		expiry := expiryFromMediaURL(rawURL)
		if expiry == 0 {
			continue
		}
		if minExpiry == 0 || expiry < minExpiry {
			minExpiry = expiry
		}
	}
	if minExpiry == 0 {
		minExpiry = v.ExpiresAt
	}
	return minExpiry
}

func normalizeVideoExpiry(v *Video) {
	if v.ExpiresAt == 0 {
		v.ExpiresAt = mediaExpiresAt(*v)
	}
}

func videoNeedsTokenRefresh(v Video, lead time.Duration) bool {
	if !hasPlayableMedia(v) {
		return tokenedSource(v.Source)
	}
	expiry := mediaExpiresAt(v)
	if expiry == 0 {
		return tokenedSource(v.Source)
	}
	return expiry <= time.Now().Add(lead).Unix()
}

func refreshLock(videoID string) func() {
	val, _ := refreshLocks.LoadOrStore(videoID, &sync.Mutex{})
	mu := val.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func scrapeVideoDetailForSource(id, source string) (Video, error) {
	switch source {
	case "xvideos":
		return scrapeXvVideoDetail(id)
	case "xhamster":
		return scrapeXhVideoDetail(id)
	case "eporner":
		return scrapeEpVideoDetail(id)
	case "tnaflix":
		return scrapeTfVideoDetail(id)
	case "drtuber":
		return scrapeDtVideoDetail(id)
	default:
		if isKVSSource(source) {
			return scrapeKVSVideoDetail(id, source)
		}
		return scrapeVideoDetail(id)
	}
}

func loadVideoFromDB(id string) (Video, bool) {
	v := Video{}
	var cats, tags string
	err := db.QueryRow(
		`SELECT id, COALESCE(slug,''), COALESCE(title,''), COALESCE(description,''),
		        COALESCE(categories,''), COALESCE(tags,''), COALESCE(uploader,''), COALESCE(upload_date,''),
		        COALESCE(duration,0), COALESCE(views,0),
		        COALESCE(url_360,''), COALESCE(url_720,''), COALESCE(url_1080,''), COALESCE(hls_url,''),
		        COALESCE(thumb_uuid,''), COALESCE(preview_url,''), COALESCE(secure_token,''),
		        COALESCE(expires_at,0), COALESCE(source,'xnxx'), COALESCE(added_at,'')
		 FROM videos WHERE id = ?`, id,
	).Scan(&v.ID, &v.Slug, &v.Title, &v.Description, &cats, &tags, &v.Uploader, &v.UploadDate,
		&v.Duration, &v.Views, &v.URL360, &v.URL720, &v.URL1080, &v.HLSURL,
		&v.ThumbUUID, &v.PreviewURL, &v.SecureToken, &v.ExpiresAt, &v.Source, &v.AddedAt)
	if err != nil {
		return v, false
	}
	if cats != "" {
		v.Categories = strings.Split(cats, ",")
	}
	if tags != "" {
		v.Tags = strings.Split(tags, ",")
	}
	normalizeVideoExpiry(&v)
	db.QueryRow("SELECT COALESCE(watch_count, 0) FROM video_watch_counts WHERE video_id = ?", id).Scan(&v.WatchCount)
	return v, true
}

func ensureFreshVideo(v Video, lead time.Duration) (Video, error) {
	if !isPlayableSource(v.Source) {
		// Non-playable sources (eporner, drtuber, tnaflix) can't have their
		// media URLs refreshed server-side. Return as-is.
		return v, nil
	}
	normalizeVideoExpiry(&v)
	if hasPlayableMedia(v) && !videoNeedsTokenRefresh(v, lead) {
		return v, nil
	}

	unlock := refreshLock(v.ID)
	defer unlock()

	if latest, ok := loadVideoFromDB(v.ID); ok {
		v = latest
		if hasPlayableMedia(v) && !videoNeedsTokenRefresh(v, lead) {
			setCachedVideo(v.ID, v)
			return v, nil
		}
	}

	refreshed, err := scrapeVideoDetailForSource(v.ID, v.Source)
	if err != nil {
		recordScrapeFailure(v.ID, err)
		return v, err
	}
	normalizeVideoExpiry(&refreshed)
	if !hasPlayableMedia(refreshed) {
		err := fmt.Errorf("%s detail scrape returned no playable media for %s", refreshed.Source, refreshed.ID)
		recordScrapeFailure(v.ID, err)
		return v, err
	}
	storeVideo(refreshed)
	clearScrapeFailure(v.ID)
	setCachedVideo(v.ID, refreshed)
	return refreshed, nil
}

func loadFreshVideoByID(id string, lead time.Duration) (Video, bool) {
	if v, ok := getCachedVideo(id); ok {
		normalizeVideoExpiry(&v)
		if hasPlayableMedia(v) && !videoNeedsTokenRefresh(v, lead) {
			return v, true
		}
	}
	v, ok := loadVideoFromDB(id)
	if !ok {
		return v, false
	}
	fresh, err := ensureFreshVideo(v, lead)
	if err != nil {
		log.Printf("fresh media unavailable for %s (%s): %v", v.ID, v.Source, err)
		return Video{}, false
	}
	return fresh, true
}

func deleteVideoEverywhere(videoID, reason string) {
	db.Exec("DELETE FROM favorites WHERE video_id = ?", videoID)
	db.Exec("DELETE FROM watch_history WHERE video_id = ?", videoID)
	db.Exec("DELETE FROM playlist_videos WHERE video_id = ?", videoID)
	db.Exec("DELETE FROM video_comments WHERE video_id = ?", videoID)
	db.Exec("DELETE FROM video_reactions WHERE video_id = ?", videoID)
	db.Exec("DELETE FROM video_watch_counts WHERE video_id = ?", videoID)
	db.Exec("DELETE FROM scrape_failures WHERE video_id = ?", videoID)
	if _, err := db.Exec("DELETE FROM videos WHERE id = ?", videoID); err != nil {
		log.Printf("delete failed for %s: %v", videoID, err)
		return
	}
	videoCache.Delete(videoID)
	log.Printf("Deleted unplayable video %s after repeated scrape failures: %s", videoID, reason)
}

func pruneFailedVideoIfUnplayable(videoID, reason string) {
	var playable int
	err := db.QueryRow(`SELECT CASE WHEN COALESCE(url_360,'') <> ''
		OR COALESCE(url_720,'') <> ''
		OR COALESCE(url_1080,'') <> ''
		OR COALESCE(hls_url,'') <> ''
		THEN 1 ELSE 0 END FROM videos WHERE id = ?`, videoID).Scan(&playable)
	if err == sql.ErrNoRows {
		clearScrapeFailure(videoID)
		return
	}
	if err != nil || playable == 1 {
		return
	}
	deleteVideoEverywhere(videoID, reason)
}

func loadOrRefreshVideo(id string) (Video, bool) {
	if v, ok := getCachedVideo(id); ok {
		normalizeVideoExpiry(&v)
		if hasPlayableMedia(v) && !videoNeedsTokenRefresh(v, tokenRefreshLead) {
			return v, true
		}
	}
	return loadFreshVideoByID(id, tokenRefreshLead)
}

var allowedCDNHostSuffixes = []string{
	"xnxx-cdn.com",
	"xhcdn.com",
	"eporner.com",
	"tnaflix.com",
	"drtuber.com",
	"drtst.com",
	"heavyfetish.com",
	"punishbang.com",
	"sunporno.com",
}

func isAllowedCDNHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix((&url.URL{Host: host}).Hostname(), "."))
	if host == "" {
		return false
	}
	for _, domain := range allowedCDNHostSuffixes {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func hasSafeResolvedIPs(host string) bool {
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if ip.IsLoopback() ||
			ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() ||
			ip.IsPrivate() ||
			ip.IsUnspecified() ||
			ip.IsMulticast() {
			return false
		}
	}
	return true
}

func proxyCDN(w http.ResponseWriter, r *http.Request, targetURL string) {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		http.Error(w, "Bad URL", 500)
		return
	}
	if strings.Contains(targetURL, "xhcdn.com") {
		req.Header.Set("Referer", xhBase+"/")
	} else {
		req.Header.Set("Referer", xnxxBase+"/")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	if rangeHdr := r.Header.Get("Range"); rangeHdr != "" {
		req.Header.Set("Range", rangeHdr)
	}

	if strings.Contains(targetURL, "thumb") || strings.Contains(targetURL, "preview") || strings.Contains(targetURL, "jpg") || strings.Contains(targetURL, "jpeg") || strings.Contains(targetURL, "webp") || strings.Contains(targetURL, "png") {
		w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
		if etag := r.Header.Get("If-None-Match"); etag != "" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	client := mediaClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "CDN unreachable", 502)
		return
	}
	defer resp.Body.Close()

	for key, vals := range resp.Header {
		for _, val := range vals {
			w.Header().Add(key, val)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, io.LimitReader(resp.Body, maxProxyBytes)); err != nil {
		log.Printf("proxy copy failed for %s: %v", targetURL, err)
	}
}

func renderPlayPage(w http.ResponseWriter, r *http.Request, v Video) {
	related := fetchRelated(v)

	data := map[string]interface{}{
		"Video":   v,
		"Related": related,
	}
	tmpl.ExecuteTemplate(w, "play.html", data)
}

func fetchRelated(v Video) []Video {
	related := []Video{}
	categories := normalizeCategoryList(v.Categories)
	if len(categories) == 0 {
		categories = loadVideoCategories(v.ID)
	}
	if len(categories) > 0 {
		placeholders := make([]string, 0, len(categories))
		catArgs := make([]interface{}, 0, len(categories)+2)
		for _, category := range categories {
			placeholders = append(placeholders, "?")
			catArgs = append(catArgs, category)
		}
		catArgs = append(catArgs, v.ID, 12)
		rrows, err := db.Query(
			"SELECT DISTINCT v.id, v.title, v.duration, v.views, v.thumb_uuid FROM videos v JOIN video_categories vc ON vc.video_id = v.id WHERE vc.category IN ("+strings.Join(placeholders, ",")+") AND v.id != ? AND "+playableMediaSQLV+" ORDER BY v.views DESC LIMIT ?",
			catArgs...)
		if err == nil {
			for rrows.Next() {
				rv := Video{}
				var rdur, rviews sql.NullInt64
				rrows.Scan(&rv.ID, &rv.Title, &rdur, &rviews, &rv.ThumbUUID)
				rv.Duration = int(rdur.Int64)
				rv.Views = int(rviews.Int64)
				related = append(related, rv)
			}
			rrows.Close()
		}
	}
	if len(related) == 0 {
		rrows, err := db.Query("SELECT id, title, duration, views, thumb_uuid FROM videos WHERE id != ? AND "+playableMediaSQL+" ORDER BY views DESC LIMIT 12", v.ID)
		if err == nil {
			for rrows.Next() {
				rv := Video{}
				var rdur, rviews sql.NullInt64
				rrows.Scan(&rv.ID, &rv.Title, &rdur, &rviews, &rv.ThumbUUID)
				rv.Duration = int(rdur.Int64)
				rv.Views = int(rviews.Int64)
				related = append(related, rv)
			}
			rrows.Close()
		}
	}
	return related
}

// --- API ---

func handleSearchSuggest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"categories": []map[string]interface{}{},
			"videos":     []Video{},
		})
		return
	}

	categorySuggestions := []map[string]interface{}{}
	categoryTerm := normalizeCategoryTerm(q)
	if categoryTerm == "" {
		categoryTerm = strings.ToLower(q)
	}
	rows, err := db.Query(`SELECT category, COUNT(*) AS c
		FROM video_categories
		WHERE category LIKE ? ESCAPE '\'
		GROUP BY category
		ORDER BY c DESC, category ASC
		LIMIT 6`, escapeLikePattern(categoryTerm)+"%")
	if err != nil {
		log.Printf("search suggest category query failed for %q: %v", q, err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var name string
			var count int
			if err := rows.Scan(&name, &count); err != nil {
				log.Printf("search suggest category scan failed: %v", err)
				continue
			}
			categorySuggestions = append(categorySuggestions, map[string]interface{}{
				"name":  name,
				"count": count,
			})
		}
		if err := rows.Err(); err != nil {
			log.Printf("search suggest category rows error: %v", err)
		}
	}

	videoSuggestions := []Video{}
	sanitized := sanitizeFTSQuery(q)
	if sanitized != "" {
		rows, err := db.Query(
			`SELECT v.id, COALESCE(v.slug,''), COALESCE(v.title,''), COALESCE(v.description,''), v.categories, v.duration, v.views, COALESCE(v.thumb_uuid,''), COALESCE(v.preview_url,''), COALESCE(v.added_at,''), v.upload_date, COALESCE(v.source,'xnxx')
			 FROM videos_fts f
			 JOIN videos v ON v.rowid = f.rowid
			 WHERE videos_fts MATCH ? AND `+playableMediaSQLV+`
			 ORDER BY rank, COALESCE(v.views, 0) DESC
			 LIMIT 6`, sanitized)
		if err != nil {
			log.Printf("search suggest video query failed for %q: %v", q, err)
		} else {
			defer rows.Close()
			for rows.Next() {
				v := Video{}
				var dur, views sql.NullInt64
				var cats, uploadDate sql.NullString
				if err := rows.Scan(&v.ID, &v.Slug, &v.Title, &v.Description, &cats, &dur, &views, &v.ThumbUUID, &v.PreviewURL, &v.AddedAt, &uploadDate, &v.Source); err != nil {
					log.Printf("search suggest video scan failed: %v", err)
					continue
				}
				v.Duration = int(dur.Int64)
				v.Views = int(views.Int64)
				if cats.Valid && cats.String != "" {
					v.Categories = strings.Split(cats.String, ",")
				}
				if uploadDate.Valid {
					v.UploadDate = uploadDate.String
				}
				videoSuggestions = append(videoSuggestions, v)
			}
			if err := rows.Err(); err != nil {
				log.Printf("search suggest video rows error: %v", err)
			}
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"categories": categorySuggestions,
		"videos":     videoSuggestions,
	})
}

func handleAPISearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")

	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, "missing q", 400)
		return
	}

	videos := scrapeXnxxSearch(q)

	validIDs := make([]string, 0, len(videos))
	for _, v := range videos {
		if isValidXnxxID(v.ID) {
			validIDs = append(validIDs, v.ID)
		}
	}
	existingIDs := existingVideoIDSet(validIDs)
	cached, newCount := 0, 0
	for _, v := range videos {
		if !isValidXnxxID(v.ID) {
			continue
		}
		if existingIDs[v.ID] {
			cached++
			continue
		}
		cats := strings.Join(extractCategories(v.Title, "", nil), ",")
		db.Exec("INSERT OR IGNORE INTO videos (id, slug, title, categories, added_at) VALUES (?,?,?,?,?)",
			v.ID, v.Slug, v.Title, cats, time.Now().Format("2006-01-02"))
		detail, err := scrapeVideoDetail(v.ID)
		if err != nil {
			log.Printf("Detail scrape failed for %s: %v", v.ID, err)
			v.Source = "xnxx"
			if v.AddedAt == "" {
				v.AddedAt = time.Now().Format("2006-01-02")
			}
			storeVideo(v)
			continue
		}
		storeVideo(detail)
		existingIDs[v.ID] = true
		newCount++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"scanned": len(videos),
		"new":     newCount,
		"cached":  cached,
		"videos":  videos,
	})
}

func handleAPICrawl(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	go runFullCrawl()
	go runXvCrawl()
	go runXhCrawl()
	go runEpCrawl()
	go runTfCrawl()
	go runDtCrawl()
	go runKVSCrawl()
	http.Redirect(w, r, "/", 302)
}

func runFullCrawl() {
	if !crawlMu.TryLock() {
		log.Println("Crawl already running (in-process lock)")
		return
	}
	if _, err := os.Stat(crawlLockPath); err == nil {
		crawlMu.Unlock()
		log.Println("Crawl already running (lock file exists)")
		return
	}
	os.WriteFile(crawlLockPath, []byte{}, 0644)
	defer os.Remove(crawlLockPath)
	defer crawlMu.Unlock()

	setProgress("xnxx", "searching", 0, 0, 0, 0, 0, 0)

	searchURL := xnxxBase + "/search/best"
	videos := []Video{}
	seen := map[string]bool{}
	newVIDs := []string{}

	for page := 0; ; page++ {
		pageURL := searchURL
		if page > 0 {
			pageURL = fmt.Sprintf("%s/%d", searchURL, page)
		}
		log.Printf("XNXX: scanning %s", pageURL)

		resp, err := httpGetWithRetry(pageURL)
		if err != nil {
			log.Printf("Search page %d failed: %v", page, err)
			if page > 0 {
				break
			}
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
		if err != nil {
			if page > 0 {
				break
			}
			continue
		}

		pageCount := 0
		batch := []Video{}
		doc.Find("a[href^='/video-']").Each(func(i int, s *goquery.Selection) {
			href, exists := s.Attr("href")
			if !exists {
				return
			}
			parts := strings.Split(strings.TrimPrefix(href, "/video-"), "/")
			if len(parts) < 2 {
				return
			}
			id, slug := parts[0], parts[1]
			if seen[id] {
				return
			}
			seen[id] = true
			pageCount++

			title := strings.TrimSpace(s.Text())
			if title == "" {
				title = slugToTitle(slug)
			}

			v := Video{ID: id, Slug: slug, Title: title}
			batch = append(batch, v)
			videos = append(videos, v)
		})

		if pageCount == 0 {
			break
		}

		// Insert stubs for this page immediately so UI updates in realtime
		for _, v := range batch {
			if !isValidXnxxID(v.ID) {
				continue
			}
			var exists string
			db.QueryRow("SELECT id FROM videos WHERE id = ?", v.ID).Scan(&exists)
			if exists != "" {
				continue
			}
			cats := strings.Join(extractCategories(v.Title, "", nil), ",")
			db.Exec("INSERT OR IGNORE INTO videos (id, slug, title, categories, added_at) VALUES (?,?,?,?,?)",
				v.ID, v.Slug, v.Title, cats, time.Now().Format("2006-01-02"))
			newVIDs = append(newVIDs, v.ID)
		}

		setProgress("xnxx", "searching", len(videos), len(newVIDs), len(videos)-len(newVIDs), 0, len(newVIDs), page)
		log.Printf("Page %d: %d new (total scanned: %d, new: %d)", page, pageCount, len(videos), len(newVIDs))

		if pageCount < 20 {
			log.Printf("Search complete: %d total, %d new across %d pages", len(videos), len(newVIDs), page+1)
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Phase 2: expand via seed queue (tags, letters, pornstars, etc.)
	defer processLetters()
	defer processTags()
	defer processCategories()
	defer processHits()
	defer processGoldHits()
	defer processPornstars()
	defer processSitemaps()

	if len(newVIDs) == 0 {
		log.Println("No new videos found from search")
		setProgress("xnxx", "idle", len(videos), 0, len(videos), 0, 0, 0)
		return
	}

	// Detail scrape all new videos
	setProgress("xnxx", "scraping", len(videos), len(newVIDs), len(videos)-len(newVIDs), 0, len(newVIDs), 0)
	log.Printf("Detail scraping %d new videos...", len(newVIDs))

	bgWg.Add(1)
	var wg sync.WaitGroup
	for i, id := range newVIDs {
		if i > 0 {
			time.Sleep(300 * time.Millisecond)
		}
		wg.Add(1)
		scrapeSem <- struct{}{}
		go func(vid string) {
			defer wg.Done()
			defer func() { <-scrapeSem }()
			detail, err := scrapeVideoDetail(vid)
			if err != nil {
				log.Printf("Detail scrape failed for %s: %v", vid, err)
				storeExistingStubVideo(vid)
				recordScrapeFailure(vid, err)
				time.Sleep(2 * time.Second)
				return
			}
			storeVideo(detail)
			clearScrapeFailure(vid)
			progress.mu.Lock()
			progress.DetailDone++
			progress.TotalCount++
			d := progress.DetailDone
			t := progress.DetailTotal
			progress.mu.Unlock()
			if d%10 == 0 {
				log.Printf("Detail progress: %d/%d", d, t)
			}
		}(id)
	}
	wg.Wait()
	bgWg.Done()

	setProgress("xnxx", "idle", len(videos), len(newVIDs), len(videos)-len(newVIDs), len(newVIDs), len(newVIDs), 0)
	log.Printf("Full crawl complete: %d scanned, %d new, %d cached, %d detail scraped", len(videos), len(newVIDs), len(videos)-len(newVIDs), len(newVIDs))
}

const maxSeedsPerType = 50

func seedCats(cats []string) {
	for _, cat := range cats {
		db.Exec("INSERT OR IGNORE INTO crawl_seeds (seed, type) VALUES (?, 'category')", cat)
	}
}

func initCrawlSeeds() {
	for c := 'a'; c <= 'z'; c++ {
		db.Exec("INSERT OR IGNORE INTO crawl_seeds (seed, type) VALUES (?, 'letter')", string(c))
	}
	seedCats([]string{"amateur", "teen", "milf", "anal", "big-tits", "blowjob", "lesbian", "creampie", "compilation", "homemade", "pov", "bbc", "rough", "latina", "outdoor", "group", "mature", "japanese", "ebony", "squirting", "handjob", "gangbang", "public", "vintage", "interracial", "threesome", "casting", "hentai", "asian", "redhead", "big-ass", "british", "german", "french", "spanish", "indian", "italian", "russian", "czech", "dutch", "korean", "arab", "transgender", "shemale", "fetish", "bdsm", "bondage", "cosplay", "massage", "webcam", "orgy", "cumshot", "facial", "deepthroat", "doggystyle", "cowgirl", "missionary", "chubby", "bbw", "skinny", "hairy", "shaved", "tattooed", "pierced", "hidden-cam", "spy", "upskirt", "park", "car", "office", "kitchen", "bedroom", "bathroom", "forest", "beach", "snow", "party", "wedding", "vacation", "travel", "camping", "swimming", "dancing", "yoga", "cartoon", "anime", "manga", "3d", "cgi", "animation", "pornstar", "reality", "romantic", "softcore", "hardcore", "solo-male", "solo-female", "small-tits", "natural-tits", "large", "huge", "fisting", "pissing", "fantasy", "superhero", "spoof", "fake", "nude", "topless", "shower", "bath", "sauna", "locker-room", "dressing-room", "train", "plane", "boat", "classroom", "library", "restaurant", "basement", "garage", "balcony", "warehouse", "factory", "farm", "barn", "island", "desert", "winter", "summer", "halloween", "christmas", "new-year", "valentine", "easter", "birthday", "honeymoon", "holiday", "road-trip", "hiking", "climbing", "skiing", "surfing", "diving", "fishing", "hunting", "golf", "tennis", "soccer", "football", "basketball", "baseball", "volleyball", "hockey", "rugby", "cricket", "boxing", "wrestling", "mma", "crossfit", "pilates", "ballet", "pole-dancing", "striptease", "burlesque", "circus", "magic", "acrobatics", "gymnastics", "parkour", "martial-arts", "karate", "taekwondo", "judo", "kung-fu", "capoeira", "kickboxing", "fencing", "biting", "tickling", "electro", "wax", "piercing", "suspension", "cage", "chains", "shackles", "gag", "blindfold", "hood", "mask", "costume", "uniform", "military", "police", "nurse", "doctor", "teacher", "student", "cheerleader", "maid", "secretary", "boss", "lawyer", "judge", "priest", "nun", "angel", "devil", "demon", "zombie", "vampire", "werewolf", "ghost", "alien", "robot", "doll", "painting", "sketch", "comic", "illustration", "digital-art", "stop-motion", "puppet", "disney", "pixar"})
	for i := 1; i <= 50; i++ {
		db.Exec("INSERT OR IGNORE INTO crawl_seeds (seed, type) VALUES (?, 'pornstar-index')", fmt.Sprintf("%d", i))
	}
	for i := 1; i <= 100; i++ {
		db.Exec("INSERT OR IGNORE INTO crawl_seeds (seed, type) VALUES (?, 'hits')", fmt.Sprintf("%d", i))
	}
	for i := 1; i <= 277; i++ {
		db.Exec("INSERT OR IGNORE INTO crawl_seeds (seed, type) VALUES (?, 'gold-hits')", fmt.Sprintf("%d", i))
	}
}

func scrapeVideosFromURL(urlStr string) []Video {
	resp, err := httpGetWithRetry(urlStr)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil
	}
	var videos []Video
	doc.Find("a[href^='/video-']").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}
		parts := strings.Split(strings.TrimPrefix(href, "/video-"), "/")
		if len(parts) < 2 {
			return
		}
		id, slug := parts[0], parts[1]
		title := strings.TrimSpace(s.Text())
		if title == "" {
			title = slugToTitle(slug)
		}
		videos = append(videos, Video{ID: id, Slug: slug, Title: title})
	})
	return videos
}

func processCategories() {
	rows, err := db.Query(`SELECT seed FROM crawl_seeds WHERE type = 'category' AND consumed = 0 LIMIT ?`, maxSeedsPerType)
	if err != nil {
		return
	}
	defer rows.Close()
	var seeds []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		seeds = append(seeds, s)
	}
	if len(seeds) == 0 {
		return
	}
	log.Printf("Phase 2d: scraping %d category pages...", len(seeds))
	var videos []Video
	seen := map[string]bool{}
	for _, cat := range seeds {
		for _, v := range scrapeVideosFromURL(xnxxBase + "/best/" + url.PathEscape(cat)) {
			if !seen[v.ID] {
				seen[v.ID] = true
				videos = append(videos, v)
			}
		}
	}
	newVIDs := insertVideoStubs(videos)
	if len(newVIDs) > 0 {
		detailScrapeBatch(newVIDs, "category")
	}
	for _, seed := range seeds {
		db.Exec("UPDATE crawl_seeds SET consumed = 1 WHERE seed = ? AND type = 'category'", seed)
	}
	log.Printf("Phase 2d: %d new from %d category seeds", len(newVIDs), len(seeds))
}

func processHits() {
	rows, err := db.Query(`SELECT seed FROM crawl_seeds WHERE type = 'hits' AND consumed = 0 LIMIT 10`)
	if err != nil {
		return
	}
	defer rows.Close()
	var seeds []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		seeds = append(seeds, s)
	}
	if len(seeds) == 0 {
		return
	}
	log.Printf("Phase 2e: scraping %d hits pages...", len(seeds))
	var videos []Video
	seen := map[string]bool{}
	for _, page := range seeds {
		for _, v := range scrapeVideosFromURL(xnxxBase + "/hits/" + page) {
			if !seen[v.ID] {
				seen[v.ID] = true
				videos = append(videos, v)
			}
		}
	}
	newVIDs := insertVideoStubs(videos)
	if len(newVIDs) > 0 {
		detailScrapeBatch(newVIDs, "hits")
	}
	for _, seed := range seeds {
		db.Exec("UPDATE crawl_seeds SET consumed = 1 WHERE seed = ? AND type = 'hits'", seed)
	}
	log.Printf("Phase 2e: %d new from %d hits seeds", len(newVIDs), len(seeds))
}

func processGoldHits() {
	rows, err := db.Query(`SELECT seed FROM crawl_seeds WHERE type = 'gold-hits' AND consumed = 0 LIMIT 10`)
	if err != nil {
		return
	}
	defer rows.Close()
	var seeds []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		seeds = append(seeds, s)
	}
	if len(seeds) == 0 {
		return
	}
	log.Printf("Phase 2f: scraping %d gold-hits pages...", len(seeds))
	var videos []Video
	seen := map[string]bool{}
	for _, page := range seeds {
		for _, v := range scrapeVideosFromURL(xnxxBase + "/gold-hits/" + page) {
			if !seen[v.ID] {
				seen[v.ID] = true
				videos = append(videos, v)
			}
		}
	}
	newVIDs := insertVideoStubs(videos)
	if len(newVIDs) > 0 {
		detailScrapeBatch(newVIDs, "gold-hits")
	}
	for _, seed := range seeds {
		db.Exec("UPDATE crawl_seeds SET consumed = 1 WHERE seed = ? AND type = 'gold-hits'", seed)
	}
	log.Printf("Phase 2f: %d new from %d gold-hits seeds", len(newVIDs), len(seeds))
}

func processSitemaps() {
	log.Println("Phase 2g: fetching sitemaps for direct video discovery...")
	setProgress("xnxx", "sitemap", 0, 0, 0, 0, 0, 0)

	// Fetch upload sitemap for direct video URLs
	resp, err := httpGetWithRetry(xnxxBase + "/sitemap_uploads_new.xml")
	if err != nil {
		log.Printf("Sitemap fetch failed: %v", err)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()

	var sitemapVIDs []string
	for _, m := range regexp.MustCompile(`video-([a-z0-9]+)/`).FindAllStringSubmatch(string(body), -1) {
		sitemapVIDs = append(sitemapVIDs, m[1])
	}

	if len(sitemapVIDs) == 0 {
		log.Println("No video IDs found in sitemap")
		return
	}

	log.Printf("Found %d video IDs in sitemap", len(sitemapVIDs))

	// Fetch popular search terms from main sitemap
	resp2, err := httpGetWithRetry(xnxxBase + "/sitemap_main.xml")
	if err == nil {
		body2, _ := io.ReadAll(io.LimitReader(resp2.Body, 1<<20))
		resp2.Body.Close()
		for _, m := range regexp.MustCompile(`xnxx\.com/search/([^<]+)`).FindAllStringSubmatch(string(body2), -1) {
			term, _ := url.QueryUnescape(m[1])
			if term != "" {
				db.Exec("INSERT OR IGNORE INTO crawl_seeds (seed, type) VALUES (?, 'popular-search')", term)
			}
		}
	}

	// Insert sitemap videos as stubs
	var newIDs []string
	for _, vid := range sitemapVIDs {
		if !isValidXnxxID(vid) {
			continue
		}
		var exists string
		db.QueryRow("SELECT id FROM videos WHERE id = ?", vid).Scan(&exists)
		if exists != "" {
			continue
		}
		db.Exec("INSERT OR IGNORE INTO videos (id, slug, title, categories, added_at) VALUES (?,?,?,?,?)",
			vid, vid, "sitemap video", "uncategorized", time.Now().Format("2006-01-02"))
		newIDs = append(newIDs, vid)
	}

	if len(newIDs) > 0 {
		log.Printf("Sitemap: %d new videos, detail scraping...", len(newIDs))
		detailScrapeBatch(newIDs, "sitemap")
	}
	log.Printf("Phase 2g: %d new from sitemap", len(newIDs))
	setProgress("xnxx", "idle", 0, 0, 0, 0, 0, 0)

	// Process popular search seeds
	popRows, err := db.Query(`SELECT seed FROM crawl_seeds WHERE type = 'popular-search' AND consumed = 0 LIMIT 10`)
	if err != nil {
		return
	}
	defer popRows.Close()
	var popSearches []string
	for popRows.Next() {
		var s string
		popRows.Scan(&s)
		popSearches = append(popSearches, s)
	}
	if len(popSearches) == 0 {
		return
	}
	log.Printf("Phase 2h: scraping %d popular search pages...", len(popSearches))
	var popVideos []Video
	popSeen := map[string]bool{}
	for _, term := range popSearches {
		for _, v := range scrapeVideosFromURL(xnxxBase + "/search/" + url.PathEscape(term)) {
			if !popSeen[v.ID] {
				popSeen[v.ID] = true
				popVideos = append(popVideos, v)
			}
		}
	}
	popNew := insertVideoStubs(popVideos)
	if len(popNew) > 0 {
		detailScrapeBatch(popNew, "popular-search")
	}
	for _, seed := range popSearches {
		db.Exec("UPDATE crawl_seeds SET consumed = 1 WHERE seed = ? AND type = 'popular-search'", seed)
	}
	log.Printf("Phase 2h: %d new from %d popular searches", len(popNew), len(popSearches))
}

func processLetters() {
	rows, err := db.Query(`SELECT seed FROM crawl_seeds WHERE type = 'letter' AND consumed = 0 LIMIT 26`)
	if err != nil {
		return
	}
	defer rows.Close()
	var letters []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		letters = append(letters, s)
	}
	if len(letters) == 0 {
		return
	}
	log.Printf("Phase 2a: processing %d tag letter pages...", len(letters))
	for _, letter := range letters {
		resp, err := httpGetWithRetry(xnxxBase + "/tags/" + letter)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		for _, m := range regexp.MustCompile(`/search/([^"]+)"[^>]*><strong>(\d+)`).FindAllStringSubmatch(string(body), -1) {
			tag, _ := url.QueryUnescape(m[1])
			if tag != "" {
				db.Exec("INSERT OR IGNORE INTO crawl_seeds (seed, type) VALUES (?, 'tag')", tag)
			}
		}
		db.Exec("UPDATE crawl_seeds SET consumed = 1 WHERE seed = ? AND type = 'letter'", letter)
	}
	log.Printf("Phase 2a: tag letter pages done")
}

func processTags() {
	rows, err := db.Query(`SELECT seed FROM crawl_seeds WHERE type = 'tag' AND consumed = 0 LIMIT ?`, maxSeedsPerType)
	if err != nil {
		log.Printf("Tag seed query failed: %v", err)
		return
	}
	defer rows.Close()
	var seeds []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		seeds = append(seeds, s)
	}
	if len(seeds) == 0 {
		return
	}
	log.Printf("Phase 2b: scraping %d tag seeds...", len(seeds))
	var videos []Video
	seen := map[string]bool{}
	for _, tag := range seeds {
		for _, v := range scrapeXnxxTagPage(tag) {
			if !seen[v.ID] {
				seen[v.ID] = true
				videos = append(videos, v)
			}
		}
	}
	newVIDs := insertVideoStubs(videos)
	if len(newVIDs) > 0 {
		detailScrapeBatch(newVIDs, "tag")
	}
	for _, seed := range seeds {
		db.Exec("UPDATE crawl_seeds SET consumed = 1 WHERE seed = ? AND type = 'tag'", seed)
	}
	log.Printf("Phase 2b: %d new from %d tag seeds", len(newVIDs), len(seeds))
}

func processPornstars() {
	rows, err := db.Query(`SELECT seed FROM crawl_seeds WHERE type = 'pornstar-index' AND consumed = 0 LIMIT 5`)
	if err != nil {
		return
	}
	defer rows.Close()
	var indices []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		indices = append(indices, s)
	}
	if len(indices) == 0 {
		return
	}
	log.Printf("Phase 2c: processing %d pornstar index pages...", len(indices))
	for _, idx := range indices {
		resp, err := httpGetWithRetry(xnxxBase + "/pornstars/" + idx)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		for _, m := range regexp.MustCompile(`href="/pornstar/([^"]+)"`).FindAllStringSubmatch(string(body), -1) {
			name := m[1]
			db.Exec("INSERT OR IGNORE INTO crawl_seeds (seed, type) VALUES (?, 'pornstar')", name)
		}
		db.Exec("UPDATE crawl_seeds SET consumed = 1 WHERE seed = ? AND type = 'pornstar-index'", idx)
	}
	// Now process a batch of pornstar pages
	prows, err := db.Query(`SELECT seed FROM crawl_seeds WHERE type = 'pornstar' AND consumed = 0 LIMIT ?`, maxSeedsPerType)
	if err != nil {
		return
	}
	defer prows.Close()
	var names []string
	for prows.Next() {
		var s string
		prows.Scan(&s)
		names = append(names, s)
	}
	if len(names) == 0 {
		return
	}
	log.Printf("Phase 2c: scraping %d pornstar pages...", len(names))
	var videos []Video
	seen := map[string]bool{}
	for _, name := range names {
		for _, v := range scrapeXnxxPornstarPage(name) {
			if !seen[v.ID] {
				seen[v.ID] = true
				videos = append(videos, v)
			}
		}
	}
	newVIDs := insertVideoStubs(videos)
	if len(newVIDs) > 0 {
		detailScrapeBatch(newVIDs, "pornstar")
	}
	for _, name := range names {
		db.Exec("UPDATE crawl_seeds SET consumed = 1 WHERE seed = ? AND type = 'pornstar'", name)
	}
	log.Printf("Phase 2c: %d new from %d pornstar seeds", len(newVIDs), len(names))
}

func insertVideoStubs(videos []Video) []string {
	var ids []string
	for _, v := range videos {
		if !isValidXnxxID(v.ID) {
			continue
		}
		var exists string
		db.QueryRow("SELECT id FROM videos WHERE id = ?", v.ID).Scan(&exists)
		if exists != "" {
			continue
		}
		cats := strings.Join(extractCategories(v.Title, "", nil), ",")
		db.Exec("INSERT OR IGNORE INTO videos (id, slug, title, categories, added_at) VALUES (?,?,?,?,?)",
			v.ID, v.Slug, v.Title, cats, time.Now().Format("2006-01-02"))
		ids = append(ids, v.ID)
	}
	return ids
}

func detailScrapeBatch(ids []string, source string) {
	log.Printf("Detail scraping %d new videos from %s expansion...", len(ids), source)
	setProgress("xnxx", "scraping", 0, len(ids), 0, 0, len(ids), 0)
	bgWg.Add(1)
	var wg sync.WaitGroup
	for i, id := range ids {
		if i > 0 {
			time.Sleep(300 * time.Millisecond)
		}
		wg.Add(1)
		scrapeSem <- struct{}{}
		go func(vid string) {
			defer wg.Done()
			defer func() { <-scrapeSem }()
			detail, err := scrapeVideoDetail(vid)
			if err != nil {
				log.Printf("%s detail scrape failed for %s: %v", source, vid, err)
				storeExistingStubVideo(vid)
				recordScrapeFailure(vid, err)
				time.Sleep(2 * time.Second)
				return
			}
			storeVideo(detail)
			clearScrapeFailure(vid)
			progress.mu.Lock()
			progress.DetailDone++
			progress.TotalCount++
			d := progress.DetailDone
			t := progress.DetailTotal
			progress.mu.Unlock()
			if d%10 == 0 {
				log.Printf("%s detail progress: %d/%d", source, d, t)
			}
		}(id)
	}
	wg.Wait()
	bgWg.Done()
	setProgress("xnxx", "idle", 0, 0, 0, 0, 0, 0)
}

func scrapeXnxxPornstarPage(name string) []Video {
	videos := scrapeVideosFromURL(xnxxBase + "/pornstar/" + url.PathEscape(name))
	if len(videos) > 0 {
		log.Printf("Pornstar %q: %d videos", name, len(videos))
	}
	return videos
}

func handleAPIRefresh(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", 400)
		return
	}
	existing, ok := loadVideoFromDB(id)
	source := "xnxx"
	if ok {
		source = existing.Source
	}
	v, err := scrapeVideoDetailForSource(id, source)
	if err != nil {
		recordScrapeFailure(id, err)
		http.Error(w, err.Error(), 500)
		return
	}
	normalizeVideoExpiry(&v)
	if !hasPlayableMedia(v) {
		err := fmt.Errorf("%s detail scrape returned no playable media for %s", v.Source, v.ID)
		recordScrapeFailure(id, err)
		http.Error(w, err.Error(), 502)
		return
	}
	storeVideo(v)
	clearScrapeFailure(id)

	// Update cache
	setCachedVideo(id, v)

	http.Redirect(w, r, "/play/"+id, 302)
}

func scrapeXnxxTagPage(query string) []Video {
	videos := scrapeVideosFromURL(xnxxBase + "/search/" + url.PathEscape(query))
	if len(videos) > 0 {
		log.Printf("Tag page %q: %d videos", query, len(videos))
	}
	return videos
}

// --- XNXX Scraper ---

var (
	reVideoLink = regexp.MustCompile(`<a[^>]*href="/video-([a-z0-9]+)/([^"]+)"`)
	reJSONLD    = regexp.MustCompile(`<script[^>]*type="application/ld\+json"[^>]*>\s*(\{[\s\S]*?\})\s*</script>`)
	reHLSSource = regexp.MustCompile(`https://hls-cdn77\.xnxx-cdn\.com/([^"'\s]+,\d+)/([a-f0-9-]+)/\d+/hls\.m3u8`)
	reThumbUUID = regexp.MustCompile(`/([a-f0-9-]+)/\d+/(?:xn_\d+_t|preview)`)
	reVidScript = regexp.MustCompile(`video_url[^=]*=\s*'([^']+)'`) // JS variable fallback

	// html5player exposes the real per-quality URLs, EACH with its own secure
	// token. xnxx no longer lets one token serve every quality, so these must be
	// captured verbatim — never synthesized by swapping the filename.
	reSetUrlHigh = regexp.MustCompile(`setVideoUrlHigh\(\s*['"]([^'"]+)['"]`)
	reSetUrlLow  = regexp.MustCompile(`setVideoUrlLow\(\s*['"]([^'"]+)['"]`)
	reSetHLS     = regexp.MustCompile(`setVideoHLS\(\s*['"]([^'"]+)['"]`)
	// Two filename generations: legacy video_{res}p.mp4 and 2026+ mp4_{label}.mp4 (sd/hq/hd/fhd)
	reMP4Any = regexp.MustCompile(`https://mp4-[^.]+\.xnxx-cdn\.com/([a-f0-9-]+)/\d+/(?:video_(\d+)p|mp4_([a-z0-9]+))\.mp4\?secure=([^"'\s\\]+)`)
)

// assignMP4Quality stores a real player MP4 URL (with its own token) into the
// quality bucket matching its actual resolution, preferring the higher stream
// when two URLs land in the same bucket. It also captures the uuid/token/expiry
// from whichever URL is parsed. Returns true if the URL was a usable xnxx MP4.
func assignMP4Quality(v *Video, rawURL string) bool {
	m := reMP4Any.FindStringSubmatch(rawURL)
	if len(m) < 5 {
		return false
	}
	uuid, token := m[1], m[4]
	res, _ := strconv.Atoi(m[2])
	if res == 0 && m[3] != "" {
		// New-format label → resolution bucket
		switch m[3] {
		case "ld", "sd":
			res = 360
		case "hq", "hd":
			res = 720
		default: // fhd, uhd, anything newer
			res = 1080
		}
	}

	if v.ThumbUUID == "" {
		v.ThumbUUID = uuid
		v.PreviewURL = fmt.Sprintf("%s/%s/0/preview.mp4", thumbCDN, uuid)
	}
	v.SecureToken = token
	v.ExpiresAt = parseTokenExpiry(token)

	switch {
	case res <= 360:
		if v.URL360 == "" || res >= 360 {
			v.URL360 = rawURL
		}
	case res <= 720:
		v.URL720 = rawURL
	default:
		v.URL1080 = rawURL
	}
	return true
}

func scrapeXnxxSearch(query string) []Video {
	searchURL := xnxxBase + "/search/best"
	if query != "" {
		searchURL = xnxxBase + "/search/" + url.PathEscape(query)
	}

	videos := []Video{}
	seen := map[string]bool{}

	for page := 0; ; page++ {
		pageURL := searchURL
		if page > 0 {
			pageURL = fmt.Sprintf("%s/%d", searchURL, page)
		}

		resp, err := httpGetWithRetry(pageURL)
		if err != nil {
			log.Printf("Search scrape page %d failed: %v", page, err)
			if page > 0 {
				break
			}
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
		if err != nil {
			if page > 0 {
				break
			}
			continue
		}

		pageCount := 0
		doc.Find("a[href^='/video-']").Each(func(i int, s *goquery.Selection) {
			href, exists := s.Attr("href")
			if !exists {
				return
			}
			parts := strings.Split(strings.TrimPrefix(href, "/video-"), "/")
			if len(parts) < 2 {
				return
			}
			id, slug := parts[0], parts[1]
			if seen[id] {
				return
			}
			seen[id] = true
			pageCount++

			title := strings.TrimSpace(s.Text())
			if title == "" {
				title = slugToTitle(slug)
			}

			videos = append(videos, Video{
				ID:    id,
				Slug:  slug,
				Title: title,
			})
		})

		if pageCount > 0 {
			log.Printf("Search page %d: %d new (total %d)", page, pageCount, len(videos))
		}

		if pageCount < 20 {
			log.Printf("Search complete: %d videos across %d pages", len(videos), page+1)
			break
		}
		time.Sleep(1 * time.Second)
	}
	return videos
}

// Real xnxx IDs are lowercase base36 (e.g. 1hl7wj6d). Mixed-case IDs are
// premium/gold teaser links that 404 or redirect off-site — never ingest them.
var reValidXnxxID = regexp.MustCompile(`^[a-z0-9]{5,12}$`)

func isValidXnxxID(id string) bool {
	return reValidXnxxID.MatchString(id)
}

func scrapeVideoDetail(id string) (Video, error) {
	v := Video{ID: id}
	if !isValidXnxxID(id) {
		return v, fmt.Errorf("invalid xnxx id %q", id)
	}

	slug := ""
	db.QueryRow("SELECT slug FROM videos WHERE id = ?", id).Scan(&slug)
	if slug == "" {
		slug = id
	}
	detailURL := fmt.Sprintf("%s/video-%s/%s", xnxxBase, id, slug)

	resp, err := httpGetWithRetry(detailURL)
	if err != nil {
		return v, err
	}
	defer resp.Body.Close()

	// Premium teasers redirect to xnxx.gold — no player data there, only upsell.
	if resp.Request != nil && resp.Request.URL != nil && resp.Request.URL.Host != "www.xnxx.com" {
		return v, fmt.Errorf("redirected off-site to %s", resp.Request.URL.Host)
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	html := string(body)

	// Primary stream source: the html5player setVideoUrl* / setVideoHLS calls.
	// Each carries its own secure token, so we store them verbatim instead of
	// reusing one token across synthesized quality URLs (which now 403s).
	if m := reSetUrlLow.FindStringSubmatch(html); len(m) > 1 {
		assignMP4Quality(&v, m[1])
	}
	if m := reSetUrlHigh.FindStringSubmatch(html); len(m) > 1 {
		assignMP4Quality(&v, m[1])
	}
	if m := reSetHLS.FindStringSubmatch(html); len(m) > 1 {
		v.HLSURL = m[1]
		if hm := reHLSSource.FindStringSubmatch(m[1]); len(hm) > 2 {
			if v.SecureToken == "" {
				v.SecureToken = hm[1]
				v.ExpiresAt = parseTokenExpiry(hm[1])
			}
			if v.ThumbUUID == "" {
				v.ThumbUUID = hm[2]
				v.PreviewURL = fmt.Sprintf("%s/%s/0/preview.mp4", thumbCDN, hm[2])
			}
		}
	}

	// JSON-LD — primary metadata source
	if m := reJSONLD.FindStringSubmatch(html); len(m) > 1 {
		var ld struct {
			Name         string   `json:"name"`
			Description  string   `json:"description"`
			ContentURL   string   `json:"contentUrl"`
			Duration     string   `json:"duration"`
			ThumbnailURL []string `json:"thumbnailUrl"`
			Interaction  struct {
				Count int `json:"userInteractionCount"`
			} `json:"interactionStatistic"`
		}
		if err := json.Unmarshal([]byte(m[1]), &ld); err == nil {
			v.Title = ld.Name
			v.Description = ld.Description
			v.Views = ld.Interaction.Count
			v.Duration = parseDuration(ld.Duration)

			if len(ld.ThumbnailURL) > 0 {
				// Store the concrete CDN thumbnail URL when available so the UI can
				// render the source-selected poster at native quality instead of
				// inventing a hard-coded xn_N_t frame number. Existing rows that only
				// have a UUID remain supported by the frontend.
				v.ThumbUUID = strings.ReplaceAll(ld.ThumbnailURL[0], "\\/", "/")
				if preview := previewURLFromThumbnail(v.ThumbUUID); preview != "" {
					v.PreviewURL = preview
				} else if m2 := reThumbUUID.FindStringSubmatch(v.ThumbUUID); len(m2) > 1 {
					v.PreviewURL = fmt.Sprintf("%s/%s/0/preview.mp4", thumbCDN, m2[1])
				}
			}

			// contentUrl is a real, individually-tokened MP4 — store it in the
			// bucket matching its resolution. Only used if the player block above
			// did not already provide streams. Never synthesize other qualities.
			if ld.ContentURL != "" {
				assignMP4Quality(&v, ld.ContentURL)
			}
		}
	}

	// Fallback: try to find HLS URL in the page source
	if v.HLSURL == "" {
		if m := reHLSSource.FindStringSubmatch(html); len(m) > 1 {
			token := m[1]
			uuid := m[2]
			v.HLSURL = fmt.Sprintf("%s/%s/%s/0/hls.m3u8", hlsCDN, token, uuid)
			v.SecureToken = token
			v.ExpiresAt = parseTokenExpiry(token)
			if uuid != "" {
				v.ThumbUUID = uuid
				v.PreviewURL = fmt.Sprintf("%s/%s/0/preview.mp4", thumbCDN, uuid)
			}
			log.Printf("Found HLS token for %s: %s", id, token)
		}
	}

	// Fallback: try JS video_url variable (carries its own token).
	if v.URL360 == "" && v.URL720 == "" && v.URL1080 == "" {
		if m := reVidScript.FindStringSubmatch(html); len(m) > 1 {
			log.Printf("Found video_url for %s: %s", id, m[1])
			assignMP4Quality(&v, m[1])
		}
	}

	// Fallback: any tokened MP4 anywhere in the page source. Each match keeps
	// its own token — we collect every quality variant present, never fabricate.
	if v.URL360 == "" && v.URL720 == "" && v.URL1080 == "" {
		for _, m := range reMP4Any.FindAllString(html, -1) {
			assignMP4Quality(&v, m)
		}
		if v.SecureToken != "" {
			log.Printf("Found MP4 token for %s via regex: %s", id, v.SecureToken)
		}
	}

	// Use goquery for remaining metadata
	if doc, err := goquery.NewDocumentFromReader(strings.NewReader(html)); err == nil {
		// Tags from meta keywords
		keywords, exists := doc.Find("meta[name='keywords']").Attr("content")
		if exists {
			for _, tag := range strings.Split(keywords, ",") {
				tag = strings.TrimSpace(tag)
				lowtag := strings.ToLower(tag)
				generic := map[string]bool{
					"porn": true, "porn movies": true, "free porn": true, "free porn movies": true,
					"sex": true, "porno": true, "free sex": true, "tube porn": true,
					"tube": true, "videos": true, "full porn": true, "xxnx": true,
					"xnxxx": true, "xxx": true, "pussy": true,
				}
				if tag != "" && !generic[lowtag] {
					v.Tags = append(v.Tags, tag)
				}
			}
		}

		// Uploader
		uploaderText := doc.Find("a[href^='/porn-maker/']").First().Text()
		if uploaderText != "" {
			v.Uploader = strings.TrimSpace(uploaderText)
		}

		// Fallback: meta duration if JSON-LD didn't give one
		if v.Duration == 0 {
			durStr, exists := doc.Find("meta[property='og:duration']").Attr("content")
			if exists {
				fmt.Sscanf(durStr, "%d", &v.Duration)
			}
		}

		// Fallback: meta description
		if v.Description == "" {
			desc, exists := doc.Find("meta[name='description']").Attr("content")
			if exists {
				v.Description = desc
			}
		}
	}

	if v.Title == "" {
		// No JSON-LD and no og:title means this isn't a playable video page
		// (gold teaser, removed video, layout change). Storing it would create
		// a blank card whose title is the raw ID — fail instead.
		return v, fmt.Errorf("no title extracted for %s — not a playable video page", id)
	}
	v.AddedAt = time.Now().Format("2006-01-02")
	v.Source = "xnxx"
	return v, nil
}

func parseTokenExpiry(token string) int64 {
	parts := strings.Split(token, ",")
	if len(parts) == 2 {
		var ts int64
		if _, err := fmt.Sscanf(parts[1], "%d", &ts); err == nil {
			return ts
		}
	}
	return 0
}

func storeVideo(v Video) {
	if v.ID == "" || v.Title == "" {
		return
	}
	mergedCats := mergeCategoryLists(v.Categories, v.Tags, extractCategories(v.Title, v.Description, v.Tags))
	cats := joinStoredCategories(mergedCats)
	tagsStr := strings.Join(v.Tags, ",")
	// On re-scrape keep the original added_at — refreshes must not push old
	// videos back to the top of the Recent feed.
	tx, err := db.Begin()
	if err != nil {
		log.Printf("storeVideo(%s) begin failed: %v", v.ID, err)
		return
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO videos (id, slug, title, description, categories, tags, uploader, upload_date, duration, views, added_at, source, thumb_uuid, url_360, url_720, url_1080, preview_url, hls_url, secure_token, expires_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			slug=CASE WHEN excluded.slug != '' THEN excluded.slug ELSE videos.slug END,
			title=excluded.title, description=excluded.description,
			categories=excluded.categories, tags=excluded.tags, uploader=excluded.uploader,
			upload_date=excluded.upload_date, duration=excluded.duration, views=excluded.views,
			source=excluded.source, thumb_uuid=CASE WHEN excluded.thumb_uuid != '' THEN excluded.thumb_uuid ELSE videos.thumb_uuid END,
			url_360=CASE WHEN excluded.url_360 != '' THEN excluded.url_360 ELSE videos.url_360 END,
			url_720=CASE WHEN excluded.url_720 != '' THEN excluded.url_720 ELSE videos.url_720 END,
			url_1080=CASE WHEN excluded.url_1080 != '' THEN excluded.url_1080 ELSE videos.url_1080 END,
			preview_url=CASE WHEN excluded.preview_url != '' THEN excluded.preview_url ELSE videos.preview_url END,
			hls_url=CASE WHEN excluded.hls_url != '' THEN excluded.hls_url ELSE videos.hls_url END,
			secure_token=CASE WHEN excluded.secure_token != '' THEN excluded.secure_token ELSE videos.secure_token END,
			expires_at=CASE WHEN excluded.expires_at != 0 THEN excluded.expires_at ELSE videos.expires_at END`,
		v.ID, v.Slug, v.Title, v.Description, cats, tagsStr, v.Uploader, v.UploadDate,
		v.Duration, v.Views, v.AddedAt, v.Source,
		v.ThumbUUID, v.URL360, v.URL720, v.URL1080, v.PreviewURL, v.HLSURL,
		v.SecureToken, v.ExpiresAt)
	if err != nil {
		log.Printf("storeVideo(%s) failed: %v", v.ID, err)
		return
	}
	if err := replaceVideoCategoriesTx(tx, v.ID, mergedCats); err != nil {
		log.Printf("storeVideo(%s) category sync failed: %v", v.ID, err)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("storeVideo(%s) commit failed: %v", v.ID, err)
	}
}

// --- xHamster Scraper ---

var (
	reXhInitials    = regexp.MustCompile(`window\.initials\s*=\s*(\{[\s\S]*?\});\s*</script>`)
	reXhHLSPreload  = regexp.MustCompile(`https://video\d+\.xhcdn\.com/key=[^"'\s<>]+m3u8`)
	reXhMP4Link     = regexp.MustCompile(`https://video\d+\.xhcdn\.com/key=[^"'\s<>]*/(\d+)p\.h264\.mp4`)
	reXhTokenExpiry = regexp.MustCompile(`[,&]end=(\d+)`)
	reXhSlugID      = regexp.MustCompile(`/videos/([^/]+)-([a-zA-Z0-9]{7,8})$`)
	reXhThumbURL    = regexp.MustCompile(`"thumbURL"\s*:\s*"([^"]+)"`)
	reXhTrailerURL  = regexp.MustCompile(`"trailerURL"\s*:\s*"([^"]+)"`)
	reXhTitle       = regexp.MustCompile(`"titleLocalized"\s*:\s*"([^"]*)"`)
	reXhDuration    = regexp.MustCompile(`"duration"\s*:\s*(\d+)`)
	reXhViews       = regexp.MustCompile(`"views"\s*:\s*(\d+)`)
	reXhDescription = regexp.MustCompile(`"description"\s*:\s*"([^"]*)"`)
	reXhTags        = regexp.MustCompile(`"isPornstar":\s*true[^}]*"name"\s*:\s*"([^"]+)"|"isCategory":\s*true[^}]*"name"\s*:\s*"([^"]+)"|"name"\s*:\s*"([^"]+)"`)
)

type xhThumb struct {
	ID         int    `json:"id"`
	Title      string `json:"titleLocalized"`
	PageURL    string `json:"pageURL"`
	ThumbURL   string `json:"thumbURL"`
	TrailerURL string `json:"trailerURL"`
	Duration   int    `json:"duration"`
	Views      int    `json:"views"`
}

func httpGetXhWithRetry(urlStr string) (*http.Response, error) {
	<-rateLimitXh

	var lastErr error
	for attempt := 0; attempt < maxHTTPRetries; attempt++ {
		if attempt > 0 {
			delay := retryBaseDelay * time.Duration(1<<(attempt-1))
			if delay > retryMaxDelay {
				delay = retryMaxDelay
			}
			jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
			time.Sleep(delay + jitter)
		}
		req, errReq := http.NewRequest("GET", urlStr, nil)
		if errReq != nil {
			return nil, errReq
		}
		ua := userAgents[rand.Intn(len(userAgents))]
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")
		req.Header.Set("Referer", xhBase+"/")

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			log.Printf("xHamster HTTP attempt %d failed: %v", attempt+1, err)
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			log.Printf("xHamster HTTP %d on attempt %d", resp.StatusCode, attempt+1)
			if attempt == maxHTTPRetries-1 {
				return nil, lastErr
			}
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("xHamster request failed after %d attempts: %w", maxHTTPRetries, lastErr)
}

func scrapeXhListing(pageURL string) []Video {
	resp, err := httpGetXhWithRetry(pageURL)
	if err != nil {
		log.Printf("xHamster listing %s failed: %v", pageURL, err)
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	resp.Body.Close()

	videos := []Video{}
	seen := map[string]bool{}

	// Extract video pageURLs from window.initials JSON — id field no longer exists
	for _, m := range regexp.MustCompile(`"pageURL"\s*:\s*"((?:[^"\\]|\\.)*)"`).FindAllStringSubmatch(string(body), -1) {
		if len(m) < 2 {
			continue
		}
		pageURL := strings.ReplaceAll(m[1], `\/`, `/`)

		slugMatch := reXhSlugID.FindStringSubmatch(pageURL)
		if slugMatch == nil {
			continue
		}
		shortID := slugMatch[2]

		if seen[shortID] {
			continue
		}
		seen[shortID] = true

		v := Video{
			ID:      shortID,
			Slug:    slugMatch[1],
			Source:  "xhamster",
			AddedAt: time.Now().Format("2006-01-02"),
			Title:   slugToTitle(slugMatch[1]),
		}

		videos = append(videos, v)
	}

	return videos
}

func scrapeXhVideoDetail(shortID string) (Video, error) {
	v := Video{ID: shortID, Source: "xhamster"}

	// Get slug from DB — needed for correct URL and to preserve it in the record
	var slug string
	db.QueryRow("SELECT slug FROM videos WHERE id = ?", shortID).Scan(&slug)
	v.Slug = slug

	url := xhBase + "/videos/" + shortID
	resp, err := httpGetXhWithRetry(url)
	if err != nil {
		return v, err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	resp.Body.Close()
	bodyStr := string(body)

	// Extract window.initials JSON block
	initMatch := reXhInitials.FindStringSubmatch(bodyStr)
	initJSON := ""
	if len(initMatch) > 1 {
		initJSON = initMatch[1]
	} else {
		// Fallback: extract video data from pageURL patterns in the HTML
		initJSON = bodyStr
	}

	// Title
	if m := regexp.MustCompile(`"titleLocalized"\s*:\s*"([^"]*)"`).FindStringSubmatch(initJSON); len(m) > 1 {
		v.Title = m[1]
	}
	// Also try meta title fallback
	if v.Title == "" {
		if m := regexp.MustCompile(`<title>([^<]+)</title>`).FindStringSubmatch(bodyStr); len(m) > 1 {
			v.Title = strings.TrimSpace(m[1])
			v.Title = strings.TrimSuffix(v.Title, " - xHamster")
		}
	}

	// Duration
	if m := regexp.MustCompile(`"duration"\s*:\s*(\d+)`).FindStringSubmatch(initJSON); len(m) > 1 {
		fmt.Sscanf(m[1], "%d", &v.Duration)
	}

	// Views
	if m := regexp.MustCompile(`"views"\s*:\s*(\d+)`).FindStringSubmatch(initJSON); len(m) > 1 {
		fmt.Sscanf(m[1], "%d", &v.Views)
	}

	// Description
	if m := regexp.MustCompile(`"description"\s*:\s*"([^"]*)"`).FindStringSubmatch(initJSON); len(m) > 1 {
		v.Description = strings.ReplaceAll(m[1], "\\n", "\n")
		v.Description = strings.ReplaceAll(v.Description, "\\\"", "\"")
	}
	if v.Description == "" {
		if m := regexp.MustCompile(`<meta[^>]*name=["']description["'][^>]*content=["']([^"']+)`).FindStringSubmatch(bodyStr); len(m) > 1 {
			v.Description = m[1]
		}
	}

	// Tags and uploader from window.initials
	tagSet := map[string]bool{}
	catSet := map[string]bool{}
	for _, tm := range reXhTags.FindAllStringSubmatch(initJSON, -1) {
		if len(tm) < 4 {
			continue
		}
		appendUnique := func(dst *[]string, seen map[string]bool, raw string) {
			name := strings.TrimSpace(raw)
			if name == "" || strings.EqualFold(name, v.Title) || seen[name] {
				return
			}
			seen[name] = true
			*dst = append(*dst, name)
		}
		switch {
		case tm[1] != "":
			appendUnique(&v.Tags, tagSet, tm[1])
		case tm[2] != "":
			appendUnique(&v.Categories, catSet, tm[2])
		case tm[3] != "":
			appendUnique(&v.Tags, tagSet, tm[3])
		}
	}

	// Uploader: look for isPornstar or isCreator names
	uploaderMatches := regexp.MustCompile(`"isPornstar":\s*true[^}]*"name"\s*:\s*"([^"]+)"`).FindStringSubmatch(initJSON)
	if len(uploaderMatches) < 2 {
		uploaderMatches = regexp.MustCompile(`"isCreator":\s*true[^}]*"name"\s*:\s*"([^"]+)"`).FindStringSubmatch(initJSON)
	}
	if len(uploaderMatches) > 1 {
		v.Uploader = uploaderMatches[1]
	}

	// Upload date from created timestamp
	if m := regexp.MustCompile(`"created"\s*:\s*(\d+)`).FindStringSubmatch(initJSON); len(m) > 1 {
		var ts int64
		fmt.Sscanf(m[1], "%d", &ts)
		v.UploadDate = time.Unix(ts, 0).Format("2006-01-02")
	}

	// HLS URL from <link rel="preload">
	hlsMatch := reXhHLSPreload.FindString(bodyStr)
	if hlsMatch != "" {
		v.HLSURL = hlsMatch
		// Parse token expiry
		if em := reXhTokenExpiry.FindStringSubmatch(hlsMatch); len(em) > 1 {
			var expiry int64
			fmt.Sscanf(em[1], "%d", &expiry)
			v.ExpiresAt = expiry
		}
	}

	// Direct MP4 URLs
	mp4Matches := reXhMP4Link.FindAllString(bodyStr, -1)
	for _, mp4URL := range mp4Matches {
		resMatch := regexp.MustCompile(`/(\d+)p\.h264\.mp4`).FindStringSubmatch(mp4URL)
		if len(resMatch) > 1 {
			var res int
			fmt.Sscanf(resMatch[1], "%d", &res)
			switch {
			case res <= 360:
				if v.URL360 == "" {
					v.URL360 = mp4URL
				}
			case res <= 720:
				if v.URL720 == "" {
					v.URL720 = mp4URL
				}
			default:
				if v.URL1080 == "" {
					v.URL1080 = mp4URL
				}
			}
		}
		// Parse token expiry if not already set
		if v.ExpiresAt == 0 {
			if em := reXhTokenExpiry.FindStringSubmatch(mp4URL); len(em) > 1 {
				var expiry int64
				fmt.Sscanf(em[1], "%d", &expiry)
				v.ExpiresAt = expiry
			}
		}
	}

	// Thumbnail URL
	if m := regexp.MustCompile(`"thumbURL"\s*:\s*"((?:[^"\\]|\\.)*)"`).FindStringSubmatch(initJSON); len(m) > 1 {
		v.ThumbUUID = unescapeJSON(m[1])
	}
	if m := regexp.MustCompile(`"spriteURL"\s*:\s*"((?:[^"\\]|\\.)*)"`).FindStringSubmatch(initJSON); len(m) > 1 {
		v.PreviewURL = unescapeJSON(m[1])
	}

	v.AddedAt = time.Now().Format("2006-01-02")

	// Try to extract video source / MP4 URLs from the page
	if m := regexp.MustCompile(`(?i)src\s*=\s*["']([^"']*\.mp4[^"']*)["']`).FindStringSubmatch(bodyStr); len(m) > 1 {
		assignMP4Quality(&v, m[1])
	}
	if m := regexp.MustCompile(`(?i)(https?://[^"'\s<>]*?\.mp4[^"'\s<>]*)`).FindStringSubmatch(bodyStr); len(m) > 1 {
		assignMP4Quality(&v, m[1])
	}
	if m := regexp.MustCompile(`"contentUrl"\s*:\s*"([^"]*)"`).FindStringSubmatch(bodyStr); len(m) > 1 {
		assignMP4Quality(&v, m[1])
	}
	if m := regexp.MustCompile(`"embedUrl"\s*:\s*"([^"]*)"`).FindStringSubmatch(bodyStr); len(m) > 1 {
		v.HLSURL = m[1]
	}

	return v, nil
}

func unescapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\/`, `/`)
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}

func handleAPICrawlXh(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	go runXhCrawl()
	http.Redirect(w, r, "/", 302)
}

func runXhCrawl() {
	if !crawlMuXh.TryLock() {
		log.Println("xHamster crawl already running")
		return
	}
	defer crawlMuXh.Unlock()
	xhLockPath := "/tmp/karaxxx-xh-crawl.lock"
	if _, err := os.Stat(xhLockPath); err == nil {
		log.Println("xHamster crawl already running (lock file exists)")
		return
	}
	os.WriteFile(xhLockPath, []byte{}, 0644)
	defer os.Remove(xhLockPath)

	log.Println("Starting xHamster crawl...")
	totalNew := 0
	totalListingFound := 0
	totalFilteredEmpty := 0
	totalFilteredExists := 0

	// Seed URLs to crawl - similar to xnxx seed strategy
	seeds := []string{
		xhBase + "/newest",
		xhBase + "/best/weekly",
		xhBase + "/best/monthly",
		xhBase + "/best/daily",
		xhBase + "/best",
		xhBase + "/top/this-month",
		xhBase + "/top/this-week",
	}

	for _, seed := range seeds {
		for page := 0; page < 10; page++ {
			pageURL := seed
			if page > 0 {
				pageURL = fmt.Sprintf("%s?page=%d", seed, page)
			}
			log.Printf("xHamster: scanning %s", pageURL)

			videos := scrapeXhListing(pageURL)
			if len(videos) == 0 {
				if page > 0 {
					break
				}
				continue
			}
			totalListingFound += len(videos)

			for _, v := range videos {
				if v.ID == "" || v.Slug == "" {
					totalFilteredEmpty++
					continue
				}

				var exists string
				db.QueryRow("SELECT id FROM videos WHERE id = ?", v.ID).Scan(&exists)
				if exists != "" {
					totalFilteredExists++
					continue
				}

				// Insert stub
				db.Exec(`INSERT OR IGNORE INTO videos (id, slug, title, source, added_at) VALUES (?,?,?,?,?)`,
					v.ID, v.Slug, v.Title, "xhamster", v.AddedAt)

				// Scrape details
				detail, err := scrapeXhVideoDetail(v.ID)
				if err != nil {
					log.Printf("xHamster detail scrape %s failed: %v", v.ID, err)
					v.Source = "xhamster"
					storeVideo(v)
					recordScrapeFailure(v.ID, err)
					continue
				}
				storeVideo(detail)
				totalNew++
				log.Printf("xHamster: new video %s: %s", v.ID, detail.Title)
			}
		}
	}

	log.Printf("xHamster crawl complete: %d new videos scraped (listing found: %d, filtered empty ID/Slug: %d, filtered existing: %d)",
		totalNew, totalListingFound, totalFilteredEmpty, totalFilteredExists)
}

// --- EPorner Scraper ---

var (
	reEpVideoBlock  = regexp.MustCompile(`data-id="(\d+)"[^>]*>\s*<div class="mbimg">.*?<a href="([^"]*)"[^>]*>\s*<img src="([^"]*)"[^>]*>\s*</a>\s*(?:<div class="mvhdico"[^>]*><span>([^<]*)</span></div>)?.*?<p class="mbtit"><a[^>]*>([^<]*)</a></p>\s*<p class="mbstats">\s*<span class="mbtim"[^>]*>([^<]*)</span>\s*(?:<span class="mbrate"[^>]*>([^<]*)</span>)?\s*<span class="mbvie"[^>]*>([^<]*)</span>(?:\s*<span class="mb-uploader"><a[^>]*>([^<]*)</a></span>)?`)
	reEpHashSlug    = regexp.MustCompile(`/video-([^/]+)/([^/]*)/?$`)
	reEpMetaDesc    = regexp.MustCompile(`<meta name="description" content="([^"]*)"`)
	reEpCatLinks    = regexp.MustCompile(`<a[^>]*href="/category/([^"]*)/"[^>]*title="([^"]*)"`)
	reEpStarLinks   = regexp.MustCompile(`<a[^>]*href="/pornstar/([^"]*)/"[^>]*>([^<]*)</a>`)
	reEpVideoURLs   = regexp.MustCompile(`https?://[^"'\s<>]*?xvideos\.com/video[^"'\s<>]*|/dload/[^"'\s<>]*|"embedUrl"\s*:\s*"([^"]*)"`)
	reEpDurationSec = regexp.MustCompile(`(\d+)\s*min`)
	reEpRdate       = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`)
)

func httpGetEpWithRetry(urlStr string) (*http.Response, error) {
	<-rateLimitEp
	var lastErr error
	for attempt := 0; attempt < maxHTTPRetries; attempt++ {
		if attempt > 0 {
			delay := retryBaseDelay * time.Duration(1<<(attempt-1))
			if delay > retryMaxDelay {
				delay = retryMaxDelay
			}
			time.Sleep(delay + time.Duration(rand.Intn(1000))*time.Millisecond)
		}
		req, _ := http.NewRequest("GET", urlStr, nil)
		ua := userAgents[rand.Intn(len(userAgents))]
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			resp.Body.Close()
			if attempt == maxHTTPRetries-1 {
				return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
			}
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("EPorner request failed: %w", lastErr)
}

func scrapeEpListing(pageURL string) []Video {
	resp, err := httpGetEpWithRetry(pageURL)
	if err != nil {
		log.Printf("EPorner listing %s failed: %v", pageURL, err)
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	resp.Body.Close()
	bodyStr := string(body)

	videos := []Video{}
	seen := map[string]bool{}

	blocks := reEpVideoBlock.FindAllStringSubmatch(bodyStr, -1)
	for _, m := range blocks {
		if len(m) < 8 {
			continue
		}
		id := m[1]
		href := m[2]
		thumbURL := m[3]
		quality := m[4]
		title := strings.TrimSpace(m[5])
		durationStr := m[6]
		rating := m[7]
		viewsStr := m[8]
		uploader := ""
		if len(m) > 9 {
			uploader = m[9]
		}

		hm := reEpHashSlug.FindStringSubmatch(href)
		if hm == nil || seen[id] {
			continue
		}
		hash := hm[1]
		slug := hm[2]
		seen[id] = true

		dur := parseEpDuration(durationStr)
		views := parseViews(viewsStr)

		v := Video{
			ID:        hash,
			Slug:      slug,
			Title:     title,
			ThumbUUID: thumbURL,
			Source:    "eporner",
			Duration:  dur,
			Views:     views,
			Uploader:  uploader,
			AddedAt:   time.Now().Format("2006-01-02"),
		}
		_ = quality
		_ = rating
		videos = append(videos, v)
	}
	return videos
}

func parseEpDuration(dur string) int {
	parts := strings.Split(dur, ":")
	if len(parts) == 3 {
		h, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		s, _ := strconv.Atoi(parts[2])
		return h*3600 + m*60 + s
	}
	if len(parts) == 2 {
		m, _ := strconv.Atoi(parts[0])
		s, _ := strconv.Atoi(parts[1])
		return m*60 + s
	}
	return 0
}

func parseViews(viewsStr string) int {
	s := strings.ToLower(strings.ReplaceAll(viewsStr, ",", ""))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.TrimSuffix(s, "k")
	if strings.HasSuffix(s, "m") {
		s = strings.TrimSuffix(s, "m")
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int(f * 1_000_000)
		}
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int(f)
	}
	return 0
}

func scrapeEpVideoDetail(hash string) (Video, error) {
	v := Video{ID: hash, Source: "eporner"}

	url := epBase + "/video-" + hash + "/"
	resp, err := httpGetEpWithRetry(url)
	if err != nil {
		return v, err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	resp.Body.Close()
	bodyStr := string(body)

	if m := reEpMetaDesc.FindStringSubmatch(bodyStr); len(m) > 1 {
		desc := m[1]
		v.Description = desc
		if strings.Contains(desc, "Starring:") {
			parts := strings.SplitN(desc, ". Starring:", 2)
			if len(parts) > 1 {
				starPart := strings.SplitN(parts[1], ". Duration:", 2)[0]
				v.Tags = append(v.Tags, strings.Split(strings.TrimSpace(starPart), ", ")...)
			}
		}
	}

	if m := reEpDurationSec.FindStringSubmatch(bodyStr); len(m) > 1 {
		v.Duration, _ = strconv.Atoi(m[1])
		v.Duration *= 60
	}

	if m := reEpRdate.FindStringSubmatch(bodyStr); len(m) > 1 {
		v.UploadDate = m[1]
	}

	// Categories
	catMatches := reEpCatLinks.FindAllStringSubmatch(bodyStr, -1)
	for _, cm := range catMatches {
		if len(cm) > 2 {
			v.Categories = append(v.Categories, cm[2])
		}
	}
	// Pornstars as tags
	starMatches := reEpStarLinks.FindAllStringSubmatch(bodyStr, -1)
	for _, sm := range starMatches {
		if len(sm) > 2 {
			v.Tags = append(v.Tags, strings.TrimSpace(sm[2]))
		}
	}

	// Extract title from og:title or h1
	if m := regexp.MustCompile(`<meta property="og:title" content="([^"]*)"`).FindStringSubmatch(bodyStr); len(m) > 1 {
		v.Title = strings.TrimSuffix(strings.TrimSpace(m[1]), " - EPORNER")
	} else if m := regexp.MustCompile(`<h1[^>]*>([^<]*)</h1>`).FindStringSubmatch(bodyStr); len(m) > 1 {
		v.Title = strings.TrimSpace(m[1])
	}

	// Thumbnail from og:image or page img
	if m := regexp.MustCompile(`<meta property="og:image" content="([^"]*)"`).FindStringSubmatch(bodyStr); len(m) > 1 {
		v.ThumbUUID = m[1]
	}

	v.AddedAt = time.Now().Format("2006-01-02")

	// Best-effort video URL extraction from page
	if m := regexp.MustCompile(`(?i)src\s*=\s*["']([^"']*\.mp4[^"']*)["']`).FindStringSubmatch(bodyStr); len(m) > 1 {
		assignMP4Quality(&v, m[1])
	}
	if m := regexp.MustCompile(`(?i)(https?://[^"'\s<>]*?\.mp4[^"'\s<>]*)`).FindStringSubmatch(bodyStr); len(m) > 1 {
		assignMP4Quality(&v, m[1])
	}
	if m := regexp.MustCompile(`"contentUrl"\s*:\s*"([^"]*)"`).FindStringSubmatch(bodyStr); len(m) > 1 {
		assignMP4Quality(&v, m[1])
	}
	if m := regexp.MustCompile(`"embedUrl"\s*:\s*"([^"]*)"`).FindStringSubmatch(bodyStr); len(m) > 1 {
		v.HLSURL = m[1]
	}

	return v, nil
}

func runEpCrawl() {
	if !crawlMuEp.TryLock() {
		log.Println("EPorner crawl already running")
		return
	}
	defer crawlMuEp.Unlock()

	log.Println("Starting EPorner crawl...")
	totalNew := 0
	seen := map[string]bool{}

	seedCatsForCrawl := []string{
		"", "hd-porn", "4k-porn", "60fps-porn", "popular",
		"teen", "milf", "anal", "blowjob", "lesbian", "big-tits",
		"ebony", "latina", "asian", "amateur", "creampie", "cumshot",
		"group", "public", "pov", "bbw", "cartoon", "squirting",
	}
	for _, cat := range seedCatsForCrawl {
		for page := 0; page < 20; page++ {
			pageURL := epBase + "/"
			if cat != "" {
				pageURL = epBase + "/" + cat + "/"
			}
			if page > 0 {
				if cat != "" {
					pageURL = fmt.Sprintf("%s/%d/", epBase+"/"+cat, page+1)
				} else {
					pageURL = fmt.Sprintf("%s/%d/", epBase, page+1)
				}
			}

			log.Printf("EPorner: scanning %s", pageURL)

			videos := scrapeEpListing(pageURL)
			if len(videos) == 0 {
				if page > 0 {
					break
				}
				continue
			}

			for _, v := range videos {
				if v.ID == "" || seen[v.ID] {
					continue
				}
				seen[v.ID] = true

				var exists string
				db.QueryRow("SELECT id FROM videos WHERE id = ?", v.ID).Scan(&exists)
				if exists != "" {
					continue
				}

				cats := strings.Join(extractCategories(v.Title, v.Description, v.Tags), ",")
				db.Exec(`INSERT OR IGNORE INTO videos (id, slug, title, description, categories, tags, uploader, duration, views, source, thumb_uuid, added_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
					v.ID, v.Slug, v.Title, v.Description, cats, strings.Join(v.Tags, ","), v.Uploader, v.Duration, v.Views, "eporner", v.ThumbUUID, v.AddedAt)

				detail, err := scrapeEpVideoDetail(v.ID)
				if err != nil {
					log.Printf("EPorner detail scrape %s failed: %v", v.ID, err)
					v.Source = "eporner"
					storeVideo(v)
					continue
				}
				storeVideo(detail)
				totalNew++
			}
		}
	}
	log.Printf("EPorner crawl complete: %d new videos scraped", totalNew)
}

func handleAPICrawlEp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	go runEpCrawl()
	http.Redirect(w, r, "/", 302)
}

func scrapeNewVideoDetails() {
	now := time.Now().Unix()
	rows, err := db.Query(`SELECT v.id, COALESCE(v.source, 'xnxx')
		FROM videos v
		LEFT JOIN scrape_failures f ON f.video_id = v.id
		WHERE (
			COALESCE(v.url_360,'') = ''
			AND COALESCE(v.url_720,'') = ''
			AND COALESCE(v.url_1080,'') = ''
			AND COALESCE(v.hls_url,'') = ''
		)
		AND (f.video_id IS NULL OR f.next_retry_at <= ? OR f.retry_count >= ?)
		ORDER BY COALESCE(f.retry_count, 0) ASC, v.added_at DESC
		LIMIT ?`, now, maxScrapeFailuresBeforeDelete, backfillBatchSize)
	if err != nil {
		log.Printf("Query for unscraped videos failed: %v", err)
		return
	}
	defer rows.Close()

	type pending struct{ id, source string }
	var pendingList []pending
	for rows.Next() {
		var id, source string
		rows.Scan(&id, &source)
		if !isPlayableSource(source) {
			// Non-playable sources (eporner, drtuber, tnaflix) don't provide
			// server-side media URLs. Skip retries to avoid infinite failure loop.
			clearScrapeFailure(id)
			continue
		}
		pendingList = append(pendingList, pending{id, source})
	}
	rows.Close()

	if len(pendingList) == 0 {
		return
	}

	bgWg.Add(1)
	defer bgWg.Done()
	setProgress("backfill", "scraping", 0, len(pendingList), 0, 0, len(pendingList), 0)

	log.Printf("Scraping details for %d videos (max %d concurrent)...", len(pendingList), scrapeWorkers)
	var wg sync.WaitGroup
	for i, p := range pendingList {
		if i > 0 {
			time.Sleep(time.Duration(500+rand.Intn(1500)) * time.Millisecond)
		}
		wg.Add(1)
		scrapeSem <- struct{}{}
		go func(id, source string) {
			defer wg.Done()
			defer func() { <-scrapeSem }()
			detail, err := scrapeVideoDetailForSource(id, source)
			if err != nil {
				log.Printf("Detail scrape failed for %s (%s): %v", id, source, err)
				recordScrapeFailure(id, err)
				time.Sleep(2 * time.Second)
				return
			}
			normalizeVideoExpiry(&detail)
			if !hasPlayableMedia(detail) {
				err := fmt.Errorf("%s detail scrape returned no playable media", source)
				log.Printf("Detail scrape found no media for %s (%s)", id, source)
				recordScrapeFailure(id, err)
				return
			}
			storeVideo(detail)
			clearScrapeFailure(id)
			progress.mu.Lock()
			progress.DetailDone++
			d := progress.DetailDone
			t := progress.DetailTotal
			progress.mu.Unlock()
			if d%10 == 0 {
				log.Printf("Detail scrape progress: %d/%d", d, t)
			}
		}(p.id, p.source)
	}
	wg.Wait()
	log.Printf("Background detail scraping complete (%d videos)", len(pendingList))

	setProgress("backfill", "idle", 0, 0, 0, 0, 0, 0)
}

// --- HTTP helpers ---

func httpGetWithRetry(urlStr string) (*http.Response, error) {
	<-rateLimiter

	var lastErr error
	for attempt := 0; attempt < maxHTTPRetries; attempt++ {
		if attempt > 0 {
			delay := retryBaseDelay * time.Duration(1<<(attempt-1))
			if delay > retryMaxDelay {
				delay = retryMaxDelay
			}
			jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
			time.Sleep(delay + jitter)
		}
		req, errReq := http.NewRequest("GET", urlStr, nil)
		if errReq != nil {
			return nil, errReq
		}
		ua := userAgents[rand.Intn(len(userAgents))]
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")
		req.Header.Set("Referer", xnxxBase+"/")

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			log.Printf("HTTP attempt %d failed: %v", attempt+1, err)
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			log.Printf("HTTP %d on attempt %d: %s", resp.StatusCode, attempt+1, strings.TrimSpace(string(body)))
			if attempt == maxHTTPRetries-1 {
				return nil, lastErr
			}
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("request failed after %d attempts: %w", maxHTTPRetries, lastErr)
}

func recordScrapeFailure(videoID string, scrapeErr error) {
	if isPermanentScrapeFailure(scrapeErr) {
		pruneFailedVideoIfUnplayable(videoID, scrapeErr.Error())
		if _, ok := loadVideoFromDB(videoID); !ok {
			return
		}
	}
	var retryCount int
	db.QueryRow("SELECT retry_count FROM scrape_failures WHERE video_id = ?", videoID).Scan(&retryCount)
	nextCount := retryCount + 1
	delay := int64(failureBaseDelay.Seconds())
	for i := 0; i < retryCount; i++ {
		delay *= 2
		if delay > int64(failureMaxDelay.Seconds()) {
			delay = int64(failureMaxDelay.Seconds())
			break
		}
	}
	nextRetry := time.Now().Unix() + delay
	db.Exec("INSERT OR REPLACE INTO scrape_failures (video_id, retry_count, last_error, next_retry_at) VALUES (?,?,?,?)",
		videoID, nextCount, scrapeErr.Error(), nextRetry)
	if nextCount >= maxScrapeFailuresBeforeDelete {
		pruneFailedVideoIfUnplayable(videoID, scrapeErr.Error())
	}
}

func clearScrapeFailure(videoID string) {
	db.Exec("DELETE FROM scrape_failures WHERE video_id = ?", videoID)
}

func isPermanentScrapeFailure(scrapeErr error) bool {
	if scrapeErr == nil {
		return false
	}
	msg := strings.ToLower(scrapeErr.Error())
	return strings.Contains(msg, "redirected off-site to www.xnxx.gold")
}

func retryFailedScrapes() {
	now := time.Now().Unix()
	rows, err := db.Query("SELECT video_id, retry_count FROM scrape_failures WHERE next_retry_at <= ? ORDER BY retry_count ASC LIMIT ?", now, maxFailuresPerBatch)
	if err != nil {
		return
	}
	defer rows.Close()

	type retryEntry struct {
		id    string
		count int
	}
	var entries []retryEntry
	for rows.Next() {
		var e retryEntry
		rows.Scan(&e.id, &e.count)
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return
	}

	log.Printf("Retrying %d previously failed scrapes...", len(entries))
	for _, e := range entries {
		time.Sleep(time.Duration(500+rand.Intn(1500)) * time.Millisecond)
		v, ok := loadVideoFromDB(e.id)
		if !ok {
			clearScrapeFailure(e.id)
			continue
		}
		if e.count >= maxScrapeFailuresBeforeDelete {
			pruneFailedVideoIfUnplayable(e.id, "retry limit reached")
			if _, ok := loadVideoFromDB(e.id); !ok {
				continue
			}
		}
		refreshed, err := ensureFreshVideo(v, tokenRefreshLead)
		if err != nil {
			log.Printf("Retry failed for %s (%s, attempt %d): %v", e.id, v.Source, e.count+1, err)
			continue
		}
		clearScrapeFailure(e.id)
		log.Printf("Retry succeeded for %s (%s)", refreshed.ID, refreshed.Source)
	}
}

func retryFailedLoop(ctx context.Context) {
	ticker := time.NewTicker(retryFailedEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			retryFailedScrapes()
		}
	}
}

// --- Cache ---

func getCachedVideo(id string) (Video, bool) {
	if val, ok := videoCache.Load(id); ok {
		entry := val.(cacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.video, true
		}
		videoCache.Delete(id)
	}
	return Video{}, false
}

func setCachedVideo(id string, v Video) {
	videoCache.Store(id, cacheEntry{
		video:     v,
		expiresAt: time.Now().Add(cacheTTL),
	})
}

// --- Helpers ---

func sanitizeFTSQuery(q string) string {
	replacer := strings.NewReplacer(
		`"`, ` `, `'`, ` `, `(`, ` `, `)`, ` `,
		`*`, ` `, `-`, ` `, `+`, ` `, `~`, ` `,
		`^`, ` `, `:`, ` `,
	)
	terms := strings.Fields(replacer.Replace(q))
	if len(terms) == 0 {
		return ""
	}
	for i, t := range terms {
		terms[i] = t + "*"
	}
	return strings.Join(terms, " ")
}

func escapeLikePattern(term string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(term)
}

func existingVideoIDSet(ids []string) map[string]bool {
	existing := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return existing
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := db.Query("SELECT id FROM videos WHERE id IN ("+placeholders+")", args...)
	if err != nil {
		log.Printf("existing video lookup failed: %v", err)
		return existing
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			log.Printf("existing video scan failed: %v", err)
			continue
		}
		existing[id] = true
	}
	if err := rows.Err(); err != nil {
		log.Printf("existing video rows error: %v", err)
	}
	return existing
}

func storeExistingStubVideo(id string) {
	var v Video
	var categories, tags string
	err := db.QueryRow(`SELECT id, COALESCE(slug,''), COALESCE(title,''), COALESCE(description,''), COALESCE(categories,''), COALESCE(tags,''), COALESCE(uploader,''), COALESCE(upload_date,''), COALESCE(duration,0), COALESCE(views,0), COALESCE(added_at,''), COALESCE(source,'xnxx'), COALESCE(thumb_uuid,''), COALESCE(preview_url,'')
		FROM videos WHERE id = ?`, id).
		Scan(&v.ID, &v.Slug, &v.Title, &v.Description, &categories, &tags, &v.Uploader, &v.UploadDate, &v.Duration, &v.Views, &v.AddedAt, &v.Source, &v.ThumbUUID, &v.PreviewURL)
	if err != nil {
		log.Printf("load stub video %s failed: %v", id, err)
		return
	}
	if categories != "" {
		v.Categories = strings.Split(categories, ",")
	}
	if tags != "" {
		v.Tags = strings.Split(tags, ",")
	}
	storeVideo(v)
}

var tagToCat = map[string]string{
	"pov": "pov", "amateur": "homemade", "anal": "anal", "blowjob": "blowjob",
	"teen": "teen", "milf": "milf", "creampie": "creampie", "cumshot": "cumshot",
	"facial": "cumshot", "cum in mouth": "cumshot", "cum-inside": "creampie",
	"big tits": "big-tits", "busty": "big-tits", "huge tits": "big-tits",
	"big titt": "big-tits", "boobs": "big-tits", "titty": "big-tits",
	"big ass": "big-ass", "big-ass": "big-ass", "ass": "anal",
	"bbw": "bbw", "chubby": "bbw", "plus size": "bbw",
	"lesbian": "lesbian", "squirt": "squirting", "squirting": "squirting",
	"compilation": "compilation", "handjob": "handjob", "deepthroat": "blowjob",
	"69": "sixty-nine", "sixty-nine": "sixty-nine", "rimming": "anal",
	"doggystyle": "doggystyle", "cowgirl": "cowgirl", "missionary": "missionary",
	"gangbang": "group", "orgy": "group", "threesome": "group", "foursome": "group",
	"bbc": "bbc", "interracial": "bbc", "ebony": "bbc", "black": "bbc",
	"latina": "latina", "latin": "latina", "spanish": "latina",
	"asian": "asian", "japanese": "asian", "chinese": "asian", "korean": "asian",
	"thai": "asian", "indian": "indian", "arab": "arab",
	"rough": "rough", "hardcore": "rough", "brutal": "rough",
	"outdoor": "outdoor", "public": "outdoor", "outside": "outdoor", "beach": "outdoor", "park": "outdoor", "nature": "outdoor",
	"homemade": "homemade", "real": "homemade", "webcam": "homemade",
	"mature": "milf", "granny": "milf", "cougar": "milf", "mom": "milf",
	"young": "teen", "teenager": "teen", "college": "teen", "school": "teen", "18yo": "teen",
	"cartoon": "cartoon", "hentai": "cartoon", "anime": "cartoon", "manga": "cartoon",
	"3d": "cartoon", "cgi": "cartoon", "animation": "cartoon",
	"bdsm": "bdsm", "bondage": "bdsm", "fetish": "fetish", "latex": "fetish",
	"cosplay": "cosplay", "roleplay": "cosplay", "costume": "cosplay",
	"massage": "massage", "strip": "strip", "striptease": "strip",
	"vintage": "vintage", "retro": "vintage", "classic": "vintage",
	"casting": "casting", "audition": "casting",
	"hidden-cam": "hidden-cam", "spy": "hidden-cam", "voyeur": "hidden-cam",
	"upskirt": "hidden-cam", "downblouse": "hidden-cam",
	"transgender": "transgender", "shemale": "transgender", "ts": "transgender",
	"hairy": "hairy", "shaved": "shaved",
	"tattoo": "tattooed", "tattooed": "tattooed", "pierced": "pierced",
	"natural": "natural-tits", "natural tits": "natural-tits",
	"small tits": "small-tits", "small-tits": "small-tits",
	"skinny": "skinny", "fitness": "fitness", "muscular": "muscular",
	"german": "european", "french": "european", "italian": "european",
	"russian": "european", "czech": "european", "dutch": "european",
	"polish": "european", "swedish": "european", "british": "european",
	"solo": "solo", "solo-male": "solo", "solo-female": "solo",
	"double-penetration": "anal", "dp": "anal", "fisting": "fetish",
	"pissing": "fetish", "golden shower": "fetish",
	"party": "party", "wedding": "party",
	"toy": "toy", "dildo": "toy", "vibrator": "toy", "machine": "toy",
	"romantic": "romantic", "softcore": "softcore",
	"pornstar": "pornstar", "premium": "pornstar",
	"reality": "reality", "fake": "reality", "fake taxi": "reality",
	"comedy": "comedy", "parody": "parody", "spoof": "parody",
	"masturbation": "solo", "dancing": "dancing", "pole dance": "dancing",
	"shower": "shower", "bath": "shower", "sauna": "shower",
	"bedroom": "bedroom", "kitchen": "bedroom", "office": "bedroom",
	"car": "car", "truck": "car", "train": "car", "boat": "car",
	"military": "uniform", "police": "uniform", "nurse": "uniform",
	"teacher": "uniform", "student": "uniform", "maid": "uniform",
	"secretary": "uniform", "cheerleader": "uniform",
	"zombie": "fantasy", "vampire": "fantasy", "monster": "fantasy",
	"angel": "fantasy", "devil": "fantasy", "demon": "fantasy",
	"superhero": "fantasy", "super hero": "fantasy",
	"step-sister": "step-family", "stepsister": "step-family", "step-sis": "step-family",
	"step-mom": "step-family", "stepmom": "step-family", "step-mother": "step-family",
	"step-daughter": "step-family", "stepdaughter": "step-family", "step-son": "step-family",
	"step-brother": "step-family", "stepbrother": "step-family", "step-dad": "step-family",
	"porn":    "pornstar",
	"cuckold": "cuckold", "cuck": "cuckold", "cuckquean": "cuckold",
	"feet": "feet", "foot": "feet", "footjob": "feet", "toes": "feet",
	"stockings": "stockings", "pantyhose": "stockings", "nylons": "stockings",
	"glasses": "glasses", "eyeglasses": "glasses", "spectacles": "glasses",
	"pregnant": "pregnant", "preggo": "pregnant", "belly": "pregnant",
	"blonde": "blonde", "blond": "blonde", "brunette": "blonde", "redhead": "redhead",
	"big-dick": "big-dick", "big cock": "big-dick", "big dick": "big-dick",
	"huge cock": "big-dick", "monster cock": "big-dick", "large cock": "big-dick",
	"thick cock": "big-dick", "bwc": "big-dick", "bubble butt": "big-ass",
	"pawg": "big-ass", "phat ass": "big-ass",
	"curvy": "bbw", "plus-size": "bbw", "fat": "bbw", "thick": "bbw",
	"petite": "skinny", "slender": "skinny",
	"muscle": "muscular", "muscles": "muscular",
	"fit": "fitness", "athletic": "fitness", "workout": "fitness", "gym": "fitness", "yoga": "fitness",
	"lingerie": "lingerie", "lace": "lingerie", "corset": "lingerie",
	"panties": "panties", "underwear": "panties",
	"high heels": "heels", "heels": "heels", "stilettos": "heels",
	"blowbang": "group", "gokkun": "cumshot",
	"gaping": "anal", "ass to mouth": "anal", "atm": "anal", "pegging": "anal",
	"strap-on": "anal", "strapon": "anal", "buttplug": "anal",
	"facesitting": "fetish", "smoking": "fetish", "pvc": "fetish", "leather": "fetish",
	"balloon": "fetish", "puppy": "fetish", "pet play": "fetish",
	"choking": "rough", "choke": "rough", "slapping": "rough",
	"gagging": "blowjob", "gag": "blowjob", "throatfuck": "blowjob",
	"facefuck": "blowjob", "gloryhole": "blowjob",
	"glory hole": "blowjob", "throat": "blowjob",
	"bisexual": "group", "bi": "group",
	"twink": "gay", "gay": "gay", "homo": "gay",
	"sissy": "transgender", "crossdresser": "transgender", "cd": "transgender",
	"trap": "transgender", "trans": "transgender",
	"vr": "vr", "virtual reality": "vr", "360": "vr",
	"celebrity": "celebrity", "celeb": "celebrity", "famous": "celebrity",
	"christmas": "holiday", "halloween": "holiday", "holiday": "holiday",
	"valentine": "holiday", "easter": "holiday",
	"orgasm": "orgasm", "multiple orgasm": "orgasm",
	"lactating": "lactation", "lactation": "lactation", "breastfeeding": "lactation",
	"milk": "lactation", "hucow": "lactation",
	"sleeping": "sleeping", "hypnosis": "sleeping", "hypno": "sleeping",
	"neighbor": "reality", "neighbour": "reality",
	"verified":    "homemade",
	"big nipples": "fetish", "nipple": "fetish", "nipples": "fetish",
	"puffy nipples": "fetish",
	"prostate":      "solo", "male solo": "solo", "jack off": "solo", "jerking": "solo",
	"dogging": "outdoor", "exhibition": "outdoor", "exhibitionist": "outdoor",
	"goth": "alternative", "alt": "alternative", "emo": "alternative", "punk": "alternative",
	"piercing": "pierced",
	"bald":     "shaved", "hairless": "shaved",
	"mommy": "milf", "matures": "milf",
	"fresh":     "teen",
	"big boobs": "big-tits", "boob": "big-tits",
	"huge boobs": "big-tits", "massive": "big-tits",
	"booty":   "big-ass",
	"mexican": "latina", "brazilian": "latina",
	"blow job": "blowjob", "bj": "blowjob",
	"hand job":       "handjob",
	"black on white": "bbc",
	"filipino":       "asian",
	"vietnamese":     "asian",
	"rough sex":      "rough",
	"home video":     "homemade", "selfie": "homemade",
	"old": "milf", "older": "milf",
	"cg":         "cartoon",
	"domination": "bdsm", "submission": "bdsm",
	"hidden camera": "hidden-cam", "spy cam": "hidden-cam", "spycam": "hidden-cam",
	"caught":     "hidden-cam",
	"ladyboy":    "transgender",
	"bush":       "hairy",
	"au naturel": "natural-tits",
	"a cups":     "small-tits", "flat chest": "small-tits",
	"middle eastern": "arab",
	"solo male":      "solo", "solo girl": "solo",
	"watersports": "fetish",
	"sex machine": "toy",
	"bathtub":     "shower",
}

var categoryKeywords = []struct {
	Cat      string
	Keywords []string
}{
	{"anal", []string{"anal", "anal sex", "in the ass", "anal creampie", "anal fisting", " anal "}},
	{"teen", []string{" teen ", " teenager ", "18 year", "18yo", "young girl", "college", "schoolgirl", "school girl"}},
	{"milf", []string{"milf", "mom ", " mother ", " mature ", " granny ", "cougar", "housewife"}},
	{"blowjob", []string{"blowjob", "deep throat", "deepthroat", "oral sex", " sucking ", "face fuck", "head from"}},
	{"big-tits", []string{"big tits", "huge tits", "big titties", "busty", "big boob", "big tits"}},
	{"big-ass", []string{"big ass", "huge ass", "fat ass", "big booty", "big ass"}},
	{"homemade", []string{"amateur", "homemade", "home made", "real couple", "homemade"}},
	{"creampie", []string{"creampie", "cum inside", "internal cum", "cream pie", "creampie"}},
	{"cumshot", []string{"cumshot", "cum shot", "cum on", "facial", "cum in mouth", "swallow"}},
	{"compilation", []string{"compilation", "best of", "collection", "montage", "mega"}},
	{"pov", []string{"pov", "point of view", "first person"}},
	{"lesbian", []string{"lesbian", "girl on girl", "scissor", "lesbo", "sapphic"}},
	{"bbc", []string{"bbc", "big black", "interracial", "black cock", "black guy", "ebony"}},
	{"latina", []string{"latina", " latin ", "spanish", " hispanic"}},
	{"asian", []string{"asian", "japanese", "chinese", "korean", "thai girl"}},
	{"indian", []string{" indian "}},
	{"rough", []string{"rough", "hardcore", "brutal", "violent", "intense"}},
	{"group", []string{"orgy", "gangbang", "threesome", "foursome", "group sex", "dp "}},
	{"outdoor", []string{"outdoor", "public", "outside", "beach", "park ", "nature", "garden"}},
	{"handjob", []string{"handjob", "hand job", "handjob"}},
	{"squirting", []string{"squirt", "squirting", "female ejac", "squirt"}},
	{"cartoon", []string{"cartoon", "hentai", "anime", "3d animation", "cgi"}},
	{"bdsm", []string{"bdsm", "bondage", "domination", "submissive", "spanking"}},
	{"fetish", []string{"fetish", "latex", "fisting", "pissing", "golden shower"}},
	{"cosplay", []string{"cosplay", "roleplay", "costume", "disguise"}},
	{"massage", []string{"massage", "erotic massage", "body rub"}},
	{"vintage", []string{"vintage", "retro", "classic", "80s", "90s"}},
	{"european", []string{"german", "french", "italian", "russian", "czech", "europe"}},
	{"transgender", []string{"transgender", "shemale", " trans ", "tgirl", "ts "}},
	{"casting", []string{"casting", "audition"}},
	{"hidden-cam", []string{"hidden", "spy", "voyeur", "upskirt"}},
	{"tattooed", []string{"tattoo", "tattooed", "inked"}},
	{"hairy", []string{"hairy", "bush", "hairless"}},
	{"toy", []string{"dildo", "vibrator", "sex toy", "fuck machine"}},
	{"shower", []string{"shower", "bath", "bathroom"}},
	{"party", []string{"party", "spring break", "bachelor"}},
	{"uniform", []string{"uniform", "military", "police", "nurse", "teacher", "maid", "cheerleader"}},
	{"fantasy", []string{"fantasy", "vampire", "zombie", "monster", "superhero"}},
	{"parody", []string{"parody", "spoof"}},
	{"solo", []string{"solo", "masturbation"}},
	{"doggystyle", []string{"doggystyle", "doggy style"}},
	{"cowgirl", []string{"cowgirl", "reverse cowgirl"}},
	{"sixty-nine", []string{"69", "sixty nine"}},
	{"strip", []string{"strip", "striptease", "lap dance"}},
	{"pornstar", []string{"pornstar"}},
	{"reality", []string{"reality", "fake taxi", "fake agent"}},
	{"step-family", []string{"step sis", "step mom", "step sister", "step mother", "step daughter", "step brother", "step dad", "step family"}},
	{"cuckold", []string{"cuckold", "cuck", "cuckquean", "husband watches"}},
	{"feet", []string{"feet", "footjob", "toes", "foot fetish"}},
	{"stockings", []string{"stockings", "pantyhose", "nylons", "fishnets"}},
	{"glasses", []string{"glasses", "eyeglasses", "spectacles", "nerdy"}},
	{"pregnant", []string{"pregnant", "preggo", "expecting", "baby bump"}},
	{"blonde", []string{"blonde", "blond", "blondes", "blonde girl"}},
	{"redhead", []string{"redhead", "red hair", "ginger"}},
	{"big-dick", []string{"big cock", "huge cock", "monster cock", "big dick", "large cock", "thick cock", "bwc"}},
	{"orgasm", []string{"orgasm", "multiple orgasm", "intense orgasm", "squirt", "cum hard"}},
	{"lactation", []string{"lactation", "lactating", "breastfeeding", "milk", "hucow"}},
	{"sleeping", []string{"sleeping", "asleep", "hypnosis", "hypno", "somno"}},
	{"alternative", []string{"alternative", "goth", "emo", "punk", "alt girl"}},
	{"pierced", []string{"pierced", "piercing", "tongue ring", "belly ring"}},
	{"lingerie", []string{"lingerie", "lace", "corset", "babydoll"}},
	{"panties", []string{"panties", "underwear", "thong", "g-string", "g string"}},
	{"heels", []string{"high heels", "heels", "stilettos", "platform heels"}},
	{"vr", []string{"vr", "virtual reality", "360 video", "oculus"}},
	{"celebrity", []string{"celebrity", "celeb", "famous"}},
	{"holiday", []string{"christmas", "halloween", "valentine", "easter", "holiday"}},
	{"gay", []string{"gay", "twink", "gay sex", "daddy"}},
	{"car", []string{"car", "in car", "backseat", "drive"}},
	{"bedroom", []string{"bedroom", "kitchen", "living room", "couch"}},
	{"muscular", []string{"muscle", "muscular", "bodybuilder", "muscle worship"}},
	{"fitness", []string{"fitness", "workout", "gym", "yoga", "athletic", "gym girl"}},
	{"skinny", []string{"skinny", "petite", "slender", "slim", "thin"}},
	{"small-tits", []string{"small tits", "small boobs", "a cup", "flat chest", "tiny tits"}},
	{"natural-tits", []string{"natural tits", "natural boobs", "au naturel"}},
	{"arab", []string{"arab", "middle eastern", "egyptian", "moroccan"}},
	{"orgy", []string{"orgy", "orgie"}},
	{"gaping", []string{"gaping", "gaped", "gapes"}},
	{"pegging", []string{"pegging", "strap on", "strap-on"}},
	{"choking", []string{"choking", "choke", "choked", "strangle"}},
	{"slapping", []string{"slapping", "slap", "face slap", "spank"}},
	{"gloryhole", []string{"gloryhole", "glory hole", "anonymous blowjob"}},
	{"exhibition", []string{"exhibition", "exhibitionist", "flashing", "flasher"}},
}

func extractCategories(title, description string, tags []string) []string {
	text := strings.ToLower(title + " " + description)
	seen := map[string]bool{}
	found := []string{}

	// Tag-based matching (most accurate)
	for _, tag := range tags {
		lowTag := strings.ToLower(strings.TrimSpace(tag))
		if cat, ok := tagToCat[lowTag]; ok && !seen[cat] {
			seen[cat] = true
			found = append(found, cat)
		}
	}

	// Title/description keyword matching
	for _, entry := range categoryKeywords {
		if seen[entry.Cat] {
			continue
		}
		for _, kw := range entry.Keywords {
			if strings.Contains(text, kw) {
				seen[entry.Cat] = true
				found = append(found, entry.Cat)
				break
			}
		}
	}

	if len(found) == 0 {
		found = append(found, "uncategorized")
	}
	return found
}

func slugToTitle(slug string) string {
	title := strings.ReplaceAll(slug, "-", " ")
	title = strings.ReplaceAll(title, "_", " ")
	words := strings.Fields(title)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func parseDuration(iso string) int {
	iso = strings.TrimPrefix(iso, "PT")
	secs := 0
	if m := regexp.MustCompile(`(\d+)H`).FindStringSubmatch(iso); len(m) > 1 {
		fmt.Sscanf(m[1], "%d", &secs)
		secs *= 3600
	}
	if m := regexp.MustCompile(`(\d+)M`).FindStringSubmatch(iso); len(m) > 1 {
		var min int
		fmt.Sscanf(m[1], "%d", &min)
		secs += min * 60
	}
	if m := regexp.MustCompile(`(\d+)S`).FindStringSubmatch(iso); len(m) > 1 {
		var s int
		fmt.Sscanf(m[1], "%d", &s)
		secs += s
	}
	return secs
}
