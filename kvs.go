package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type kvsSourceConfig struct {
	Key         string
	Label       string
	Base        string
	Seeds       []string
	Pages       int
	PathPattern string
}

var kvsSources = []kvsSourceConfig{
	{
		Key:   "heavyfetish",
		Label: "HeavyFetish",
		Base:  "https://heavyfetish.com",
		Seeds: []string{
			"https://heavyfetish.com/tags/bdsm/",
			"https://heavyfetish.com/tags/bondage/",
			"https://heavyfetish.com/tags/femdom/",
			"https://heavyfetish.com/fetish-videos/",
		},
		Pages:       5,
		PathPattern: "/videos/%s/",
	},
	{
		Key:   "punishbang",
		Label: "PunishBang",
		Base:  "https://www.punishbang.com",
		Seeds: []string{
			"https://www.punishbang.com/videos/",
			"https://www.punishbang.com/categories/bondage/",
		},
		Pages:       5,
		PathPattern: "/video/%s/%s/",
	},
	{
		Key:   "sunporno",
		Label: "SunPorno BDSM",
		Base:  "https://www.sunporno.com",
		Seeds: []string{
			"https://www.sunporno.com/tags/bdsm/",
			"https://www.sunporno.com/tags/bondage/",
			"https://www.sunporno.com/tags/femdom/",
		},
		Pages:       5,
		PathPattern: "/v/%s/%s/",
	},
}

var (
	reKVSVideoID       = regexp.MustCompile(`video_id:\s*['"]?([A-Za-z0-9_-]+)['"]?`)
	reKVSMP4           = regexp.MustCompile(`https?://[^"'<>\s]+\.mp4[^"'<>\s]*`)
	reKVSFlashURL      = regexp.MustCompile(`video(?:_alt_url\d*|_url)\s*:\s*['"]([^'"]+\.mp4[^'"]*)['"]`)
	reKVSPreview       = regexp.MustCompile(`preview_url\d*\s*:\s*['"]([^'"]+)['"]`)
	reKVSViews         = regexp.MustCompile(`(?i)([\d,.]+)\s*(K|M)?\s*views?`)
	reKVSQuality       = regexp.MustCompile(`(?i)(1080|720|480|360|240)p`)
	reKVSJSONLDScripts = regexp.MustCompile(`(?is)<script[^>]+type=["']application/ld\+json["'][^>]*>\s*(.*?)\s*</script>`)
)

func kvsConfig(source string) (kvsSourceConfig, bool) {
	for _, cfg := range kvsSources {
		if cfg.Key == source {
			return cfg, true
		}
	}
	return kvsSourceConfig{}, false
}

func kvsSourceKeys() []string {
	keys := make([]string, 0, len(kvsSources))
	for _, cfg := range kvsSources {
		keys = append(keys, cfg.Key)
	}
	return keys
}

func isKVSSource(source string) bool {
	_, ok := kvsConfig(source)
	return ok
}

