package main

import (
	"encoding/json"
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

const tfBase = "https://www.tnaflix.com"

var (
	reTfVideoThumb = regexp.MustCompile(`(?s)<a[^>]*class="[^"]*video-thumb[^"]*"[^>]*href="([^"]+)"[^>]*data-trailer="([^"]*)"[^>]*>(.*?)</a>`)
	reTfImg        = regexp.MustCompile(`<img[^>]*data-src="([^"]+)"[^>]*alt="([^"]+)"`)
	reTfDuration   = regexp.MustCompile(`<div[^>]*video-duration[^>]*>([^<]+)</div>`)
	reTfVideoID    = regexp.MustCompile(`/video(\d+)$`)
	reTfCatURL     = regexp.MustCompile(`https?://[^/]+/([^/]+)/`)
	reTfJSONLD     = regexp.MustCompile(`<script type="application/ld\+json">(.*?)</script>`)
	reTfViews      = regexp.MustCompile(`<span[^>]*class="[^"]*views[^"]*"[^>]*>([\d,]+)[^<]*</span>`)
	reTfCategory   = regexp.MustCompile(`<a[^>]*data-category="([^"]*)"[^>]*>([^<]*)</a>`)
	reTfStar       = regexp.MustCompile(`<a[^>]*href="/pornstar/([^"]*)/"[^>]*>([^<]*)</a>`)
	reTfSites      = regexp.MustCompile(`<a[^>]*href="https?://[^"]*"[^>]*data-site-name="([^"]*)"`)
)

type tfJSONLD struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	UploadDate   string `json:"uploadDate"`
	Duration     string `json:"duration"`
	EmbedURL     string `json:"embedUrl"`
	ThumbnailURL string `json:"thumbnailUrl"`
}

func httpGetTfWithRetry(urlStr string) (*http.Response, error) {
	<-rateLimitTf
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
	return nil, fmt.Errorf("TNAFlix request failed: %w", lastErr)
}

