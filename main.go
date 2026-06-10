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
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/PuerkitoBio/goquery"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed templates/*
var templateFS embed.FS

var (
	db         *sql.DB
	tmpl       *template.Template
	httpClient  *http.Client
	mediaClient *http.Client
	scrapeSem   chan struct{}
	videoCache sync.Map
	bgWg       sync.WaitGroup
	progress   CrawlProgress
	crawlMu    sync.Mutex
	crawlMuXh  sync.Mutex
	crawlMuEp  sync.Mutex
	crawlMuTf  sync.Mutex
	crawlMuDt  sync.Mutex
	catCache   catCacheT
	rateLimiter chan time.Time
	rateLimitXh chan time.Time
	rateLimitEp chan time.Time
	rateLimitTf chan time.Time
	rateLimitDt chan time.Time
	routeAPI    func(w http.ResponseWriter, r *http.Request)
	startTime   = time.Now()
	loginAttempts = make(map[string]*loginEntry)
	loginMu       sync.Mutex
)

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:126.0) Gecko/20100101 Firefox/126.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:126.0) Gecko/20100101 Firefox/126.0",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
}

type catCacheT struct {
	mu     sync.RWMutex
	cats   []string
	last   time.Time
}

type CrawlProgress struct {
	mu          sync.RWMutex
	Status      string `json:"status"`
	Source      string `json:"source"`
	Scanned     int    `json:"scanned"`
	NewVideos   int    `json:"new_videos"`
	Cached      int    `json:"cached"`
	DetailDone  int    `json:"detail_done"`
	DetailTotal int    `json:"detail_total"`
	Page        int    `json:"page"`
	TotalCount  int    `json:"total_count"`
	SourceCounts map[string]int `json:"source_counts"`
}

const (
	xnxxBase     = "https://www.xnxx.com"
	thumbCDN     = "https://thumb-cdn77.xnxx-cdn.com"
	thumbsCDN    = "https://thumbs-gcore.xnxx-cdn.com"
	mp4CDN       = "https://mp4-cdn77.xnxx-cdn.com"
	hlsCDN       = "https://hls-cdn77.xnxx-cdn.com"
	xhBase       = "https://xhamster.com"
	xhCDN        = "https://video3.xhcdn.com"
	epBase       = "https://www.eporner.com"
	epCDN        = "https://static-eu-cdn.eporner.com"
	dbPath       = "karaxxx.db"
	port         = "8799"
	scrapeWorkers = 5
	cacheTTL     = 5 * time.Minute
	refreshEvery   = 20 * time.Minute
	crawlLockPath  = "/tmp/karaxxx-crawl.lock"
	maxHTTPRetries    = 3
	retryBaseDelay    = 5 * time.Second
	retryMaxDelay     = 30 * time.Second
	rateLimitInterval = 400 * time.Millisecond
	failureBaseDelay  = 5 * time.Minute
	failureMaxDelay   = 6 * time.Hour
	maxFailuresPerBatch = 20
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
}

type cacheEntry struct {
	video     Video
	expiresAt time.Time
}

type loginEntry struct {
	attempts int
	until    time.Time
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func loadOrCreateJWTSecret() {
	secretFile := dbPath + ".jwt_secret"
	if data, err := os.ReadFile(secretFile); err == nil && len(data) == 64 {
		jwtSecret = string(data)
		log.Println("Loaded JWT secret from file")
		return
	}
	jwtSecret = randomHex(32)
	if err := os.WriteFile(secretFile, []byte(jwtSecret), 0600); err != nil {
		log.Printf("Warning: could not persist JWT secret: %v", err)
	} else {
		log.Println("Created new JWT secret")
	}
}

// --- Init ---

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	loadOrCreateJWTSecret()

	initHTTPClient()
	initDB()
	initTemplates()
	initRoutes()

	go refreshLoop(ctx)
	go scrapeNewVideoDetails()
	go retryFailedLoop(ctx)
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
			loginMu.Lock()
			now := time.Now()
			for ip, entry := range loginAttempts {
				if now.After(entry.until) {
					delete(loginAttempts, ip)
				}
			}
			loginMu.Unlock()
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
			select { case ch <- t: default: }
		}
	}()
	return ch
}

