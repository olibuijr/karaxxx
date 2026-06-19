package main

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const dtBase = "https://www.drtuber.com"

var (
	reDtVideoLink  = regexp.MustCompile(`<a[^>]*href="/video/(\d+)/([^"]+)"[^>]*class="th ch-video[^"]*"[^>]*>\s*<img[^>]*src="([^"]+)"[^>]*alt="([^"]*)"[^>]*>`)
	reDtMetaDesc   = regexp.MustCompile(`<meta name="description" content="([^"]*)"`)
	reDtTags       = regexp.MustCompile(`<a[^>]*href="/tags/[^"]*"[^>]*>([^<]+)</a>`)
	reDtCatLinks   = regexp.MustCompile(`<a[^>]*href="/categories/[^"]*"[^>]*>([^<]+)</a>`)
	reDtViews      = regexp.MustCompile(`<span[^>]*class="[^"]*views[^"]*"[^>]*>\s*([\d,.]+)\s*</span>`)
	reDtOgTitle    = regexp.MustCompile(`<meta property="og:title" content="([^"]*)"`)
	reDtOgImage    = regexp.MustCompile(`<meta property="og:image" content="([^"]*)"`)
	reDtUploadDate = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`)
	reDtStarLinks  = regexp.MustCompile(`<a[^>]*href="/pornstars/[^"]*"[^>]*>([^<]+)</a>`)
)

func httpGetDtWithRetry(urlStr string) (*http.Response, error) {
	<-rateLimitDt
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
	return nil, fmt.Errorf("DrTuber request failed: %w", lastErr)
}

func scrapeDtListing(pageURL string) []Video {
	resp, err := httpGetDtWithRetry(pageURL)
	if err != nil {
		log.Printf("DrTuber listing %s failed: %v", pageURL, err)
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	resp.Body.Close()
	bodyStr := string(body)

	videos := []Video{}
	seen := map[string]bool{}

	blocks := reDtVideoLink.FindAllStringSubmatch(bodyStr, -1)
	for _, m := range blocks {
		if len(m) < 5 {
			continue
		}
		id := m[1]
		slug := m[2]

		if seen[id] {
			continue
		}
		seen[id] = true

		v := Video{
			ID:        id,
			Slug:      slug,
			Title:     strings.TrimSpace(m[4]),
			ThumbUUID: m[3],
			Source:    "drtuber",
			AddedAt:   time.Now().Format("2006-01-02"),
		}

		videos = append(videos, v)
	}
	return videos
}

func scrapeDtVideoDetail(videoID string) (Video, error) {
	v := Video{ID: videoID, Source: "drtuber"}

	url := dtBase + "/video/" + videoID
	resp, err := httpGetDtWithRetry(url)
	if err != nil {
		return v, err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	resp.Body.Close()
	bodyStr := string(body)

	if m := reDtOgTitle.FindStringSubmatch(bodyStr); len(m) > 1 {
		v.Title = strings.TrimSpace(strings.ReplaceAll(m[1], " - DrTuber", ""))
	}
	if m := reDtOgImage.FindStringSubmatch(bodyStr); len(m) > 1 {
		v.ThumbUUID = m[1]
	}
	if m := reDtMetaDesc.FindStringSubmatch(bodyStr); len(m) > 1 {
		v.Description = strings.TrimSpace(m[1])
	}
	if m := reDtViews.FindStringSubmatch(bodyStr); len(m) > 1 {
		cleaned := strings.ReplaceAll(strings.ReplaceAll(m[1], ",", ""), ".", "")
		v.Views, _ = strconv.Atoi(cleaned)
	}

	if durMatch := regexp.MustCompile(`(\d+):(\d+)`).FindStringSubmatch(bodyStr); len(durMatch) > 2 && v.Duration == 0 {
		m, _ := strconv.Atoi(durMatch[1])
		s, _ := strconv.Atoi(durMatch[2])
		v.Duration = m*60 + s
	}

	tagMatches := reDtTags.FindAllStringSubmatch(bodyStr, -1)
	for _, tm := range tagMatches {
		if len(tm) > 1 {
			v.Tags = append(v.Tags, strings.TrimSpace(tm[1]))
		}
	}

	catMatches := reDtCatLinks.FindAllStringSubmatch(bodyStr, -1)
	for _, cm := range catMatches {
		if len(cm) > 1 {
			v.Categories = append(v.Categories, strings.TrimSpace(cm[1]))
		}
	}

	starMatches := reDtStarLinks.FindAllStringSubmatch(bodyStr, -1)
	for _, sm := range starMatches {
		if len(sm) > 1 {
			v.Tags = append(v.Tags, strings.TrimSpace(sm[1]))
		}
	}

	if m := reDtUploadDate.FindStringSubmatch(bodyStr); len(m) > 1 {
		v.UploadDate = m[1]
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
	// For DrTuber the video player is JS-heavy so URL extraction from raw HTML
	// is not reliable. Fallback is silent — video stays as metadata-only.
	return v, nil
}

func runDtCrawl() {
	if !crawlMuDt.TryLock() {
		log.Println("DrTuber crawl already running")
		return
	}
	defer crawlMuDt.Unlock()

	log.Println("Starting DrTuber crawl...")
	totalNew := 0
	seen := map[string]bool{}

	seeds := []string{
		dtBase + "/",
		dtBase + "/most-popular",
		dtBase + "/top-rated",
		dtBase + "/most-commented",
		dtBase + "/new",
		dtBase + "/longest",
		dtBase + "/categories/teen",
		dtBase + "/categories/milf",
		dtBase + "/categories/anal",
		dtBase + "/categories/lesbian",
		dtBase + "/categories/blowjob",
		dtBase + "/categories/big-cock",
		dtBase + "/categories/amateur",
		dtBase + "/categories/hardcore",
		dtBase + "/categories/group",
		dtBase + "/categories/compilation",
	}

	for _, seed := range seeds {
		for page := 1; page <= 20; page++ {
			pageURL := seed
			if page > 1 {
				if strings.HasSuffix(seed, "/") {
					pageURL = fmt.Sprintf("%s%d", seed, page)
				} else {
					pageURL = fmt.Sprintf("%s/%d", seed, page)
				}
			}
			log.Printf("DrTuber: scanning %s", pageURL)

			videos := scrapeDtListing(pageURL)
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

				db.Exec(`INSERT OR IGNORE INTO videos (id, slug, title, duration, source, thumb_uuid, added_at) VALUES (?,?,?,?,?,?,?)`,
					v.ID, v.Slug, v.Title, v.Duration, "drtuber", v.ThumbUUID, v.AddedAt)

				detail, err := scrapeDtVideoDetail(v.ID)
				if err != nil {
					log.Printf("DrTuber detail scrape %s failed: %v", v.ID, err)
					storeVideo(v)
					continue
				}
				storeVideo(detail)
				totalNew++
			}
		}
	}

	log.Printf("DrTuber crawl complete: %d new videos scraped", totalNew)
}

func handleAPICrawlDt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	go runDtCrawl()
	http.Redirect(w, r, "/", 302)
}
