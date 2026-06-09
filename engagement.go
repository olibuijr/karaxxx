package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func handlePlaylistListCreate(w http.ResponseWriter, r *http.Request) {
	uid, _, ok := authMiddleware(w, r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	if r.Method == "POST" {
		var body struct{ Name string `json:"name"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			http.Error(w, `{"error":"name required"}`, 400)
			return
		}
		res, err := db.Exec("INSERT INTO playlists (user_id, name) VALUES (?, ?)", uid, body.Name)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		id, _ := res.LastInsertId()
		json.NewEncoder(w).Encode(map[string]int64{"id": id})
		return
	}
	if r.Method == "GET" {
		rows, err := db.Query(
			`SELECT p.id, p.name, p.created_at, (SELECT COUNT(*) FROM playlist_videos WHERE playlist_id = p.id) as video_count
			 FROM playlists p WHERE p.user_id = ? ORDER BY p.created_at DESC`, uid)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		type Playlist struct {
			ID         int    `json:"id"`
			Name       string `json:"name"`
			VideoCount int    `json:"video_count"`
			CreatedAt  string `json:"created_at"`
		}
		playlists := []Playlist{}
		for rows.Next() {
			var pl Playlist
			rows.Scan(&pl.ID, &pl.Name, &pl.CreatedAt, &pl.VideoCount)
			playlists = append(playlists, pl)
		}
		json.NewEncoder(w).Encode(playlists)
		return
	}
	http.Error(w, "method not allowed", 405)
}

func handlePlaylistRouter(w http.ResponseWriter, r *http.Request) {
	uid, _, ok := authMiddleware(w, r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/playlists/")
	parts := strings.SplitN(path, "/", 2)
	playlistID := parts[0]
	subPath := ""
	if len(parts) > 1 {
		subPath = parts[1]
	}

	var ownerID int
	db.QueryRow("SELECT user_id FROM playlists WHERE id = ?", playlistID).Scan(&ownerID)
	if ownerID != uid {
		http.Error(w, `{"error":"not found"}`, 404)
		return
	}

	pid, _ := strconv.Atoi(playlistID)

	if subPath == "" {
		if r.Method == "PUT" {
			var body struct{ Name string `json:"name"` }
			json.NewDecoder(r.Body).Decode(&body)
			db.Exec("UPDATE playlists SET name = ? WHERE id = ? AND user_id = ?", body.Name, pid, uid)
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
			return
		}
		if r.Method == "DELETE" {
			db.Exec("DELETE FROM playlist_videos WHERE playlist_id = ?", pid)
			db.Exec("DELETE FROM playlists WHERE id = ? AND user_id = ?", pid, uid)
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
			return
		}
		if r.Method == "GET" {
			rows, err := db.Query(
				`SELECT v.id, v.slug, v.title, v.description, v.categories, v.duration, v.views, v.thumb_uuid, v.preview_url, v.added_at, v.upload_date, v.source, pv.position
				 FROM playlist_videos pv JOIN videos v ON pv.video_id = v.id
				 WHERE pv.playlist_id = ? ORDER BY pv.position ASC`, pid)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			defer rows.Close()
			type PLVideo struct {
				Video
				Position int `json:"position"`
			}
			videos := []PLVideo{}
			for rows.Next() {
				vv := Video{}
				var dur, views sql.NullInt64
				var cats, uploadDate sql.NullString
				var pos int
				rows.Scan(&vv.ID, &vv.Slug, &vv.Title, &vv.Description, &cats, &dur, &views, &vv.ThumbUUID, &vv.PreviewURL, &vv.AddedAt, &uploadDate, &vv.Source, &pos)
				vv.Duration = int(dur.Int64)
				vv.Views = int(views.Int64)
				if cats.Valid && cats.String != "" {
					vv.Categories = strings.Split(cats.String, ",")
				}
				if uploadDate.Valid {
					vv.UploadDate = uploadDate.String
				}
				videos = append(videos, PLVideo{Video: vv, Position: pos})
			}
			json.NewEncoder(w).Encode(videos)
			return
		}
	}

	if subPath == "videos" && r.Method == "POST" {
		var body struct{ VideoID string `json:"video_id"` }
		json.NewDecoder(r.Body).Decode(&body)
		var maxPos int
		db.QueryRow("SELECT COALESCE(MAX(position), -1) FROM playlist_videos WHERE playlist_id = ?", pid).Scan(&maxPos)
		db.Exec("INSERT OR IGNORE INTO playlist_videos (playlist_id, video_id, position) VALUES (?, ?, ?)", pid, body.VideoID, maxPos+1)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		return
	}

	if strings.HasPrefix(subPath, "videos/") {
		videoID := strings.TrimPrefix(subPath, "videos/")
		if r.Method == "DELETE" {
			db.Exec("DELETE FROM playlist_videos WHERE playlist_id = ? AND video_id = ?", pid, videoID)
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
			return
		}
	}

	if subPath == "reorder" && r.Method == "PUT" {
		var body struct{ VideoIDs []string `json:"video_ids"` }
		json.NewDecoder(r.Body).Decode(&body)
		for i, vid := range body.VideoIDs {
			db.Exec("UPDATE playlist_videos SET position = ? WHERE playlist_id = ? AND video_id = ?", i, pid, vid)
		}
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		return
	}

	http.Error(w, "not found", 404)
}

func handleRateVideo(w http.ResponseWriter, r *http.Request) {
	uid, _, ok := authMiddleware(w, r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	videoID := strings.TrimPrefix(r.URL.Path, "/api/rate/")
	videoID = strings.TrimSuffix(videoID, "/")
	if videoID == "" {
		http.Error(w, "missing video id", 400)
		return
	}

	if r.Method == "POST" {
		var body struct{ Rating int `json:"rating"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		if body.Rating == 0 {
			db.Exec("DELETE FROM ratings WHERE user_id = ? AND video_id = ?", uid, videoID)
		} else {
			db.Exec("INSERT OR REPLACE INTO ratings (user_id, video_id, rating) VALUES (?, ?, ?)", uid, videoID, body.Rating)
		}
		var up, down int
		db.QueryRow("SELECT SUM(CASE WHEN rating = 1 THEN 1 ELSE 0 END), SUM(CASE WHEN rating = -1 THEN 1 ELSE 0 END) FROM ratings WHERE video_id = ?", videoID).Scan(&up, &down)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"rating":     body.Rating,
			"up_count":   up,
			"down_count": down,
		})
		return
	}

	if r.Method == "GET" {
		var userRating int
		err := db.QueryRow("SELECT rating FROM ratings WHERE user_id = ? AND video_id = ?", uid, videoID).Scan(&userRating)
		if err != nil {
			userRating = 0
		}
		var up, down int
		db.QueryRow("SELECT SUM(CASE WHEN rating = 1 THEN 1 ELSE 0 END), SUM(CASE WHEN rating = -1 THEN 1 ELSE 0 END) FROM ratings WHERE video_id = ?", videoID).Scan(&up, &down)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"rating":     userRating,
			"up_count":   up,
			"down_count": down,
		})
		return
	}

	http.Error(w, "method not allowed", 405)
}