func initHTTPClient() {
	jar, _ := cookiejar.New(nil)

	tr := &http.Transport{
		MaxIdleConns:        20,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  false,
	}
	httpClient = &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
		Jar:       jar,
	}
	mediaTr := &http.Transport{
		MaxIdleConns:        5,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
		ResponseHeaderTimeout: 15 * time.Second,
	}
	mediaClient = &http.Client{
		Transport: mediaTr,
		Timeout:   0,
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
	rateLimitXh = newRateLimiter(rateLimitInterval)
	rateLimitEp = newRateLimiter(2 * time.Second) // EPorner is aggressive with 429s
	rateLimitTf = newRateLimiter(rateLimitInterval)
	rateLimitDt = newRateLimiter(rateLimitInterval)
}

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=OFF&cache=shared&_busy_timeout=5000")
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

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

	initCrawlSeeds()

	db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS videos_fts USING fts5(
		title, description, categories, tags,
		content='videos', content_rowid='rowid'
	)`)

	db.Exec(`CREATE TRIGGER IF NOT EXISTS videos_ai AFTER INSERT ON videos BEGIN
		INSERT INTO videos_fts(rowid, title, description, categories, tags)
		VALUES (new.rowid, new.title, new.description, new.categories, new.tags);
	END`)
	db.Exec(`CREATE TRIGGER IF NOT EXISTS videos_ad AFTER DELETE ON videos BEGIN
		INSERT INTO videos_fts(videos_fts, rowid, title, description, categories, tags)
		VALUES ('delete', old.rowid, old.title, old.description, old.categories, old.tags);
	END`)
	db.Exec(`CREATE TRIGGER IF NOT EXISTS videos_au AFTER UPDATE ON videos BEGIN
		INSERT INTO videos_fts(videos_fts, rowid, title, description, categories, tags)
		VALUES ('delete', old.rowid, old.title, old.description, old.categories, old.tags);
		INSERT INTO videos_fts(rowid, title, description, categories, tags)
		VALUES (new.rowid, new.title, new.description, new.categories, new.tags);
	END`)

	db.Exec(`INSERT OR IGNORE INTO videos_fts(rowid, title, description, categories, tags)
		SELECT rowid, title, description, categories, tags FROM videos`)

	// Auth tables
	db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		created_at TEXT DEFAULT (datetime('now'))
	)`)
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
		watched_at TEXT DEFAULT (datetime('now')),
		updated_at TEXT DEFAULT (datetime('now')),
		PRIMARY KEY (user_id, video_id),
		FOREIGN KEY (user_id) REFERENCES users(id),
		FOREIGN KEY (video_id) REFERENCES videos(id)
	)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_watch_history_user ON watch_history(user_id, updated_at DESC)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_watch_history_video ON watch_history(video_id)`)

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
	var pageCount, pageSize int
	db.QueryRow("SELECT page_count FROM pragma_page_count()").Scan(&pageCount)
	db.QueryRow("SELECT page_size FROM pragma_page_size()").Scan(&pageSize)
	dbSize := pageCount * pageSize

	walSize := int64(0)
	if fi, err := os.Stat(dbPath + "-wal"); err == nil {
		walSize = fi.Size()
	}

	var staleTokens int
	db.QueryRow("SELECT COUNT(*) FROM videos WHERE expires_at < unixepoch() AND expires_at > 0").Scan(&staleTokens)

	var failCount int
	db.QueryRow("SELECT COUNT(*) FROM scrape_failures").Scan(&failCount)

	videosBySource := map[string]int{}
	vrows, err := db.Query("SELECT source, COUNT(*) FROM videos GROUP BY source")
	if err == nil {
		defer vrows.Close()
		for vrows.Next() {
			var src string
			var cnt int
			vrows.Scan(&src, &cnt)
			videosBySource[src] = cnt
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"db_size_bytes":    dbSize,
		"wal_size_bytes":   walSize,
		"videos_by_source": videosBySource,
		"stale_tokens":     staleTokens,
		"scrape_failures":  failCount,
		"uptime_seconds":   int(time.Since(startTime).Seconds()),
		"goroutines":       runtime.NumGoroutine(),
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(rw, r)
		log.Printf("[%s] %s %s %d %s", r.Method, r.URL.Path, r.RemoteAddr, rw.statusCode, time.Since(start))
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
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"gt": func(a, b int) bool { return a > b },
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

func hashPassword(password string) string {
	salt := randomHex(16)
	h := sha256.Sum256([]byte(salt + password))
	return salt + ":" + hex.EncodeToString(h[:])
}

func checkPassword(password, stored string) bool {
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
	payload := fmt.Sprintf(`{"uid":%d,"un":"%s","exp":%d}`, userID, username, time.Now().Add(30*24*time.Hour).Unix())
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte(payload))
	signingInput := header + "." + payloadB64
	mac := hmac.New(sha256.New, []byte(jwtSecret))
	mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sig
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
		log.Printf("JWT: signature mismatch: got=%q want=%q", sig[:10], expectedSig[:10])
		return 0, "", false
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		log.Printf("JWT: payload decode error: %v", err)
		return 0, "", false
	}
	var claims struct {
		UID int    `json:"uid"`
		UN  string `json:"un"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		log.Printf("JWT: claims parse error: %v, payload=%s", err, string(payloadJSON))
		return 0, "", false
	}
	if time.Now().Unix() > claims.Exp {
		return 0, "", false
	}
	return claims.UID, claims.UN, true
}

func authMiddleware(w http.ResponseWriter, r *http.Request) (int, string, bool) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return 0, "", false
	}
	return parseToken(strings.TrimPrefix(auth, "Bearer "))
}

func handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" || body.Password == "" {
		http.Error(w, `{"error":"username and password required"}`, 400)
		return
	}
	if len(body.Password) < 4 {
		http.Error(w, `{"error":"password too short"}`, 400)
		return
	}
	hash := hashPassword(body.Password)
	res, err := db.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", body.Username, hash)
	if err != nil {
		http.Error(w, `{"error":"username taken"}`, 409)
		return
	}
	id, _ := res.LastInsertId()
	token := createToken(int(id), body.Username)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"token":"%s","user":{"id":%d,"username":"%s"}}`, token, id, body.Username)
}

func handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	ip := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ip = strings.Split(forwarded, ",")[0]
	}
	loginMu.Lock()
	entry, exists := loginAttempts[ip]
	if exists && time.Now().Before(entry.until) && entry.attempts >= 5 {
		loginMu.Unlock()
		w.Header().Set("Retry-After", "900")
		http.Error(w, `{"error":"too many attempts, try again in 15 minutes"}`, 429)
		return
	}
	loginMu.Unlock()

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid body"}`, 400)
		return
	}
	var id int
	var hash string
	err := db.QueryRow("SELECT id, password_hash FROM users WHERE username = ?", body.Username).Scan(&id, &hash)
	if err != nil || !checkPassword(body.Password, hash) {
		loginMu.Lock()
		if !exists {
			loginAttempts[ip] = &loginEntry{attempts: 1, until: time.Now().Add(15 * time.Minute)}
		} else {
			entry.attempts++
		}
		loginMu.Unlock()
		http.Error(w, `{"error":"invalid credentials"}`, 401)
		return
	}
	loginMu.Lock()
	delete(loginAttempts, ip)
	loginMu.Unlock()
	token := createToken(id, body.Username)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"token":"%s","user":{"id":%d,"username":"%s"}}`, token, id, body.Username)
}

func handleAuthMe(w http.ResponseWriter, r *http.Request) {
	uid, un, ok := authMiddleware(w, r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id":%d,"username":"%s"}`, uid, un)
}