func httpGetKVSWithRetry(cfg kvsSourceConfig, urlStr string) (*http.Response, error) {
	<-rateLimitKVS
	var lastErr error
	for attempt := 0; attempt < maxHTTPRetries; attempt++ {
		if attempt > 0 {
			delay := retryBaseDelay * time.Duration(1<<(attempt-1))
			if delay > retryMaxDelay {
				delay = retryMaxDelay
			}
			time.Sleep(delay + time.Duration(rand.Intn(1000))*time.Millisecond)
		}
		req, errReq := http.NewRequest("GET", urlStr, nil)
		if errReq != nil {
			return nil, errReq
		}
		req.Header.Set("User-Agent", userAgents[rand.Intn(len(userAgents))])
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")
		req.Header.Set("Referer", cfg.Base+"/")
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == 403 || resp.StatusCode == 404 || resp.StatusCode == 429 || resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			if resp.StatusCode == 403 || resp.StatusCode == 404 || attempt == maxHTTPRetries-1 {
				return nil, lastErr
			}
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("%s request failed: %w", cfg.Key, lastErr)
}

func kvsPageURL(seed string, page int) string {
	if page <= 1 {
		return seed
	}
	trimmed := strings.TrimRight(seed, "/")
	return fmt.Sprintf("%s/%d/", trimmed, page)
}

func kvsVideoID(source, rawID string) string {
	rawID = strings.Trim(rawID, "/")
	return source + "_" + rawID
}

func kvsRawID(source, id string) string {
	prefix := source + "_"
	return strings.TrimPrefix(id, prefix)
}

func scrapeKVSListing(cfg kvsSourceConfig, pageURL string) []Video {
	resp, err := httpGetKVSWithRetry(cfg, pageURL)
	if err != nil {
		log.Printf("%s listing %s failed: %v", cfg.Label, pageURL, err)
		return nil
	}
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil
	}

	seen := map[string]bool{}
	videos := []Video{}
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		detailURL, rawID, slug := parseKVSLink(cfg, href)
		if detailURL == "" || slug == "" || rawID == "" {
			return
		}
		id := kvsVideoID(cfg.Key, rawID)
		if seen[id] {
			return
		}
		seen[id] = true

		title, _ := s.Attr("title")
		title = strings.TrimSpace(strings.TrimPrefix(title, "Porn Videos"))
		if title == "" {
			title = strings.TrimSpace(s.Find(".video-title, .title, strong").First().Text())
		}
		if title == "" {
			title = slugToTitle(slug)
		}
		thumb, _ := s.Find("img").First().Attr("data-webp")
		if thumb == "" {
			thumb, _ = s.Find("img").First().Attr("data-original")
		}
		if thumb == "" {
			thumb, _ = s.Find("img").First().Attr("src")
		}
		preview, _ := s.Find("img").First().Attr("data-preview")
		duration := parseDurationText(strings.TrimSpace(s.Find(".duration, .time").First().Text()))
		views := parseKVSViews(strings.TrimSpace(s.Find(".video-views, .views").First().Text()))

		videos = append(videos, Video{
			ID:         id,
			Slug:       slug,
			Title:      html.UnescapeString(title),
			Duration:   duration,
			Views:      views,
			Source:     cfg.Key,
			ThumbUUID:  absKVSURL(cfg, thumb),
			PreviewURL: absKVSURL(cfg, preview),
			AddedAt:    time.Now().Format("2006-01-02"),
		})
		_ = detailURL
	})
	return videos
}

func parseKVSLink(cfg kvsSourceConfig, href string) (detailURL, rawID, slug string) {
	if href == "" || strings.HasPrefix(href, "#") {
		return "", "", ""
	}
	abs := absKVSURL(cfg, href)
	u, err := url.Parse(abs)
	if err != nil {
		return "", "", ""
	}
	baseURL, _ := url.Parse(cfg.Base)
	if u.Host != baseURL.Host {
		return "", "", ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch cfg.Key {
	case "heavyfetish":
		if len(parts) == 2 && parts[0] == "videos" && !strings.Contains(parts[1], "?") {
			return abs, parts[1], parts[1]
		}
	case "punishbang":
		if len(parts) >= 3 && parts[0] == "video" {
			return abs, parts[1], parts[2]
		}
	case "sunporno":
		if len(parts) >= 3 && parts[0] == "v" {
			return abs, parts[1], parts[2]
		}
	}
	return "", "", ""
}

func kvsDetailURL(cfg kvsSourceConfig, id, slug string) string {
	raw := kvsRawID(cfg.Key, id)
	if cfg.Key == "heavyfetish" {
		if slug == "" {
			slug = raw
		}
		return cfg.Base + fmt.Sprintf(cfg.PathPattern, slug)
	}
	return cfg.Base + fmt.Sprintf(cfg.PathPattern, raw, slug)
}

func scrapeKVSVideoDetail(id, source string) (Video, error) {
	cfg, ok := kvsConfig(source)
	if !ok {
		return Video{ID: id, Source: source}, fmt.Errorf("unknown KVS source %q", source)
	}
	v := Video{ID: id, Source: cfg.Key, AddedAt: time.Now().Format("2006-01-02")}
	db.QueryRow("SELECT COALESCE(slug,''), COALESCE(title,''), COALESCE(thumb_uuid,''), COALESCE(preview_url,'') FROM videos WHERE id = ?", id).Scan(&v.Slug, &v.Title, &v.ThumbUUID, &v.PreviewURL)
	detailURL := kvsDetailURL(cfg, id, v.Slug)
	resp, err := httpGetKVSWithRetry(cfg, detailURL)
	if err != nil {
		return v, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 3<<20))
	htmlStr := string(body)

	if m := reKVSVideoID.FindStringSubmatch(htmlStr); len(m) > 1 && cfg.Key == "heavyfetish" && strings.HasPrefix(v.ID, cfg.Key+"_") {
		// Keep the slug-based primary key for stability, but use player metadata for media extraction.
		_ = m[1]
	}
	parseKVSJSONLD(&v, htmlStr)
	parseKVSFlashvars(&v, cfg, htmlStr)
	parseKVSMetaFallbacks(&v, htmlStr)
	if v.Slug == "" {
		v.Slug = kvsRawID(cfg.Key, id)
	}
	if v.Title == "" {
		v.Title = slugToTitle(v.Slug)
	}
	if !hasPlayableMedia(v) {
		return v, fmt.Errorf("%s detail page returned no playable MP4", cfg.Key)
	}
	return v, nil
}

