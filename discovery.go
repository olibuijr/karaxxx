package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

func handleAPIRandom(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	cat := r.URL.Query().Get("cat")
	query := "SELECT id FROM videos"
	clauses := []string{playableMediaSQL}
	var args []interface{}
	if source != "" {
		clauses = append(clauses, "source = ?")
		args = append(args, source)
	}
	if normalizedCat := normalizeCategoryTerm(cat); normalizedCat != "" {
		clauses = append(clauses, "id IN (SELECT video_id FROM video_categories WHERE category = ?)")
		args = append(args, normalizedCat)
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY RANDOM() LIMIT 1"
	var id string
	err := db.QueryRow(query, args...).Scan(&id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func handleAPIRelated(w http.ResponseWriter, r *http.Request) {
	videoID := strings.TrimPrefix(r.URL.Path, "/api/related/")
	videoID = strings.TrimSuffix(videoID, "/")
	if videoID == "" {
		http.Error(w, "missing video id", 400)
		return
	}
	limit := 12
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	var vSource string
	db.QueryRow("SELECT source FROM videos WHERE id = ?", videoID).Scan(&vSource)

	related := []Video{}
	categories := loadVideoCategories(videoID)
	if len(categories) > 0 {
		placeholders := make([]string, 0, len(categories))
		catArgs := make([]interface{}, 0, len(categories)+2)
		for _, category := range categories {
			placeholders = append(placeholders, "?")
			catArgs = append(catArgs, category)
		}
		catArgs = append(catArgs, videoID, limit)
		rows, err := db.Query(
			"SELECT DISTINCT v.id, v.slug, v.title, v.description, v.categories, v.duration, v.views, v.thumb_uuid, v.preview_url, v.added_at, v.upload_date, v.source FROM videos v JOIN video_categories vc ON vc.video_id = v.id WHERE vc.category IN ("+strings.Join(placeholders, ",")+") AND v.id != ? AND "+playableMediaSQLV+" ORDER BY v.views DESC LIMIT ?",
			catArgs...)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				vv := Video{}
				var dur, views sql.NullInt64
				var rc, rDate sql.NullString
				rows.Scan(&vv.ID, &vv.Slug, &vv.Title, &vv.Description, &rc, &dur, &views, &vv.ThumbUUID, &vv.PreviewURL, &vv.AddedAt, &rDate, &vv.Source)
				vv.Duration = int(dur.Int64)
				vv.Views = int(views.Int64)
				if rc.Valid && rc.String != "" {
					vv.Categories = strings.Split(rc.String, ",")
				}
				if rDate.Valid {
					vv.UploadDate = rDate.String
				}
				related = append(related, vv)
			}
		}
	}
	if len(related) == 0 {
		rows, err := db.Query(
			"SELECT id, slug, title, description, categories, duration, views, thumb_uuid, preview_url, added_at, upload_date, source FROM videos WHERE id != ? AND source = ? AND "+playableMediaSQL+" ORDER BY views DESC LIMIT ?",
			videoID, vSource, limit)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				vv := Video{}
				var dur, views sql.NullInt64
				var rc, rDate sql.NullString
				rows.Scan(&vv.ID, &vv.Slug, &vv.Title, &vv.Description, &rc, &dur, &views, &vv.ThumbUUID, &vv.PreviewURL, &vv.AddedAt, &rDate, &vv.Source)
				vv.Duration = int(dur.Int64)
				vv.Views = int(views.Int64)
				if rc.Valid && rc.String != "" {
					vv.Categories = strings.Split(rc.String, ",")
				}
				if rDate.Valid {
					vv.UploadDate = rDate.String
				}
				related = append(related, vv)
			}
		}
	}
	json.NewEncoder(w).Encode(related)
}

func handleAPITags(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	tagCounts := map[string]int{}
	rows, err := db.Query("SELECT tags FROM videos WHERE tags != '' AND tags IS NOT NULL AND " + playableMediaSQL + " LIMIT 5000")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		rows.Scan(&t)
		for _, tag := range strings.Split(t, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tagCounts[tag]++
			}
		}
	}
	type tagCount struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	var sorted []tagCount
	for name, count := range tagCounts {
		sorted = append(sorted, tagCount{name, count})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Count > sorted[j].Count })
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	json.NewEncoder(w).Encode(sorted)
}