func handleAuthDebug(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	log.Printf("DEBUG auth header: %q", auth)
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		uid, un, ok := parseToken(token)
		log.Printf("DEBUG parseToken: uid=%d un=%q ok=%v jwtSecret=%s", uid, un, ok, jwtSecret[:8])
		fmt.Fprintf(w, `{"uid":%d,"un":"%s","ok":%v,"secret_prefix":"%s"}`, uid, un, ok, jwtSecret[:8])
		return
	}
	fmt.Fprintf(w, `{"error":"no bearer token"}`)
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
	rows, err := db.Query("SELECT video_id FROM favorites WHERE user_id = ? ORDER BY created_at DESC", uid)
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

func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive, nosnippet, noimageindex")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; media-src *; img-src *; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
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
			case r.URL.Path == "/api/crawl":
				handleAPICrawl(w, r)
			case r.URL.Path == "/api/crawl-xh":
				handleAPICrawlXh(w, r)
			case r.URL.Path == "/api/crawl-ep":
				handleAPICrawlEp(w, r)
			case r.URL.Path == "/api/crawl-tf":
				handleAPICrawlTf(w, r)
			case r.URL.Path == "/api/crawl-dt":
				handleAPICrawlDt(w, r)
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
			case r.URL.Path == "/api/auth/register":
				handleAuthRegister(w, r)
			case r.URL.Path == "/api/auth/login":
				handleAuthLogin(w, r)
			case r.URL.Path == "/api/auth/me":
				handleAuthMe(w, r)
			case r.URL.Path == "/api/auth/debug":
				handleAuthDebug(w, r)
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
	http.HandleFunc("/api/crawl", handleAPICrawl)
	http.HandleFunc("/api/crawl-xh", handleAPICrawlXh)
	http.HandleFunc("/api/crawl-ep", handleAPICrawlEp)
	http.HandleFunc("/api/crawl-tf", handleAPICrawlTf)
	http.HandleFunc("/api/crawl-dt", handleAPICrawlDt)
	http.HandleFunc("/api/categories", handleAPICategories)
	http.HandleFunc("/api/browse", handleAPIBrowse)
	http.HandleFunc("/api/video/", handleAPIVideo)
	http.HandleFunc("/api/refresh", handleAPIRefresh)
	http.HandleFunc("/api/reclassify", handleAPIRclassify)
	http.HandleFunc("/api/status", handleStatusSSE)
	http.HandleFunc("/api/auth/register", handleAuthRegister)
	http.HandleFunc("/api/auth/login", handleAuthLogin)
	http.HandleFunc("/api/auth/me", handleAuthMe)
	http.HandleFunc("/api/auth/debug", handleAuthDebug)
	http.HandleFunc("/api/fav/video/", handleFavVideo)
	http.HandleFunc("/api/fav/videos", handleFavVideos)
	http.HandleFunc("/api/fav/category", handleFavCategory)
	http.HandleFunc("/api/fav/categories", handleFavCategories)
	http.HandleFunc("/api/health", handleHealth)
	http.HandleFunc("/api/random", handleAPIRandom)
	http.HandleFunc("/api/related/", handleAPIRelated)
	http.HandleFunc("/api/tags", handleAPITags)
	http.HandleFunc("/api/watch/", handleWatchRouter)
	http.HandleFunc("/api/watch-later", handleWatchLaterList)
	http.HandleFunc("/api/watch-later/", handleWatchLaterRouter)
	http.HandleFunc("/api/playlists", handlePlaylistListCreate)
	http.HandleFunc("/api/playlists/", handlePlaylistRouter)
	http.HandleFunc("/api/rate/", handleRateVideo)
	http.HandleFunc("/api/for-you", handleForYou)
	http.HandleFunc("/api/suggestions", handleSuggestions)
	http.HandleFunc("/api/profile", handleProfile)
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

func refreshExpiring() {
	cutoff := time.Now().Add(6 * time.Hour).Unix()
	rows, err := db.Query("SELECT id, slug FROM videos WHERE expires_at > 0 AND expires_at < ? ORDER BY expires_at ASC LIMIT 500", cutoff)
	if err != nil {
		log.Printf("Refresh query failed: %v", err)
		return
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id, slug string
		rows.Scan(&id, &slug)
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
			if v, err := scrapeVideoDetail(vid); err == nil {
				storeVideo(v)
				clearScrapeFailure(vid)
				log.Printf("Refreshed %s", vid)
			} else {
				recordScrapeFailure(vid, err)
			}
		}(id)
	}
	wg.Wait()
	log.Println("Refresh cycle complete")
}

// --- Progress / SSE ---