func parseKVSJSONLD(v *Video, htmlStr string) {
	for _, m := range reKVSJSONLDScripts.FindAllStringSubmatch(htmlStr, -1) {
		if len(m) < 2 || !strings.Contains(m[1], "VideoObject") {
			continue
		}
		var ld struct {
			Type         string      `json:"@type"`
			Name         string      `json:"name"`
			Description  string      `json:"description"`
			Keywords     interface{} `json:"keywords"`
			ThumbnailURL interface{} `json:"thumbnailUrl"`
			UploadDate   string      `json:"uploadDate"`
			Duration     string      `json:"duration"`
			Actor        interface{} `json:"actor"`
		}
		cleaned := strings.TrimSpace(strings.ReplaceAll(m[1], "\\/", "/"))
		if err := json.Unmarshal([]byte(cleaned), &ld); err != nil {
			continue
		}
		if ld.Name != "" {
			v.Title = html.UnescapeString(ld.Name)
		}
		if ld.Description != "" {
			v.Description = html.UnescapeString(ld.Description)
		}
		if ld.UploadDate != "" {
			v.UploadDate = strings.Split(ld.UploadDate, "T")[0]
		}
		if ld.Duration != "" {
			v.Duration = parseDuration(ld.Duration)
		}
		if thumb := firstJSONLDString(ld.ThumbnailURL); thumb != "" && v.ThumbUUID == "" {
			v.ThumbUUID = thumb
		}
		for _, tag := range splitJSONLDStrings(ld.Keywords) {
			v.Tags = appendUniqueString(v.Tags, tag)
		}
		for _, actor := range splitJSONLDStrings(ld.Actor) {
			if v.Uploader == "" {
				v.Uploader = actor
			}
			v.Tags = appendUniqueString(v.Tags, actor)
		}
	}
}

func parseKVSFlashvars(v *Video, cfg kvsSourceConfig, htmlStr string) {
	for _, m := range reKVSFlashURL.FindAllStringSubmatch(htmlStr, -1) {
		if len(m) < 2 {
			continue
		}
		assignKVSMediaURL(v, cfg, m[1])
	}
	for _, raw := range reKVSMP4.FindAllString(htmlStr, -1) {
		if strings.Contains(raw, "_preview.mp4") || strings.Contains(raw, "preview_") || strings.Contains(raw, ".mp4.jpg") {
			continue
		}
		assignKVSMediaURL(v, cfg, raw)
	}
	if v.PreviewURL == "" {
		if m := reKVSPreview.FindStringSubmatch(htmlStr); len(m) > 1 {
			v.PreviewURL = absKVSURL(cfg, m[1])
		}
	}
}

func parseKVSMetaFallbacks(v *Video, htmlStr string) {
	if v.Title == "" {
		if m := regexp.MustCompile(`(?i)<meta\s+property=["']og:title["']\s+content=["']([^"']*)`).FindStringSubmatch(htmlStr); len(m) > 1 {
			v.Title = html.UnescapeString(strings.TrimSpace(m[1]))
		}
	}
	if v.Description == "" {
		if m := regexp.MustCompile(`(?i)<meta\s+name=["']description["']\s+content=["']([^"']*)`).FindStringSubmatch(htmlStr); len(m) > 1 {
			v.Description = html.UnescapeString(strings.TrimSpace(m[1]))
		}
	}
	if v.ThumbUUID == "" {
		if m := regexp.MustCompile(`(?i)<meta\s+property=["']og:image["']\s+content=["']([^"']*)`).FindStringSubmatch(htmlStr); len(m) > 1 {
			v.ThumbUUID = strings.TrimSpace(m[1])
		}
	}
}

