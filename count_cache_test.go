package main

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"
)

func setupCountCacheTestDB(t *testing.T) *sql.DB {
	t.Helper()

	originalDB := db
	originalCache := countCache

	dsn := fmt.Sprintf("file:count-cache-%d?mode=memory&cache=shared", time.Now().UnixNano())
	testDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("sql.Open() failed: %v", err)
	}
	testDB.SetMaxOpenConns(1)

	db = testDB
	countCacheMu.Lock()
	countCache = map[string]struct {
		n   int
		exp time.Time
	}{}
	countCacheMu.Unlock()

	t.Cleanup(func() {
		_ = testDB.Close()
		db = originalDB
		countCacheMu.Lock()
		countCache = originalCache
		countCacheMu.Unlock()
	})

	return testDB
}

func TestCachedCountCachesAndSeparatesArgs(t *testing.T) {
	testDB := setupCountCacheTestDB(t)

	if _, err := testDB.Exec(`CREATE TABLE videos (id TEXT PRIMARY KEY, source TEXT)`); err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}
	if _, err := testDB.Exec(`INSERT INTO videos (id, source) VALUES ('1', 'xhamster'), ('2', 'xhamster'), ('3', 'xnxx')`); err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	query := `SELECT COUNT(*) FROM videos WHERE source = ?`

	first := cachedCount("browse_count:source", query, "xhamster")
	if first != 2 {
		t.Fatalf("first cachedCount() = %d, want 2", first)
	}

	if _, err := testDB.Exec(`INSERT INTO videos (id, source) VALUES ('4', 'xhamster')`); err != nil {
		t.Fatalf("second INSERT failed: %v", err)
	}

	second := cachedCount("browse_count:source", query, "xhamster")
	if second != 2 {
		t.Fatalf("second cachedCount() = %d, want cached 2", second)
	}

	other := cachedCount("browse_count:source", query, "xnxx")
	if other != 1 {
		t.Fatalf("cachedCount() with different args = %d, want 1", other)
	}
}

func TestCachedCountDoesNotPoisonCacheOnError(t *testing.T) {
	testDB := setupCountCacheTestDB(t)

	query := `SELECT COUNT(*) FROM videos`

	if got := cachedCount("videos_total", query); got != 0 {
		t.Fatalf("cachedCount() on missing table = %d, want 0", got)
	}

	if _, err := testDB.Exec(`CREATE TABLE videos (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}
	if _, err := testDB.Exec(`INSERT INTO videos (id) VALUES ('1')`); err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	if got := cachedCount("videos_total", query); got != 1 {
		t.Fatalf("cachedCount() after query recovery = %d, want 1", got)
	}
}

func TestCachedCountSupportsConcurrentReaders(t *testing.T) {
	testDB := setupCountCacheTestDB(t)

	if _, err := testDB.Exec(`CREATE TABLE videos (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}
	if _, err := testDB.Exec(`INSERT INTO videos (id) VALUES ('1'), ('2'), ('3')`); err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	const goroutines = 24
	const iterations = 25

	results := make(chan int, goroutines*iterations)
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				results <- cachedCount("videos_total", `SELECT COUNT(*) FROM videos`)
			}
		}()
	}

	wg.Wait()
	close(results)

	for result := range results {
		if result != 3 {
			t.Fatalf("cachedCount() concurrent result = %d, want 3", result)
		}
	}
}
