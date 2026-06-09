package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func handleWatchRouter(w http.ResponseWriter, r *http.Request) {
	uid, _, ok := authMiddleware(w, r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/watch/")

	if r.Method == "GET" && path == "history" {
		limit := 20
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		rows, err := db.Query(
			`SELECT v.id, v.slug, v.title, v.description, v.categories, v.duration, v.views, v.thumb_uuid, v.preview_url, v.added_at, v.upload_date, v.source, v.url_360, v.url_720, v.url_1080, v.hls_url, COALESCE(wh.position, 0), wh.updated_at
			 FROM watch_history wh JOIN videos v ON wh.video_id = v.id
			 WHERE wh.user_id = ? ORDER BY wh.updated_at DESC LIMIT ?`, uid, limit)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		type WatchVideo struct {
			Video
			WatchedPosition int    `json:"watched_position"`
			WatchedAt       string `json:"watched_at"`
		}
		result := []WatchVideo{}
		for rows.Next() {
			vv := Video{}
			var dur, views sql.NullInt64
			var cats, uploadDate sql.NullString
			var pos int
			var watchedAt string
			rows.Scan(&vv.ID, &vv.Slug, &vv.Title, &vv.Description, &cats, &dur, &views, &vv.ThumbUUID, &vv.PreviewURL, &vv.AddedAt, &uploadDate, &vv.Source, &vv.URL360, &vv.URL720, &vv.URL1080, &vv.HLSURL, &pos, &watchedAt)
			vv.Duration = int(dur.Int64)
			vv.Views = int(views.Int64)
			if cats.Valid && cats.String != "" {
				vv.Categories = strings.Split(cats.String, ",")
			}
			if uploadDate.Valid {
				vv.UploadDate = uploadDate.String
			}
			result = append(result, WatchVideo{Video: vv, WatchedPosition: pos, WatchedAt: watchedAt})
		}
		json.NewEncoder(w).Encode(result)
		return
	}

	if r.Method == "POST" {
		videoID := path
		if videoID == "" {
			http.Error(w, `{"error":"missing video id"}`, 400)
			return
		}
		var body struct {
			Position int `json:"position"`
			Duration int `json:"duration,omitempty"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		var existsID string
		db.QueryRow("SELECT id FROM videos WHERE id = ?", videoID).Scan(&existsID)
		if existsID == "" {
			http.Error(w, `{"error":"video not found"}`, 404)
			return
		}
		db.Exec("INSERT OR REPLACE INTO watch_history (user_id, video_id, position, duration, updated_at) VALUES (?, ?, ?, ?, datetime('now'))",
			uid, videoID, body.Position, body.Duration)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		return
	}

	if r.Method == "DELETE" {
		videoID := path
		if videoID == "" || videoID == "history" {
			http.Error(w, `{"error":"missing video id"}`, 400)
			return
		}
		db.Exec("DELETE FROM watch_history WHERE user_id = ? AND video_id = ?", uid, videoID)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		return
	}

	http.Error(w, "method not allowed", 405)
}