func setProgress(source, status string, scanned, newVideos, cached, detailDone, detailTotal, page int) {
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

func getProgressJSON() []byte {
	progress.mu.RLock()
	p := progress
	progress.mu.RUnlock()
	var total int
	db.QueryRow("SELECT COUNT(*) FROM videos").Scan(&total)
	p.TotalCount = total
	p.SourceCounts = map[string]int{}
	rows, err := db.Query("SELECT source, COUNT(*) FROM videos GROUP BY source")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var src string
			var count int
			rows.Scan(&src, &count)
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
			newCats := strings.Join(extractCategories(title, desc, tags), ",")
			if newCats != catsStr {
				db.Exec("UPDATE videos SET categories = ? WHERE id = ?", newCats, id)
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
	if cat != "" {
		rows, err = db.Query(
			"SELECT id, slug, title, description, categories, duration, views, thumb_uuid, preview_url, added_at, upload_date, source FROM videos WHERE categories LIKE ? ORDER BY "+order+" LIMIT ? OFFSET ?",
			"%"+cat+"%", perPage, (page-1)*perPage)
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
		rows.Scan(&v.ID, &v.Slug, &v.Title, &v.Description, &cats, &dur, &views, &v.ThumbUUID, &v.PreviewURL, &v.AddedAt, &uploadDate)
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

	count := 0
	db.QueryRow("SELECT COUNT(*) FROM videos").Scan(&count)

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
	rows, err := db.Query(`SELECT categories FROM videos WHERE categories != '' AND categories IS NOT NULL AND categories != 'uncategorized'`)
	if err != nil {
		return
	}
	defer rows.Close()
	freq := map[string]int{}
	for rows.Next() {
		var cats string
		rows.Scan(&cats)
		for _, c := range strings.Split(cats, ",") {
			c = strings.TrimSpace(c)
			if c != "" && c != "uncategorized" {
				freq[c]++
			}
		}
	}
	type catCount struct {
		Name  string
		Count int
	}
	var sorted []catCount
	for name, count := range freq {
		sorted = append(sorted, catCount{name, count})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Count > sorted[j].Count })
	catCache.cats = nil
	for i, cc := range sorted {
		if i >= 30 {
			break
		}
		catCache.cats = append(catCache.cats, cc.Name)
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
		"recent": "v.added_at DESC",
		"new":    "v.upload_date DESC",
		"views":  "v.views DESC",
		"duration": "v.duration DESC",
		"trending": "(CAST(v.views AS REAL) / MAX(1.0, julianday('now') - julianday(v.added_at))) DESC",
	}
	orderBy := "v.added_at DESC"
	if o, ok := validSorts[sort]; ok {
		orderBy = o
	}
	var whereClauses []string
	var args []interface{}
	if cat != "" {
		whereClauses = append(whereClauses, "v.categories LIKE ?")
		args = append(args, "%"+cat+"%")
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
				`SELECT v.id, COALESCE(v.slug,''), COALESCE(v.title,''), COALESCE(v.description,''), v.categories, v.duration, v.views, COALESCE(v.thumb_uuid,''), COALESCE(v.preview_url,''), COALESCE(v.added_at,''), v.upload_date, COALESCE(v.source,'xnxx')
				 FROM videos_fts f JOIN videos v ON v.rowid = f.rowid`+ftsWhere+` ORDER BY rank LIMIT ? OFFSET ?`,
				append(ftsArgs, perPage, (page-1)*perPage)...)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					vv := Video{}
					var dur, views sql.NullInt64
					var cats, uploadDate sql.NullString
					rows.Scan(&vv.ID, &vv.Slug, &vv.Title, &vv.Description, &cats, &dur, &views, &vv.ThumbUUID, &vv.PreviewURL, &vv.AddedAt, &uploadDate, &vv.Source)
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
		query := `SELECT v.id, COALESCE(v.slug,''), COALESCE(v.title,''), COALESCE(v.description,''), v.categories, v.duration, v.views, COALESCE(v.thumb_uuid,''), COALESCE(v.preview_url,''), COALESCE(v.added_at,''), v.upload_date, COALESCE(v.source,'xnxx') FROM videos v` + where + ` ORDER BY ` + orderBy + ` LIMIT ? OFFSET ?`
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
			rows.Scan(&vv.ID, &vv.Slug, &vv.Title, &vv.Description, &cats, &dur, &views, &vv.ThumbUUID, &vv.PreviewURL, &vv.AddedAt, &uploadDate, &vv.Source)
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
		db.QueryRow(countQuery+ftsWhere, append([]interface{}{sanitizeFTSQuery(q)}, args...)...).Scan(&cnt)
		count = cnt
		if count == 0 {
			count = len(videos)
		}
	} else {
		db.QueryRow("SELECT COUNT(*) FROM videos v"+where, args...).Scan(&count)
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
	w.Header().Set("Cache-Control", "public, max-age=60")

	id := strings.TrimPrefix(r.URL.Path, "/api/video/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		http.Error(w, "missing id", 400)
		return
	}

	v := Video{}
	var dur, views sql.NullInt64
	var cats, tags sql.NullString
	var uploader, uploadDate sql.NullString
	var expiresAt sql.NullInt64
	err := db.QueryRow(
		`SELECT id, slug, title, description, categories, tags, uploader, upload_date,
		        duration, views, url_360, url_720, url_1080, hls_url, thumb_uuid,
		        preview_url, expires_at, source
		 FROM videos WHERE id = ?`, id,
	).Scan(&v.ID, &v.Slug, &v.Title, &v.Description, &cats, &tags, &uploader, &uploadDate,
		&dur, &views, &v.URL360, &v.URL720, &v.URL1080, &v.HLSURL, &v.ThumbUUID,
		&v.PreviewURL, &expiresAt, &v.Source)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	v.Duration = int(dur.Int64)
	v.Views = int(views.Int64)
	if expiresAt.Valid {
		v.ExpiresAt = expiresAt.Int64
	}
	if cats.Valid && cats.String != "" {
		v.Categories = strings.Split(cats.String, ",")
	}
	if tags.Valid && tags.String != "" {
		v.Tags = strings.Split(tags.String, ",")
	}
	if uploader.Valid {
		v.Uploader = uploader.String
	}
	if uploadDate.Valid {
		v.UploadDate = uploadDate.String
	}

	// Include watched_position for authenticated users
	if uid, _, ok := authMiddleware(w, r); ok {
		var pos int
		db.QueryRow("SELECT COALESCE(position, 0) FROM watch_history WHERE user_id = ? AND video_id = ?", uid, id).Scan(&pos)
		// Append as extra field — wrap in map to include both Video fields and watched_position
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": v.ID, "slug": v.Slug, "title": v.Title, "description": v.Description,
			"categories": v.Categories, "tags": v.Tags, "uploader": v.Uploader,
			"upload_date": v.UploadDate, "duration": v.Duration, "views": v.Views,
			"added_at": v.AddedAt, "source": v.Source, "thumb_uuid": v.ThumbUUID,
			"url_360": v.URL360, "url_720": v.URL720, "url_1080": v.URL1080,
			"preview_url": v.PreviewURL, "hls_url": v.HLSURL,
			"secure_token": v.SecureToken, "expires_at": v.ExpiresAt,
			"watched_position": pos,
		})
		return
	}

	if v.ExpiresAt == 0 || (v.ExpiresAt > 0 && v.ExpiresAt < time.Now().Add(10*time.Minute).Unix()) {
		go func(vid string) {
			if refreshed, err := scrapeVideoDetail(vid); err == nil {
				storeVideo(refreshed)
				setCachedVideo(vid, refreshed)
			}
		}(id)
	}

	json.NewEncoder(w).Encode(v)
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
	w.Header().Set("Cache-Control", "public, max-age=300")

	id := strings.TrimPrefix(r.URL.Path, "/play/")
	id = strings.TrimSuffix(id, "/")

	// Check in-memory cache
	if v, ok := getCachedVideo(id); ok {
		renderPlayPage(w, r, v)
		return
	}

	v := Video{}
	var dur, views sql.NullInt64
	var cats, tags sql.NullString
	var uploader, uploadDate sql.NullString
	var expiresAt sql.NullInt64
	err := db.QueryRow(
		`SELECT id, slug, title, description, categories, tags, uploader, upload_date,
		        duration, views, url_360, url_720, url_1080, hls_url, thumb_uuid,
		        preview_url, expires_at, source
		 FROM videos WHERE id = ?`, id,
	).Scan(&v.ID, &v.Slug, &v.Title, &v.Description, &cats, &tags, &uploader, &uploadDate,
		&dur, &views, &v.URL360, &v.URL720, &v.URL1080, &v.HLSURL, &v.ThumbUUID,
		&v.PreviewURL, &expiresAt, &v.Source)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	v.Duration = int(dur.Int64)
	v.Views = int(views.Int64)
	if expiresAt.Valid {
		v.ExpiresAt = expiresAt.Int64
	}
	if cats.Valid && cats.String != "" {
		v.Categories = strings.Split(cats.String, ",")
	}
	if tags.Valid && tags.String != "" {
		v.Tags = strings.Split(tags.String, ",")
	}
	if uploader.Valid {
		v.Uploader = uploader.String
	}
	if uploadDate.Valid {
		v.UploadDate = uploadDate.String
	}

	// If token is expired or about to expire in <10 min, auto-refresh in background
	if v.ExpiresAt == 0 || (v.ExpiresAt > 0 && v.ExpiresAt < time.Now().Add(10*time.Minute).Unix()) {
		go func(vid string) {
			if refreshed, err := scrapeVideoDetail(vid); err == nil {
				storeVideo(refreshed)
				setCachedVideo(vid, refreshed)
				log.Printf("Auto-refreshed %s (token was expiring)", vid)
			}
		}(id)
	}

	setCachedVideo(id, v)
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
	quality := "360"
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
	default:
		targetURL = v.URL360
	}
	if targetURL == "" {
		for _, u := range []string{v.URL360, v.URL720, v.URL1080} {
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

func handleThumbProxy(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/thumb/")
	if path == "" {
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
	proxyCDN(w, r, targetURL)
}

func loadOrRefreshVideo(id string) (Video, bool) {
	if v, ok := getCachedVideo(id); ok {
		if v.ExpiresAt == 0 || v.ExpiresAt > time.Now().Unix() {
			return v, true
		}
	}
	v := Video{}
	var dur, views sql.NullInt64
	var cats, tags sql.NullString
	var uploader, uploadDate sql.NullString
	var expiresAt sql.NullInt64
	err := db.QueryRow(
		`SELECT id, slug, title, description, categories, tags, uploader, upload_date,
		        duration, views, url_360, url_720, url_1080, hls_url, thumb_uuid,
		        preview_url, expires_at, source
		 FROM videos WHERE id = ?`, id,
	).Scan(&v.ID, &v.Slug, &v.Title, &v.Description, &cats, &tags, &uploader, &uploadDate,
		&dur, &views, &v.URL360, &v.URL720, &v.URL1080, &v.HLSURL, &v.ThumbUUID,
		&v.PreviewURL, &expiresAt, &v.Source)
	if err != nil {
		return v, false
	}
	v.Duration = int(dur.Int64)
	v.Views = int(views.Int64)
	if expiresAt.Valid {
		v.ExpiresAt = expiresAt.Int64
	}
	if cats.Valid && cats.String != "" {
		v.Categories = strings.Split(cats.String, ",")
	}
	if tags.Valid && tags.String != "" {
		v.Tags = strings.Split(tags.String, ",")
	}
	if uploader.Valid {
		v.Uploader = uploader.String
	}
	if uploadDate.Valid {
		v.UploadDate = uploadDate.String
	}

	if v.ExpiresAt == 0 || (v.ExpiresAt > 0 && v.ExpiresAt < time.Now().Unix()) {
		if refreshed, err := scrapeVideoDetail(id); err == nil {
			storeVideo(refreshed)
			setCachedVideo(id, refreshed)
			return refreshed, true
		}
	}
	setCachedVideo(id, v)
	return v, true
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

	resp, err := mediaClient.Do(req)
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
	io.Copy(w, resp.Body)
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
	if len(v.Categories) > 0 && v.Categories[0] != "uncategorized" {
		catPatterns := []string{}
		catArgs := []interface{}{}
		for _, cat := range v.Categories {
			catPatterns = append(catPatterns, "categories LIKE ?")
			catArgs = append(catArgs, "%"+cat+"%")
		}
		catArgs = append(catArgs, v.ID)
		rrows, err := db.Query(
			"SELECT id, title, duration, views, thumb_uuid FROM videos WHERE ("+strings.Join(catPatterns, " OR ")+") AND id != ? ORDER BY views DESC LIMIT 12",
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
		rrows, err := db.Query("SELECT id, title, duration, views, thumb_uuid FROM videos WHERE id != ? ORDER BY views DESC LIMIT 12", v.ID)
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

func handleAPISearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")

	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, "missing q", 400)
		return
	}

	videos := scrapeXnxxSearch(q)

	cached, newCount := 0, 0
	for _, v := range videos {
		if !isValidXnxxID(v.ID) {
			continue
		}
		var exists string
		db.QueryRow("SELECT id FROM videos WHERE id = ?", v.ID).Scan(&exists)
		if exists != "" {
			cached++
			continue
		}
		cats := strings.Join(extractCategories(v.Title, "", nil), ",")
		db.Exec("INSERT OR IGNORE INTO videos (id, slug, title, categories, added_at) VALUES (?,?,?,?,?)",
			v.ID, v.Slug, v.Title, cats, time.Now().Format("2006-01-02"))
		detail, err := scrapeVideoDetail(v.ID)
		if err != nil {
			log.Printf("Detail scrape failed for %s: %v", v.ID, err)
			continue
		}
		storeVideo(detail)
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
	go runXhCrawl()
	go runEpCrawl()
	go runTfCrawl()
	go runDtCrawl()
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
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1 << 20))
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
		body2, _ := io.ReadAll(io.LimitReader(resp2.Body, 1 << 20))
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1 << 20))
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1 << 20))
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
	v, err := scrapeVideoDetail(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
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
	reVideoLink  = regexp.MustCompile(`<a[^>]*href="/video-([a-z0-9]+)/([^"]+)"`)
	reJSONLD     = regexp.MustCompile(`<script[^>]*type="application/ld\+json"[^>]*>\s*(\{[\s\S]*?\})\s*</script>`)
	reHLSSource  = regexp.MustCompile(`https://hls-cdn77\.xnxx-cdn\.com/([^"'\s]+,\d+)/([a-f0-9-]+)/\d+/hls\.m3u8`)
	reThumbUUID  = regexp.MustCompile(`/([a-f0-9-]+)/\d+/(?:xn_\d+_t|preview)`)
	reVidScript  = regexp.MustCompile(`video_url[^=]*=\s*'([^']+)'`) // JS variable fallback

	// html5player exposes the real per-quality URLs, EACH with its own secure
	// token. xnxx no longer lets one token serve every quality, so these must be
	// captured verbatim — never synthesized by swapping the filename.
	reSetUrlHigh = regexp.MustCompile(`setVideoUrlHigh\(\s*['"]([^'"]+)['"]`)
	reSetUrlLow  = regexp.MustCompile(`setVideoUrlLow\(\s*['"]([^'"]+)['"]`)
	reSetHLS     = regexp.MustCompile(`setVideoHLS\(\s*['"]([^'"]+)['"]`)
	// Two filename generations: legacy video_{res}p.mp4 and 2026+ mp4_{label}.mp4 (sd/hq/hd/fhd)
	reMP4Any     = regexp.MustCompile(`https://mp4-[^.]+\.xnxx-cdn\.com/([a-f0-9-]+)/\d+/(?:video_(\d+)p|mp4_([a-z0-9]+))\.mp4\?secure=([^"'\s\\]+)`)
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
			Name        string   `json:"name"`
			Description string   `json:"description"`
			ContentURL  string   `json:"contentUrl"`
			Duration    string   `json:"duration"`
			ThumbnailURL []string `json:"thumbnailUrl"`
			Interaction struct {
				Count int `json:"userInteractionCount"`
			} `json:"interactionStatistic"`
		}
		if err := json.Unmarshal([]byte(m[1]), &ld); err == nil {
			v.Title = ld.Name
			v.Description = ld.Description
			v.Views = ld.Interaction.Count
			v.Duration = parseDuration(ld.Duration)

			if len(ld.ThumbnailURL) > 0 {
				if m2 := reThumbUUID.FindStringSubmatch(ld.ThumbnailURL[0]); len(m2) > 1 {
					v.ThumbUUID = m2[1]
					v.PreviewURL = fmt.Sprintf("%s/%s/0/preview.mp4", thumbCDN, v.ThumbUUID)
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
	cats := strings.Join(extractCategories(v.Title, v.Description, v.Tags), ",")
	tagsStr := strings.Join(v.Tags, ",")
	// On re-scrape keep the original added_at — refreshes must not push old
	// videos back to the top of the Recent feed.
	_, err := db.Exec(`INSERT INTO videos (id, slug, title, description, categories, tags, uploader, upload_date, duration, views, added_at, source, thumb_uuid, url_360, url_720, url_1080, preview_url, hls_url, secure_token, expires_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			slug=excluded.slug, title=excluded.title, description=excluded.description,
			categories=excluded.categories, tags=excluded.tags, uploader=excluded.uploader,
			upload_date=excluded.upload_date, duration=excluded.duration, views=excluded.views,
			source=excluded.source, thumb_uuid=excluded.thumb_uuid,
			url_360=excluded.url_360, url_720=excluded.url_720, url_1080=excluded.url_1080,
			preview_url=excluded.preview_url, hls_url=excluded.hls_url,
			secure_token=excluded.secure_token, expires_at=excluded.expires_at`,
		v.ID, v.Slug, v.Title, v.Description, cats, tagsStr, v.Uploader, v.UploadDate,
		v.Duration, v.Views, v.AddedAt, v.Source,
		v.ThumbUUID, v.URL360, v.URL720, v.URL1080, v.PreviewURL, v.HLSURL,
		v.SecureToken, v.ExpiresAt)
	if err != nil {
		log.Printf("storeVideo(%s) failed: %v", v.ID, err)
	}
}

// --- xHamster Scraper ---

var (
	reXhInitials    = regexp.MustCompile(`window\.initials\s*=\s*(\{[\s\S]*?\});\s*</script>`)
	reXhVideoList   = regexp.MustCompile(`"pageURL"\s*:\s*"((?:[^"\\]|\\.)*)"[^}]*"id"\s*:\s*(\d+)`)
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
	ID    int    `json:"id"`
	Title string `json:"titleLocalized"`
	PageURL string `json:"pageURL"`
	ThumbURL string `json:"thumbURL"`
	TrailerURL string `json:"trailerURL"`
	Duration int `json:"duration"`
	Views int `json:"views"`
}

func httpGetXhWithRetry(urlStr string) (*http.Response, error) {
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
		if len(m) < 2 { continue }
		pageURL := strings.ReplaceAll(m[1], `\/`, `/`)

		slugMatch := reXhSlugID.FindStringSubmatch(pageURL)
		if slugMatch == nil { continue }
		shortID := slugMatch[2]

		if seen[shortID] { continue }
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
	// Extract all tag names
	tagMatches := regexp.MustCompile(`"name"\s*:\s*"([^"]+)"`).FindAllStringSubmatch(initJSON, -1)
	for _, tm := range tagMatches {
		if len(tm) > 1 {
			name := tm[1]
			if name != "" && name != v.Title {
				tagSet[name] = true
			}
		}
	}
	for name := range tagSet {
		v.Tags = append(v.Tags, name)
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
				if v.URL360 == "" { v.URL360 = mp4URL }
			case res <= 720:
				if v.URL720 == "" { v.URL720 = mp4URL }
			default:
				if v.URL1080 == "" { v.URL1080 = mp4URL }
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

	// Seed URLs to crawl - similar to xnxx seed strategy
	seeds := []string{
		xhBase + "/newest",
		xhBase + "/best/weekly",
		xhBase + "/best/monthly",
		xhBase + "/channels",
	}

	for _, seed := range seeds {
		for page := 0; page < 5; page++ {
			pageURL := seed
			if page > 0 {
				pageURL = fmt.Sprintf("%s?page=%d", seed, page)
			}
			log.Printf("xHamster: scanning %s", pageURL)

			videos := scrapeXhListing(pageURL)
			if len(videos) == 0 {
				if page > 0 { break }
				continue
			}

			for _, v := range videos {
				if v.ID == "" || v.Slug == "" { continue }

				var exists string
				db.QueryRow("SELECT id FROM videos WHERE id = ?", v.ID).Scan(&exists)
				if exists != "" { continue }

				// Insert stub
				db.Exec(`INSERT OR IGNORE INTO videos (id, slug, title, source, added_at) VALUES (?,?,?,?,?)`,
					v.ID, v.Slug, v.Title, "xhamster", v.AddedAt)

				// Scrape details
				detail, err := scrapeXhVideoDetail(v.ID)
				if err != nil {
					log.Printf("xHamster detail scrape %s failed: %v", v.ID, err)
					recordScrapeFailure(v.ID, err)
					continue
				}
				storeVideo(detail)
				totalNew++
				log.Printf("xHamster: new video %s: %s", v.ID, detail.Title)
			}
		}
	}

	log.Printf("xHamster crawl complete: %d new videos scraped", totalNew)
}

// --- EPorner Scraper ---

var (
	reEpVideoBlock  = regexp.MustCompile(`data-id="(\d+)"[^>]*>\s*<div class="mbimg">.*?<a href="([^"]*)"[^>]*>\s*<img src="([^"]*)"[^>]*>\s*</a>\s*(?:<div class="mvhdico"[^>]*><span>([^<]*)</span></div>)?.*?<p class="mbtit"><a[^>]*>([^<]*)</a></p>\s*<p class="mbstats">\s*<span class="mbtim"[^>]*>([^<]*)</span>\s*(?:<span class="mbrate"[^>]*>([^<]*)</span>)?\s*<span class="mbvie"[^>]*>([^<]*)</span>(?:\s*<span class="mb-uploader"><a[^>]*>([^<]*)</a></span>)?`)
	reEpHashSlug   = regexp.MustCompile(`/video-([^/]+)/([^/]*)/?$`)
	reEpMetaDesc   = regexp.MustCompile(`<meta name="description" content="([^"]*)"`)
	reEpCatLinks   = regexp.MustCompile(`<a[^>]*href="/category/([^"]*)/"[^>]*title="([^"]*)"`)
	reEpStarLinks  = regexp.MustCompile(`<a[^>]*href="/pornstar/([^"]*)/"[^>]*>([^<]*)</a>`)
	reEpVideoURLs  = regexp.MustCompile(`https?://[^"'\s<>]*?xvideos\.com/video[^"'\s<>]*|/dload/[^"'\s<>]*|"embedUrl"\s*:\s*"([^"]*)"`)
	reEpDurationSec = regexp.MustCompile(`(\d+)\s*min`)
	reEpRdate      = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`)
)

func httpGetEpWithRetry(urlStr string) (*http.Response, error) {
	<-rateLimitEp
	var lastErr error
	for attempt := 0; attempt < maxHTTPRetries; attempt++ {
		if attempt > 0 {
			delay := retryBaseDelay * time.Duration(1<<(attempt-1))
			if delay > retryMaxDelay { delay = retryMaxDelay }
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
			if attempt == maxHTTPRetries-1 { return nil, fmt.Errorf("HTTP %d", resp.StatusCode) }
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
		if len(m) < 8 { continue }
		id := m[1]
		href := m[2]
		thumbURL := m[3]
		quality := m[4]
		title := strings.TrimSpace(m[5])
		durationStr := m[6]
		rating := m[7]
		viewsStr := m[8]
		uploader := ""
		if len(m) > 9 { uploader = m[9] }

		hm := reEpHashSlug.FindStringSubmatch(href)
		if hm == nil || seen[id] { continue }
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
	if err != nil { return v, err }
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
		if len(cm) > 2 { v.Categories = append(v.Categories, cm[2]) }
	}
	// Pornstars as tags
	starMatches := reEpStarLinks.FindAllStringSubmatch(bodyStr, -1)
	for _, sm := range starMatches {
		if len(sm) > 2 { v.Tags = append(v.Tags, strings.TrimSpace(sm[2])) }
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
			if cat != "" { pageURL = epBase + "/" + cat + "/" }
			if page > 0 {
				if cat != "" {
					pageURL = fmt.Sprintf("%s/%d/", epBase+"/"+cat, page)
				} else {
					pageURL = fmt.Sprintf("%s/%d/", epBase, page)
				}
			}

			videos := scrapeEpListing(pageURL)
			if len(videos) == 0 {
				if page > 0 { break }
				continue
			}

			for _, v := range videos {
				if v.ID == "" || seen[v.ID] { continue }
				seen[v.ID] = true

				var exists string
				db.QueryRow("SELECT id FROM videos WHERE id = ?", v.ID).Scan(&exists)
				if exists != "" { continue }

				cats := strings.Join(extractCategories(v.Title, v.Description, v.Tags), ",")
				db.Exec(`INSERT OR IGNORE INTO videos (id, slug, title, description, categories, tags, uploader, duration, views, source, thumb_uuid, added_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
					v.ID, v.Slug, v.Title, v.Description, cats, strings.Join(v.Tags, ","), v.Uploader, v.Duration, v.Views, "eporner", v.ThumbUUID, v.AddedAt)

				detail, err := scrapeEpVideoDetail(v.ID)
				if err != nil {
					log.Printf("EPorner detail scrape %s failed: %v", v.ID, err)
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
	rows, err := db.Query("SELECT id, slug FROM videos WHERE url_360 = '' OR url_360 IS NULL OR thumb_uuid = '' OR thumb_uuid IS NULL OR (url_360 != '' AND expires_at = 0)")
	if err != nil {
		log.Printf("Query for unscraped videos failed: %v", err)
		return
	}
	defer rows.Close()

	type pending struct{ id, slug string }
	var pendingList []pending
	for rows.Next() {
		var id, slug string
		rows.Scan(&id, &slug)
		pendingList = append(pendingList, pending{id, slug})
	}
	rows.Close()

	if len(pendingList) == 0 {
		return
	}

	bgWg.Add(1)
	progress.mu.Lock()
	progress.Status = "scraping"
	progress.DetailTotal += len(pendingList)
	progress.mu.Unlock()

	log.Printf("Scraping details for %d videos (max %d concurrent)...", len(pendingList), scrapeWorkers)
	var wg sync.WaitGroup
	for i, p := range pendingList {
		if i > 0 {
			time.Sleep(300 * time.Millisecond)
		}
		wg.Add(1)
		scrapeSem <- struct{}{}
		go func(id, slug string) {
			defer wg.Done()
			defer func() { <-scrapeSem }()
			detail, err := scrapeVideoDetail(id)
			if err != nil {
				log.Printf("Detail scrape failed for %s: %v", id, err)
				recordScrapeFailure(id, err)
				time.Sleep(2 * time.Second)
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
		}(p.id, p.slug)
	}
	wg.Wait()
	bgWg.Done()
	log.Printf("Background detail scraping complete (%d videos)", len(pendingList))

	progress.mu.Lock()
	if progress.Scanned == 0 {
		progress.Status = "idle"
	}
	progress.mu.Unlock()
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
	var retryCount int
	db.QueryRow("SELECT retry_count FROM scrape_failures WHERE video_id = ?", videoID).Scan(&retryCount)
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
		videoID, retryCount+1, scrapeErr.Error(), nextRetry)
}

func clearScrapeFailure(videoID string) {
	db.Exec("DELETE FROM scrape_failures WHERE video_id = ?", videoID)
}

func retryFailedScrapes() {
	now := time.Now().Unix()
	rows, err := db.Query("SELECT video_id, retry_count FROM scrape_failures WHERE next_retry_at <= ? ORDER BY retry_count ASC LIMIT ?", now, maxFailuresPerBatch)
	if err != nil {
		return
	}
	defer rows.Close()

	type retryEntry struct{ id string; count int }
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
	<-rateLimitXh
		v, err := scrapeVideoDetail(e.id)
		if err != nil {
			delay := int64(failureBaseDelay.Seconds())
			for i := 0; i < e.count+1; i++ {
				delay *= 2
				if delay > int64(failureMaxDelay.Seconds()) {
					delay = int64(failureMaxDelay.Seconds())
					break
				}
			}
			nextRetry := time.Now().Unix() + delay
			db.Exec("UPDATE scrape_failures SET retry_count = retry_count + 1, last_error = ?, next_retry_at = ? WHERE video_id = ?",
				err.Error(), nextRetry, e.id)
			log.Printf("Retry failed for %s (attempt %d): %v", e.id, e.count+1, err)
			continue
		}
		storeVideo(v)
		clearScrapeFailure(e.id)
		setCachedVideo(e.id, v)
		log.Printf("Retry succeeded for %s", e.id)
	}
}

func retryFailedLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
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