func assignKVSMediaURL(v *Video, cfg kvsSourceConfig, rawURL string) bool {
	rawURL = strings.TrimSpace(strings.ReplaceAll(rawURL, "\\/", "/"))
	rawURL = strings.TrimPrefix(rawURL, "function/0/")
	rawURL = strings.ReplaceAll(rawURL, "&amp;", "&")
	rawURL = absKVSURL(cfg, rawURL)
	if rawURL == "" || !strings.Contains(rawURL, ".mp4") {
		return false
	}
	quality := 360
	if m := reKVSQuality.FindStringSubmatch(rawURL); len(m) > 1 {
		quality, _ = strconv.Atoi(m[1])
	}
	switch {
	case quality >= 1080:
		if v.URL1080 == "" {
			v.URL1080 = rawURL
		}
	case quality >= 720:
		if v.URL720 == "" {
			v.URL720 = rawURL
		}
	default:
		if v.URL360 == "" {
			v.URL360 = rawURL
		}
	}
	return true
}

func absKVSURL(cfg kvsSourceConfig, raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\/", "/"))
	if raw == "" || strings.HasPrefix(raw, "data:") {
		return ""
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	if strings.HasPrefix(raw, "/") {
		return cfg.Base + raw
	}
	return cfg.Base + "/" + raw
}

func parseDurationText(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	parts := strings.Split(s, ":")
	if len(parts) == 2 {
		m, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
		sec, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
		return m*60 + sec
	}
	if len(parts) == 3 {
		h, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
		m, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
		sec, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
		return h*3600 + m*60 + sec
	}
	return 0
}

func parseKVSViews(s string) int {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return 0
	}
	if m := reKVSViews.FindStringSubmatch(s); len(m) > 1 {
		val, _ := strconv.ParseFloat(m[1], 64)
		switch strings.ToUpper(m[2]) {
		case "M":
			val *= 1000000
		case "K":
			val *= 1000
		}
		return int(val)
	}
	return 0
}

func firstJSONLDString(v interface{}) string {
	s := splitJSONLDStrings(v)
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

func splitJSONLDStrings(v interface{}) []string {
	out := []string{}
	switch x := v.(type) {
	case string:
		for _, part := range strings.Split(x, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, html.UnescapeString(part))
			}
		}
	case []interface{}:
		for _, item := range x {
			out = append(out, splitJSONLDStrings(item)...)
		}
	case map[string]interface{}:
		if name, ok := x["name"].(string); ok && name != "" {
			out = append(out, html.UnescapeString(strings.TrimSpace(name)))
		}
	}
	return out
}

func appendUniqueString(items []string, item string) []string {
	item = strings.TrimSpace(item)
	if item == "" {
		return items
	}
	for _, existing := range items {
		if strings.EqualFold(existing, item) {
			return items
		}
	}
	return append(items, item)
}

func runKVSCrawl() {
	if !crawlMuKVS.TryLock() {
		log.Println("KVS BDSM crawl already running")
		return
	}
	defer crawlMuKVS.Unlock()
	lockPath := "/tmp/karaxxx-kvs-crawl.lock"
	if _, err := os.Stat(lockPath); err == nil {
		log.Println("KVS BDSM crawl already running (lock file exists)")
		return
	}
	os.WriteFile(lockPath, []byte{}, 0644)
	defer os.Remove(lockPath)

	totalNew := 0
	for _, cfg := range kvsSources {
		newForSource := 0
		seen := map[string]bool{}
		log.Printf("Starting %s crawl...", cfg.Label)
		setProgress(cfg.Key, "searching", 0, 0, 0, 0, 0, 0)
		for _, seed := range cfg.Seeds {
			for page := 1; page <= cfg.Pages; page++ {
				pageURL := kvsPageURL(seed, page)
				log.Printf("%s: scanning %s", cfg.Label, pageURL)
				videos := scrapeKVSListing(cfg, pageURL)
				if len(videos) == 0 {
					if page > 1 {
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
					storeVideo(v)
					detail, err := scrapeKVSVideoDetail(v.ID, cfg.Key)
					if err != nil {
						log.Printf("%s detail scrape %s failed: %v", cfg.Label, v.ID, err)
						recordScrapeFailure(v.ID, err)
						continue
					}
					storeVideo(detail)
					clearScrapeFailure(v.ID)
					newForSource++
					totalNew++
				}
				setProgress(cfg.Key, "searching", len(seen), newForSource, len(seen)-newForSource, 0, 0, page)
			}
		}
		log.Printf("%s crawl complete: %d new videos scraped", cfg.Label, newForSource)
		setProgress(cfg.Key, "idle", len(seen), newForSource, len(seen)-newForSource, 0, 0, 0)
	}
	log.Printf("KVS BDSM crawl complete: %d new videos scraped", totalNew)
}

func handleAPICrawlKVS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	go runKVSCrawl()
	http.Redirect(w, r, "/", http.StatusFound)
}
