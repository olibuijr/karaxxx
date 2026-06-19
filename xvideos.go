package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const xvBase = "https://www.xvideos.com"

var (
	reXvVideoLink  = regexp.MustCompile(`href="/video[\w.]+/([^"]+)"[^>]*title="([^"]*)"`)
	reXvVideoHref  = regexp.MustCompile(`href="/video([\w.]+)/([^"]+)"`)
	reXvNumericID  = regexp.MustCompile(`href="/video(\d+)/([^"]+)"`)
	reXvOgTitle    = regexp.MustCompile(`<meta\s+property="og:title"\s+content="([^"]*)"`)
	reXvOgImage    = regexp.MustCompile(`<meta\s+property="og:image"\s+content="([^"]*)"`)
	reXvOgDesc     = regexp.MustCompile(`<meta\s+property="og:description"\s+content="([^"]*)"`)
	reXvDuration   = regexp.MustCompile(`<span[^>]+class=["']duration["'][^>]*>(?:[\s\n]*)<?(\d[^<]*)`)
	reXvViews      = regexp.MustCompile(`(?i)([\d,.]+)\s*views?`)
	reXvTags       = regexp.MustCompile(`<a[^>]*href="/tags/[^"]*"[^>]*>([^<]+)</a>`)
	reXvCats       = regexp.MustCompile(`<a[^>]*href="/cats/[^"]*"[^>]*>([^<]+)</a>`)
	reXvUploadDate = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`)
	reXvSetUrlHigh = regexp.MustCompile(`setVideoUrlHigh\s*\(\s*['"]([^'"]+)['"]`)
	reXvSetUrlLow  = regexp.MustCompile(`setVideoUrlLow\s*\(\s*['"]([^'"]+)['"]`)
	reXvSetHLS     = regexp.MustCompile(`setVideoHLS\s*\(\s*['"]([^'"]+)['"]`)
	reXvFlvUrl     = regexp.MustCompile(`flv_url=(.+?)&`)
	reXvJSONLD     = regexp.MustCompile(`<script[^>]*type="application/ld\+json"[^>]*>\s*(\{[\s\S]*?\})\s*</script>`)
	reXvTitle      = regexp.MustCompile(`setVideoTitle\s*\(\s*['"]([^'"]+)['"]`)
	reXvUploader   = regexp.MustCompile(`<a[^>]*href="/profiles/[^"]*"[^>]*>([^<]+)</a>`)
)

func httpGetXvWithRetry(urlStr string) (*http.Response, error) {
	<-rateLimitXv
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
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")
		req.Header.Set("Referer", xvBase+"/")
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
			delay := retryBaseDelay * time.Duration(1<<attempt)
			time.Sleep(delay + time.Duration(rand.Intn(2000))*time.Millisecond)
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("xVideos request failed: %w", lastErr)
}

// scrapeXvListing extracts video metadata from a listing page.
func scrapeXvListing(pageURL string) []Video {
	resp, err := httpGetXvWithRetry(pageURL)
	if err != nil {
		log.Printf("xVideos listing %s failed: %v", pageURL, err)
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	resp.Body.Close()
	bodyStr := string(body)

	videos := []Video{}
	seen := map[string]bool{}

	// Match href="/video.{id}/{slug}" with title attribute
	hrefMatches := reXvVideoHref.FindAllStringSubmatch(bodyStr, -1)
	titleMatches := reXvVideoLink.FindAllStringSubmatch(bodyStr, -1)

	// Build title map: slug -> title
	titleMap := make(map[string]string)
	for _, tm := range titleMatches {
		if len(tm) > 2 {
			titleMap[tm[1]] = tm[2]
		}
	}

	for _, m := range hrefMatches {
		if len(m) < 3 {
			continue
		}
		rawID := strings.TrimSpace(m[1])
		slug := strings.TrimSpace(m[2])

		// Strip leading dot if present (e.g. ".iopkmuaedc3" -> "iopkmuaedc3")
		id := strings.TrimPrefix(rawID, ".")
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true

		title := titleMap[slug]
		if title == "" {
			title = slugToTitleXv(slug)
		}
		// Unescape HTML entities
		title = strings.ReplaceAll(title, "&#039;", "'")
		title = strings.ReplaceAll(title, "&amp;", "&")
		title = strings.ReplaceAll(title, "&quot;", "\"")
		title = strings.TrimSpace(title)

		v := Video{
			ID:      id,
			Slug:    slug,
			Title:   title,
			Source:  "xvideos",
			AddedAt: time.Now().Format("2006-01-02"),
		}
		videos = append(videos, v)
	}

	// Also catch pure numeric IDs (older format)
	for _, m := range reXvNumericID.FindAllStringSubmatch(bodyStr, -1) {
		if len(m) < 3 {
			continue
		}
		id := strings.TrimSpace(m[1])
		slug := strings.TrimSpace(m[2])
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		title := titleMap[slug]
		if title == "" {
			title = slugToTitleXv(slug)
		}
		v := Video{
			ID:      id,
			Slug:    slug,
			Title:   title,
			Source:  "xvideos",
			AddedAt: time.Now().Format("2006-01-02"),
		}
		videos = append(videos, v)
	}

	return videos
}

// slugToTitleXv converts a URL slug to a readable title.
func slugToTitleXv(slug string) string {
	// xVideos slugs use underscores for spaces
	t := strings.ReplaceAll(slug, "_", " ")
	t = strings.ReplaceAll(t, "-", " ")
	// Capitalize words
	words := strings.Fields(t)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// scrapeXvVideoDetail fetches a single xVideos video page and extracts all metadata + URLs.
// NOTE: xVideos 404s without a slug — we MUST have it.
func scrapeXvVideoDetail(videoID string) (Video, error) {
	v := Video{ID: videoID, Source: "xvideos"}

	// Get slug from DB — must exist or we can't fetch details
	var slug string
	db.QueryRow("SELECT slug FROM videos WHERE id = ?", videoID).Scan(&slug)
	if slug == "" {
		return v, fmt.Errorf("xVideos detail %s: no slug available, skipping detail scrape", videoID)
	}

	// xVideos uses /video.{id}/slug for alphanumeric IDs and /video{id}/slug for numeric IDs
	dot := "."
	if _, err := strconv.Atoi(videoID); err == nil {
		dot = "" // numeric IDs don't use a dot
	}
	url := xvBase + "/video" + dot + videoID + "/" + slug
	resp, err := httpGetXvWithRetry(url)
	if err != nil {
		return v, fmt.Errorf("xVideos detail %s: %w", videoID, err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1 << 20))
	resp.Body.Close()
	html := string(body)

	// --- Video URL Extraction ---
	// xVideos uses html5player.setVideoUrl* inside #html5video_base
	if m := reXvSetUrlHigh.FindStringSubmatch(html); len(m) > 1 {
		assignXvMediaURL(&v, m[1])
	}
	if m := reXvSetUrlLow.FindStringSubmatch(html); len(m) > 1 {
		assignXvMediaURL(&v, m[1])
	}
	if m := reXvSetHLS.FindStringSubmatch(html); len(m) > 1 {
		v.HLSURL = m[1]
		// Extract secure token and UUID from HLS URL
		// Pattern: https://hls-cdn77.xvideos-cdn.com/{TOKEN},{EXPIRY}/{UUID}/0/hls.m3u8
		if hm := regexp.MustCompile(`/([a-zA-Z0-9+/=]+,\d+)/([a-f0-9-]+)/`).FindStringSubmatch(m[1]); len(hm) > 2 {
			if v.SecureToken == "" {
				v.SecureToken = hm[1]
			}
			if v.ThumbUUID == "" {
				v.ThumbUUID = hm[2]
			}
		}
	}

	// ThumbUUID from high-quality MP4 URL
	if v.ThumbUUID == "" && v.URL360 != "" {
		if hm := regexp.MustCompile(`mp4-cdn77\.xvideos-cdn\.com/([a-f0-9-]+)/`).FindStringSubmatch(v.URL360); len(hm) > 1 {
			v.ThumbUUID = hm[1]
		}
	}

	// --- Metadata from JSON-LD ---
	if m := reXvJSONLD.FindStringSubmatch(html); len(m) > 1 {
		var ld struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			ContentURL  string   `json:"contentUrl"`
			Duration    string   `json:"duration"`
			Thumbnail   []string `json:"thumbnailUrl"`
			Interaction struct {
				Count int `json:"userInteractionCount"`
			} `json:"interactionStatistic"`
		}
		if err := json.Unmarshal([]byte(m[1]), &ld); err == nil {
			if ld.Name != "" {
				v.Title = ld.Name
			}
			v.Description = ld.Description
			v.Views = ld.Interaction.Count
			if dur := parseDuration(ld.Duration); dur > 0 {
				v.Duration = dur
			}
			if len(ld.Thumbnail) > 0 {
				// Keep the real thumbnail URL. Earlier extraction may have filled
				// ThumbUUID with a media UUID from MP4/HLS URLs, which is not a
				// renderable image for xVideos cards. The JSON-LD URL is the highest
				// fidelity poster xVideos exposes server-side.
				v.ThumbUUID = ld.Thumbnail[0]
			}
		}
	}

	// --- Title ---
	if v.Title == "" {
		if m := reXvTitle.FindStringSubmatch(html); len(m) > 1 {
			v.Title = strings.TrimSpace(m[1])
		} else if m := reXvOgTitle.FindStringSubmatch(html); len(m) > 1 {
			v.Title = strings.TrimSpace(strings.ReplaceAll(m[1], " - XVIDEOS", ""))
		}
	}

	// --- Thumbnail ---
	if v.ThumbUUID == "" || !strings.HasPrefix(v.ThumbUUID, "http") {
		if m := reXvOgImage.FindStringSubmatch(html); len(m) > 1 {
			v.ThumbUUID = m[1]
		}
	}

	// --- Duration ---
	if v.Duration == 0 {
		if m := reXvDuration.FindStringSubmatch(html); len(m) > 1 {
			durHTML := strings.TrimSpace(m[1])
			if dm := regexp.MustCompile(`(?:(\d+):)?(\d+):(\d+)`).FindStringSubmatch(durHTML); len(dm) > 3 {
				h, _ := strconv.Atoi(dm[1])
				mn, _ := strconv.Atoi(dm[2])
				s, _ := strconv.Atoi(dm[3])
				v.Duration = h*3600 + mn*60 + s
			} else if dm := regexp.MustCompile(`(\d+):(\d+)`).FindStringSubmatch(durHTML); len(dm) > 2 {
				mn, _ := strconv.Atoi(dm[1])
				s, _ := strconv.Atoi(dm[2])
				v.Duration = mn*60 + s
			}
		}
	}

	// --- Views ---
	if v.Views == 0 {
		if m := reXvViews.FindStringSubmatch(html); len(m) > 1 {
			cleaned := strings.ReplaceAll(strings.ReplaceAll(m[1], ",", ""), ".", "")
			v.Views, _ = strconv.Atoi(cleaned)
		}
	}

	// --- Tags ---
	tagMatches := reXvTags.FindAllStringSubmatch(html, -1)
	for _, tm := range tagMatches {
		if len(tm) > 1 {
			v.Tags = append(v.Tags, strings.TrimSpace(tm[1]))
		}
	}

	// --- Categories ---
	catMatches := reXvCats.FindAllStringSubmatch(html, -1)
	for _, cm := range catMatches {
		if len(cm) > 1 {
			v.Categories = append(v.Categories, strings.TrimSpace(cm[1]))
		}
	}

	// --- Uploader ---
	if m := reXvUploader.FindStringSubmatch(html); len(m) > 1 {
		v.Uploader = strings.TrimSpace(m[1])
	}

	// --- Upload date ---
	if m := reXvUploadDate.FindStringSubmatch(html); len(m) > 1 {
		v.UploadDate = m[1]
	}

	// --- Description ---
	if v.Description == "" {
		if m := reXvOgDesc.FindStringSubmatch(html); len(m) > 1 {
			v.Description = strings.TrimSpace(m[1])
		}
	}

	// Extract secure token from URL for token refresh
	if m := regexp.MustCompile(`secure=([^"'\s&]+)`).FindStringSubmatch(html); len(m) > 1 && v.SecureToken == "" {
		v.SecureToken = m[1]
	}

	v.AddedAt = time.Now().Format("2006-01-02")
	return v, nil
}

// assignXvMediaURL parses an xVideos CDN MP4 URL and stores it in the correct quality bucket.
// Pattern: https://mp4-cdn77.xvideos-cdn.com/{uuid}/0/video_{res}p.mp4?secure={token},{expiry}
func assignXvMediaURL(v *Video, rawURL string) bool {
	if rawURL == "" {
		return false
	}
	rawURL = strings.ReplaceAll(rawURL, "&amp;", "&")

	m := regexp.MustCompile(`video_(\d+)p\.mp4`).FindStringSubmatch(rawURL)
	var quality int
	if len(m) > 1 {
		quality, _ = strconv.Atoi(m[1])
	}
	if quality == 0 {
		quality = 360 // default fallback
	}

	// Extract UUID for thumb if not set
	if v.ThumbUUID == "" && strings.Contains(rawURL, "xvideos-cdn.com") {
		if um := regexp.MustCompile(`mp4-cdn77\.xvideos-cdn\.com/([a-f0-9-]+)/`).FindStringSubmatch(rawURL); len(um) > 1 {
			v.ThumbUUID = um[1]
		}
	}

	// Extract secure token
	if v.SecureToken == "" {
		if sm := regexp.MustCompile(`secure=([^"'\s&]+)`).FindStringSubmatch(rawURL); len(sm) > 1 {
			v.SecureToken = sm[1]
		}
	}

	switch {
	case quality <= 360:
		if v.URL360 == "" {
			v.URL360 = rawURL
		}
	case quality <= 720:
		if v.URL720 == "" {
			v.URL720 = rawURL
		} else if v.URL720 == rawURL && quality > 360 {
			v.URL720 = rawURL
		}
	case quality >= 1080:
		if v.URL1080 == "" {
			v.URL1080 = rawURL
		}
	}
	return true
}

// runXvCrawl performs a full xVideos crawl across multiple seed URLs.
func runXvCrawl() {
	if !crawlMuXv.TryLock() {
		log.Println("xVideos crawl already running")
		return
	}
	defer crawlMuXv.Unlock()
	xvLockPath := "/tmp/karaxxx-xv-crawl.lock"
	if _, err := os.Stat(xvLockPath); err == nil {
		log.Println("xVideos crawl already running (lock file exists)")
		return
	}
	os.WriteFile(xvLockPath, []byte{}, 0644)
	defer os.Remove(xvLockPath)

	log.Println("Starting xVideos crawl...")
	totalNew := 0
	seen := map[string]bool{}

	// Seed URLs — xVideos uses flat pagination: /new/1, /new/2, etc.
	seeds := []string{
		xvBase + "/new/1",
		xvBase + "/best",
		xvBase + "/popular/1",
		xvBase + "/rated/1",
		xvBase + "/longest/1",
	}

	for _, seed := range seeds {
		cfg := parseXvSeedConfig(seed)
		for page := 0; page < cfg.pages; page++ {
			pageURL := cfg.makeURL(page)
			log.Printf("xVideos: scanning %s", pageURL)

			videos := scrapeXvListing(pageURL)
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

				// Insert stub
				db.Exec(`INSERT OR IGNORE INTO videos (id, slug, title, source, added_at) VALUES (?,?,?,?,?)`,
					v.ID, v.Slug, v.Title, "xvideos", v.AddedAt)

				// Scrape detailed info including video URLs
				detail, err := scrapeXvVideoDetail(v.ID)
				if err != nil {
					log.Printf("xVideos detail scrape %s failed: %v", v.ID, err)
					storeVideo(v)
					continue
				}
				storeVideo(detail)
				totalNew++
			}
		}
	}

	log.Printf("xVideos crawl complete: %d new videos scraped", totalNew)
}

// xvSeedConfig holds the pagination configuration for a seed URL.
type xvSeedConfig struct {
	prefix string
	base   string
	pages  int
}

func parseXvSeedConfig(seed string) xvSeedConfig {
	// xVideos pagination:
	// /new/N -> /new/<N+1>, /best -> /best/YYYY-MM -> /best/YYYY-MM?page=N
	// /popular/N -> /popular/<N+1>
	if seed == xvBase+"/best" {
		return xvSeedConfig{
			prefix: "monthly",
			base:   seed,
			pages:  1,
		}
	}
	return xvSeedConfig{
		prefix: "numeric",
		base:   seed,
		pages:  10,
	}
}

func (c xvSeedConfig) makeURL(page int) string {
	if page == 0 {
		return c.base
	}
	switch c.prefix {
	case "numeric":
		// /new/1 -> /new/2 -> /new/3
		parts := strings.Split(c.base, "/")
		basePath := strings.Join(parts[:len(parts)-1], "/")
		return fmt.Sprintf("%s/%d", basePath, page+1)
	case "monthly":
		return fmt.Sprintf("%s?page=%d", c.base, page)
	default:
		return fmt.Sprintf("%s/%d", strings.TrimRight(c.base, "/"), page)
	}
}

// handleAPICrawlXv triggers an xVideos crawl via HTTP.
func handleAPICrawlXv(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	go runXvCrawl()
	http.Redirect(w, r, "/", 302)
}
