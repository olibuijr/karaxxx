package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func handleForYou(w http.ResponseWriter, r *http.Request) {
	uid, _, ok := authMiddleware(w, r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	weightedCats := map[string]int{}
	var rows *sql.Rows
	var err error

	rows, err = db.Query("SELECT category FROM fav_categories WHERE user_id = ?", uid)
	if err == nil {
		for rows.Next() {
			var c string
			rows.Scan(&c)
			weightedCats[c] += 3
		}
		rows.Close()
	}

	rows, err = db.Query(
		`SELECT v.categories FROM watch_history wh JOIN videos v ON wh.video_id = v.id WHERE wh.user_id = ?`, uid)
	if err == nil {
		for rows.Next() {
			var c string
			rows.Scan(&c)
			for _, cat := range strings.Split(c, ",") {
				cat = strings.TrimSpace(cat)
				if cat != "" && cat != "uncategorized" {
					weightedCats[cat] += 2
				}
			}
		}
		rows.Close()
	}

	rows, err = db.Query(
		"SELECT v.categories FROM ratings r JOIN videos v ON r.video_id = v.id WHERE r.user_id = ? AND r.rating = 1", uid)
	if err == nil {
		for rows.Next() {
			var c string
			rows.Scan(&c)
			for _, cat := range strings.Split(c, ",") {
				cat = strings.TrimSpace(cat)
				if cat != "" && cat != "uncategorized" {
					weightedCats[cat]++
				}
			}
		}
		rows.Close()
	}

	if len(weightedCats) == 0 {
		rows, err = db.Query(
			"SELECT id, slug, title, description, categories, duration, views, thumb_uuid, preview_url, added_at, upload_date, source FROM videos ORDER BY views DESC LIMIT ?", limit)
		if err == nil {
			defer rows.Close()
			videos := []Video{}
			for rows.Next() {
				vv := Video{}
				var dur, views sql.NullInt64
				var cats, uploadDate sql.NullString
				rows.Scan(&vv.ID, &vv.Slug, &vv.Title, &vv.Description, &cats, &dur, &views, &vv.ThumbUUID, &vv.PreviewURL, &vv.AddedAt, &uploadDate, &vv.Source)
				vv.Duration = int(dur.Int64)
				vv.Views = int(views.Int64)
				if cats.Valid && cats.String != "" { vv.Categories = strings.Split(cats.String, ",") }
				if uploadDate.Valid { vv.UploadDate = uploadDate.String }
				videos = append(videos, vv)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"videos": videos, "reason": "trending"})
		}
		return
	}

	var catClauses []string
	var catArgs []interface{}
	for cat := range weightedCats {
		catClauses = append(catClauses, "v.categories LIKE ?")
		catArgs = append(catArgs, "%"+cat+"%")
	}
	catArgs = append(catArgs, uid, uid, limit)

	rows, err = db.Query(
		`SELECT v.id, v.slug, v.title, v.description, v.categories, v.duration, v.views, v.thumb_uuid, v.preview_url, v.added_at, v.upload_date, v.source FROM videos v
		 WHERE (`+strings.Join(catClauses, " OR ")+`)
		 AND v.id NOT IN (SELECT video_id FROM watch_history WHERE user_id = ?)
		 AND v.id NOT IN (SELECT video_id FROM favorites WHERE user_id = ?)
		 ORDER BY v.views DESC LIMIT ?`, catArgs...)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type ForYouVideo struct {
		Video
		Reason string `json:"reason"`
	}
	videos := []ForYouVideo{}
	for rows.Next() {
		vv := Video{}
		var dur, views sql.NullInt64
		var cats, uploadDate sql.NullString
		rows.Scan(&vv.ID, &vv.Slug, &vv.Title, &vv.Description, &cats, &dur, &views, &vv.ThumbUUID, &vv.PreviewURL, &vv.AddedAt, &uploadDate, &vv.Source)
		vv.Duration = int(dur.Int64)
		vv.Views = int(views.Int64)
		if cats.Valid && cats.String != "" { vv.Categories = strings.Split(cats.String, ",") }
		if uploadDate.Valid { vv.UploadDate = uploadDate.String }
		reason := ""
		if len(vv.Categories) > 0 {
			reason = "Because you liked " + vv.Categories[0]
		}
		videos = append(videos, ForYouVideo{Video: vv, Reason: reason})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"videos": videos})
}

func handleSuggestions(w http.ResponseWriter, r *http.Request) {
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

	result := map[string][]Video{}
	for rows.Next() {
		var cat string
		rows.Scan(&cat)
		vrows, err := db.Query(
			"SELECT id, slug, title, description, categories, duration, views, thumb_uuid, preview_url, added_at, upload_date, source FROM videos WHERE categories LIKE ? ORDER BY views DESC LIMIT 3",
			"%"+cat+"%")
		if err != nil {
			continue
		}
		var vids []Video
		for vrows.Next() {
			vv := Video{}
			var dur, views sql.NullInt64
			var cats, uploadDate sql.NullString
			vrows.Scan(&vv.ID, &vv.Slug, &vv.Title, &vv.Description, &cats, &dur, &views, &vv.ThumbUUID, &vv.PreviewURL, &vv.AddedAt, &uploadDate, &vv.Source)
			vv.Duration = int(dur.Int64)
			vv.Views = int(views.Int64)
			if cats.Valid && cats.String != "" { vv.Categories = strings.Split(cats.String, ",") }
			if uploadDate.Valid { vv.UploadDate = uploadDate.String }
			vids = append(vids, vv)
		}
		vrows.Close()
		if len(vids) > 0 {
			result[cat] = vids
		}
	}
	json.NewEncoder(w).Encode(result)
}