func handleWatchLaterList(w http.ResponseWriter, r *http.Request) {
	uid, _, ok := authMiddleware(w, r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	rows, err := db.Query(
		`SELECT v.id, v.slug, v.title, v.description, v.categories, v.duration, v.views, v.thumb_uuid, v.preview_url, v.added_at, v.upload_date, v.source, wl.position, wl.added_at
		 FROM watch_later wl JOIN videos v ON wl.video_id = v.id
		 WHERE wl.user_id = ? ORDER BY wl.added_at DESC LIMIT ?`, uid, limit)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type WLVideo struct {
		Video
		Position int    `json:"position"`
		AddedAt  string `json:"added_at"`
	}
	videos := []WLVideo{}
	for rows.Next() {
		vv := Video{}
		var dur, views sql.NullInt64
		var cats, uploadDate sql.NullString
		var pos int
		var addedAt string
		rows.Scan(&vv.ID, &vv.Slug, &vv.Title, &vv.Description, &cats, &dur, &views, &vv.ThumbUUID, &vv.PreviewURL, &vv.AddedAt, &uploadDate, &vv.Source, &pos, &addedAt)
		vv.Duration = int(dur.Int64)
		vv.Views = int(views.Int64)
		if cats.Valid && cats.String != "" {
			vv.Categories = strings.Split(cats.String, ",")
		}
		if uploadDate.Valid {
			vv.UploadDate = uploadDate.String
		}
		videos = append(videos, WLVideo{Video: vv, Position: pos, AddedAt: addedAt})
	}
	json.NewEncoder(w).Encode(videos)
}

func handleWatchLaterRouter(w http.ResponseWriter, r *http.Request) {
	uid, _, ok := authMiddleware(w, r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	videoID := strings.TrimPrefix(r.URL.Path, "/api/watch-later/")
	videoID = strings.TrimSuffix(videoID, "/")
	if videoID == "" {
		http.Error(w, "missing video id", 400)
		return
	}
	if r.Method == "POST" {
		db.Exec("INSERT OR IGNORE INTO watch_later (user_id, video_id, position) VALUES (?, ?, 0)", uid, videoID)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		return
	}
	if r.Method == "DELETE" {
		db.Exec("DELETE FROM watch_later WHERE user_id = ? AND video_id = ?", uid, videoID)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		return
	}
	http.Error(w, "method not allowed", 405)
}