func scrapeTfListing(pageURL string) []Video {
	resp, err := httpGetTfWithRetry(pageURL)
	if err != nil {
		log.Printf("TNAFlix listing %s failed: %v", pageURL, err)
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	resp.Body.Close()
	bodyStr := string(body)

	videos := []Video{}
	seen := map[string]bool{}

	blocks := reTfVideoThumb.FindAllStringSubmatch(bodyStr, -1)
	for _, m := range blocks {
		if len(m) < 3 {
			continue
		}
		pageURL := m[1]
		inner := m[3]

		idMatch := reTfVideoID.FindStringSubmatch(pageURL)
		if idMatch == nil {
			continue
		}
		id := idMatch[1]
		if seen[id] {
			continue
		}
		seen[id] = true

		v := Video{ID: id, Source: "tnaflix", AddedAt: time.Now().Format("2006-01-02")}

		if catMatch := reTfCatURL.FindStringSubmatch(pageURL); len(catMatch) > 1 {
			v.Categories = []string{strings.ReplaceAll(catMatch[1], "-", " ")}
		}

		if imgMatch := reTfImg.FindStringSubmatch(inner); len(imgMatch) > 2 {
			v.ThumbUUID = imgMatch[1]
			v.Title = strings.TrimSpace(imgMatch[2])
		}

		if durMatch := reTfDuration.FindStringSubmatch(inner); len(durMatch) > 1 {
			v.Duration = parseEpDuration(strings.TrimSpace(durMatch[1]))
		}

		videos = append(videos, v)
	}
	return videos
}

func scrapeTfVideoDetail(videoID string) (Video, error) {
	v := Video{ID: videoID, Source: "tnaflix"}

	url := tfBase + "/video" + videoID
	resp, err := httpGetTfWithRetry(url)
	if err != nil {
		return v, err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	resp.Body.Close()
	bodyStr := string(body)

	if m := reTfJSONLD.FindStringSubmatch(bodyStr); len(m) > 1 {
		var ld tfJSONLD
		if err := json.Unmarshal([]byte(m[1]), &ld); err == nil {
			v.Title = ld.Name
			v.Description = ld.Description
			v.UploadDate = strings.Split(ld.UploadDate, "T")[0]
			if ld.ThumbnailURL != "" {
				v.ThumbUUID = ld.ThumbnailURL
			}
			if ld.Duration != "" {
				durStr := strings.TrimPrefix(ld.Duration, "PT")
				if idx := strings.Index(durStr, "H"); idx >= 0 {
					h, _ := strconv.Atoi(durStr[:idx])
					durStr = durStr[idx+1:]
					v.Duration += h * 3600
				}
				if idx := strings.Index(durStr, "M"); idx >= 0 {
					m, _ := strconv.Atoi(durStr[:idx])
					durStr = durStr[idx+1:]
					v.Duration += m * 60
				}
				if idx := strings.Index(durStr, "S"); idx >= 0 {
					s, _ := strconv.Atoi(durStr[:idx])
					v.Duration += s
				}
			}
		}
	}

	if v.Title == "" {
		if m := regexp.MustCompile(`<meta property="og:title" content="([^"]*)"`).FindStringSubmatch(bodyStr); len(m) > 1 {
			v.Title = strings.TrimSpace(strings.ReplaceAll(m[1], " - TNAFlix", ""))
		}
	}

	if viewsMatch := reTfViews.FindStringSubmatch(bodyStr); len(viewsMatch) > 1 {
		v.Views, _ = strconv.Atoi(strings.ReplaceAll(viewsMatch[1], ",", ""))
	}

	catMatches := reTfCategory.FindAllStringSubmatch(bodyStr, -1)
	for _, cm := range catMatches {
		if len(cm) > 2 {
			v.Categories = append(v.Categories, strings.TrimSpace(cm[2]))
		}
	}

	starMatches := reTfStar.FindAllStringSubmatch(bodyStr, -1)
	for _, sm := range starMatches {
		if len(sm) > 2 {
			v.Tags = append(v.Tags, strings.TrimSpace(sm[2]))
		}
	}

	siteMatches := reTfSites.FindAllStringSubmatch(bodyStr, -1)
	for _, sm := range siteMatches {
		if len(sm) > 1 && sm[1] != "" {
			v.Uploader = sm[1]
		}
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

	return v, nil
}

func runTfCrawl() {
	if !crawlMuTf.TryLock() {
		log.Println("TNAFlix crawl already running")
		return
	}
	defer crawlMuTf.Unlock()

	log.Println("Starting TNAFlix crawl...")
	totalNew := 0
	seen := map[string]bool{}

	seeds := []string{
		tfBase + "/",
		tfBase + "/popular",
		tfBase + "/hd-videos",
		tfBase + "/top-rated",
		tfBase + "/longest",
		tfBase + "/newest",
		tfBase + "/most-viewed",
		tfBase + "/amateur",
		tfBase + "/teen",
		tfBase + "/milf",
		tfBase + "/anal",
		tfBase + "/blowjob",
		tfBase + "/lesbian",
		tfBase + "/big-cock",
		tfBase + "/ebony",
		tfBase + "/cartoon",
	}

	for _, seed := range seeds {
		for page := 1; page <= 20; page++ {
			pageURL := seed
			if page > 1 {
				pageURL = fmt.Sprintf("%s/%d", strings.TrimRight(seed, "/"), page)
			}
			log.Printf("TNAFlix: scanning %s", pageURL)

			videos := scrapeTfListing(pageURL)
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

				cats := strings.Join(v.Categories, ",")
				db.Exec(`INSERT OR IGNORE INTO videos (id, slug, title, categories, duration, source, thumb_uuid, added_at) VALUES (?,?,?,?,?,?,?,?)`,
					v.ID, "", v.Title, cats, v.Duration, "tnaflix", v.ThumbUUID, v.AddedAt)

				detail, err := scrapeTfVideoDetail(v.ID)
				if err != nil {
					log.Printf("TNAFlix detail scrape %s failed: %v", v.ID, err)
					continue
				}
				storeVideo(detail)
				totalNew++
			}
		}
	}

	log.Printf("TNAFlix crawl complete: %d new videos scraped", totalNew)
}

func handleAPICrawlTf(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	go runTfCrawl()
	http.Redirect(w, r, "/", 302)
}