func handleProfile(w http.ResponseWriter, r *http.Request) {
	uid, un, ok := authMiddleware(w, r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}

	var accountAge float64
	db.QueryRow("SELECT julianday('now') - julianday(created_at) FROM users WHERE id = ?", uid).Scan(&accountAge)

	var totalWatched int
	db.QueryRow("SELECT COUNT(*) FROM watch_history WHERE user_id = ?", uid).Scan(&totalWatched)

	var totalWatchTime int
	db.QueryRow("SELECT COALESCE(SUM(position), 0) FROM watch_history WHERE user_id = ?", uid).Scan(&totalWatchTime)

	var playlistCount int
	db.QueryRow("SELECT COUNT(*) FROM playlists WHERE user_id = ?", uid).Scan(&playlistCount)

	var favCount int
	db.QueryRow("SELECT COUNT(*) FROM favorites WHERE user_id = ?", uid).Scan(&favCount)

	var ratingsGiven int
	var ratingUp int
	db.QueryRow("SELECT COUNT(*), COALESCE(SUM(CASE WHEN rating=1 THEN 1 ELSE 0 END), 0) FROM ratings WHERE user_id = ?", uid).Scan(&ratingsGiven, &ratingUp)

	ratingRatio := 0.0
	if ratingsGiven > 0 {
		ratingRatio = float64(ratingUp) / float64(ratingsGiven)
	}

	topCategories := []map[string]interface{}{}
	topCatRows, err := db.Query(
		`SELECT v.categories, COUNT(*) as cnt FROM watch_history wh JOIN videos v ON wh.video_id = v.id WHERE wh.user_id = ? AND v.categories != '' AND v.categories IS NOT NULL GROUP BY v.categories ORDER BY cnt DESC LIMIT 5`, uid)
	if err == nil {
		defer topCatRows.Close()
		for topCatRows.Next() {
			var c string
			var cnt int
			topCatRows.Scan(&c, &cnt)
			for _, cat := range strings.Split(c, ",") {
				cat = strings.TrimSpace(cat)
				if cat != "" && cat != "uncategorized" {
					topCategories = append(topCategories, map[string]interface{}{"name": cat, "count": cnt})
				}
			}
		}
	}

	recentWatched := []Video{}
	recentRows, err := db.Query(
		`SELECT v.id, v.slug, v.title, v.description, v.categories, v.duration, v.views, v.thumb_uuid, v.preview_url, v.added_at, v.upload_date, v.source
		 FROM watch_history wh JOIN videos v ON wh.video_id = v.id WHERE wh.user_id = ? ORDER BY wh.updated_at DESC LIMIT 5`, uid)
	if err == nil {
		defer recentRows.Close()
		for recentRows.Next() {
			vv := Video{}
			var dur, views sql.NullInt64
			var cats, uploadDate sql.NullString
			recentRows.Scan(&vv.ID, &vv.Slug, &vv.Title, &vv.Description, &cats, &dur, &views, &vv.ThumbUUID, &vv.PreviewURL, &vv.AddedAt, &uploadDate, &vv.Source)
			vv.Duration = int(dur.Int64)
			vv.Views = int(views.Int64)
			if cats.Valid && cats.String != "" { vv.Categories = strings.Split(cats.String, ",") }
			if uploadDate.Valid { vv.UploadDate = uploadDate.String }
			recentWatched = append(recentWatched, vv)
		}
	}

	topRated := []Video{}
	topRows, err := db.Query(
		`SELECT v.id, v.slug, v.title, v.description, v.categories, v.duration, v.views, v.thumb_uuid, v.preview_url, v.added_at, v.upload_date, v.source
		 FROM ratings r JOIN videos v ON r.video_id = v.id WHERE r.user_id = ? AND r.rating = 1 ORDER BY r.created_at DESC LIMIT 5`, uid)
	if err == nil {
		defer topRows.Close()
		for topRows.Next() {
			vv := Video{}
			var dur, views sql.NullInt64
			var cats, uploadDate sql.NullString
			topRows.Scan(&vv.ID, &vv.Slug, &vv.Title, &vv.Description, &cats, &dur, &views, &vv.ThumbUUID, &vv.PreviewURL, &vv.AddedAt, &uploadDate, &vv.Source)
			vv.Duration = int(dur.Int64)
			vv.Views = int(views.Int64)
			if cats.Valid && cats.String != "" { vv.Categories = strings.Split(cats.String, ",") }
			if uploadDate.Valid { vv.UploadDate = uploadDate.String }
			topRated = append(topRated, vv)
		}
	}

	favCats := []string{}
	fcRows, err := db.Query("SELECT category FROM fav_categories WHERE user_id = ?", uid)
	if err == nil {
		defer fcRows.Close()
		for fcRows.Next() {
			var c string
			fcRows.Scan(&c)
			favCats = append(favCats, c)
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"username":                un,
		"account_age_days":        int(accountAge),
		"total_watched":           totalWatched,
		"total_watch_time_seconds": totalWatchTime,
		"favorite_categories":     favCats,
		"top_categories":          topCategories,
		"playlist_count":          playlistCount,
		"favorite_count":          favCount,
		"ratings_given":           ratingsGiven,
		"rating_ratio":            ratingRatio,
		"recently_watched":        recentWatched,
		"top_rated":               topRated,
	})
}
