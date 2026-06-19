package main

import (
	"fmt"
	"os"
	"testing"
)

func withTempDB(t *testing.T) {
	t.Helper()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if db != nil {
			db.Close()
			db = nil
		}
		os.Chdir(oldwd)
	})
	initDB()
}

func TestRecordScrapeFailurePrunesPermanentGoldRedirectImmediately(t *testing.T) {
	withTempDB(t)
	storeVideo(Video{ID: "gold_teaser", Title: "Premium teaser", Source: "xnxx", AddedAt: "2026-06-19"})

	recordScrapeFailure("gold_teaser", fmt.Errorf("redirected off-site to www.xnxx.gold"))

	if _, ok := loadVideoFromDB("gold_teaser"); ok {
		t.Fatal("expected permanent xnxx.gold redirect to delete the unplayable teaser immediately")
	}
}

func TestRetryFailedScrapesClearsFailureWhenNonPlayableSourceIsSkipped(t *testing.T) {
	withTempDB(t)
	storeVideo(Video{ID: "ep_meta", Title: "Metadata only", Source: "eporner", AddedAt: "2026-06-19"})
	_, err := db.Exec("INSERT INTO scrape_failures (video_id, retry_count, last_error, next_retry_at) VALUES (?,?,?,0)", "ep_meta", 2, "old metadata-only failure")
	if err != nil {
		t.Fatal(err)
	}

	retryFailedScrapes()

	var count int
	db.QueryRow("SELECT COUNT(*) FROM scrape_failures WHERE video_id = ?", "ep_meta").Scan(&count)
	if count != 0 {
		t.Fatalf("expected metadata-only retry success to clear stale failure, got %d rows", count)
	}
}
