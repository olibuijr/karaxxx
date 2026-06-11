package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
)

var anonymousMascots = []string{
	"Velvet Nova", "Neon Pulse", "Scarlet Orbit", "Ember Halo",
	"Midnight Spark", "Chrome Muse", "Ruby Signal", "Afterglow",
	"Hidden Rhythm", "Lunar Vibe", "Silk Circuit", "Mellow Static",
}

var allowedReactions = map[string]bool{
	"like": true, "fire": true, "heart": true, "peach": true, "spark": true,
}

func createAnonymousName() string {
	name := anonymousMascots[rand.Intn(len(anonymousMascots))]
	return fmt.Sprintf("Anonymous %s %02d", name, rand.Intn(90)+10)
}

func cleanSocialText(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	return s
}

func getPublicDisplayName(userID int, username string) (string, bool) {
	var displayName, anonymousName string
	var anonymous int
	err := db.QueryRow(`SELECT COALESCE(display_name, ''), anonymous_name, comment_anonymously
		FROM user_profiles WHERE user_id = ?`, userID).Scan(&displayName, &anonymousName, &anonymous)
	if err != nil {
		anonymousName = createAnonymousName()
		db.Exec("INSERT OR IGNORE INTO user_profiles (user_id, anonymous_name) VALUES (?, ?)", userID, anonymousName)
		anonymous = 1
	}
	if anonymous == 1 {
		return anonymousName, true
	}
	if strings.TrimSpace(displayName) != "" {
		return displayName, false
	}
	return username, false
}

func handleProfileSettings(w http.ResponseWriter, r *http.Request) {
	uid, un, ok := authMiddleware(w, r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "GET" {
		writeProfileSettings(w, uid, un)
		return
	}

	if r.Method == "PUT" || r.Method == "POST" {
		var body struct {
			DisplayName        string `json:"display_name"`
			CommentAnonymously bool   `json:"comment_anonymously"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid body")
			return
		}
		displayName := cleanSocialText(body.DisplayName, 40)
		anonymousInt := 0
		if body.CommentAnonymously {
			anonymousInt = 1
		}
		_, err := db.Exec(`UPDATE user_profiles SET display_name = ?, comment_anonymously = ?, updated_at = datetime('now') WHERE user_id = ?`,
			displayName, anonymousInt, uid)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeProfileSettings(w, uid, un)
		return
	}

	writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeProfileSettings(w http.ResponseWriter, uid int, username string) {
	var displayName, anonymousName string
	var anonymous int
	db.QueryRow(`SELECT COALESCE(display_name, ''), anonymous_name, comment_anonymously
		FROM user_profiles WHERE user_id = ?`, uid).Scan(&displayName, &anonymousName, &anonymous)
	if anonymousName == "" {
		anonymousName = createAnonymousName()
		db.Exec("INSERT OR REPLACE INTO user_profiles (user_id, display_name, anonymous_name, comment_anonymously, updated_at) VALUES (?, ?, ?, 1, datetime('now'))", uid, displayName, anonymousName)
		anonymous = 1
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"username":              username,
		"display_name":          displayName,
		"anonymous_name":        anonymousName,
		"comment_anonymously":   anonymous == 1,
		"public_commenter_name": map[bool]string{true: anonymousName, false: firstNonEmpty(displayName, username)}[anonymous == 1],
	})
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func handleVideoSocialRouter(w http.ResponseWriter, r *http.Request) {
	uid, un, ok := authMiddleware(w, r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/social/video/"), "/")
	if path == "" {
		writeJSONError(w, http.StatusBadRequest, "missing video id")
		return
	}
	parts := strings.Split(path, "/")
	videoID := parts[0]

	if len(parts) == 1 && r.Method == "GET" {
		writeVideoSocial(w, uid, videoID)
		return
	}

	if len(parts) == 2 && parts[1] == "comments" && r.Method == "POST" {
		var body struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid body")
			return
		}
		text := cleanSocialText(body.Body, 500)
		if text == "" {
			writeJSONError(w, http.StatusBadRequest, "comment required")
			return
		}
		name, anonymous := getPublicDisplayName(uid, un)
		anonInt := 0
		if anonymous {
			anonInt = 1
		}
		if _, err := db.Exec(`INSERT INTO video_comments (video_id, user_id, body, display_name, anonymous)
			VALUES (?, ?, ?, ?, ?)`, videoID, uid, text, name, anonInt); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeVideoSocial(w, uid, videoID)
		return
	}

	if len(parts) == 2 && parts[1] == "reactions" && r.Method == "POST" {
		var body struct {
			Reaction string `json:"reaction"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid body")
			return
		}
		reaction := strings.ToLower(strings.TrimSpace(body.Reaction))
		if !allowedReactions[reaction] {
			writeJSONError(w, http.StatusBadRequest, "invalid reaction")
			return
		}
		var exists int
		db.QueryRow("SELECT COUNT(*) FROM video_reactions WHERE video_id = ? AND user_id = ? AND reaction = ?", videoID, uid, reaction).Scan(&exists)
		if exists > 0 {
			db.Exec("DELETE FROM video_reactions WHERE video_id = ? AND user_id = ? AND reaction = ?", videoID, uid, reaction)
		} else {
			db.Exec("INSERT OR IGNORE INTO video_reactions (video_id, user_id, reaction) VALUES (?, ?, ?)", videoID, uid, reaction)
		}
		writeVideoSocial(w, uid, videoID)
		return
	}

	writeJSONError(w, http.StatusNotFound, "not found")
}

func writeVideoSocial(w http.ResponseWriter, uid int, videoID string) {
	w.Header().Set("Content-Type", "application/json")
	type comment struct {
		ID          int    `json:"id"`
		DisplayName string `json:"display_name"`
		Body        string `json:"body"`
		Anonymous   bool   `json:"anonymous"`
		CreatedAt   string `json:"created_at"`
	}
	comments := []comment{}
	rows, err := db.Query(`SELECT id, display_name, body, anonymous, created_at
		FROM video_comments WHERE video_id = ? ORDER BY created_at ASC LIMIT 80`, videoID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var c comment
			var anonymous int
			rows.Scan(&c.ID, &c.DisplayName, &c.Body, &anonymous, &c.CreatedAt)
			c.Anonymous = anonymous == 1
			comments = append(comments, c)
		}
	}

	reactions := map[string]int{}
	rrows, err := db.Query("SELECT reaction, COUNT(*) FROM video_reactions WHERE video_id = ? GROUP BY reaction", videoID)
	if err == nil {
		defer rrows.Close()
		for rrows.Next() {
			var reaction string
			var count int
			rrows.Scan(&reaction, &count)
			reactions[reaction] = count
		}
	}
	userReactions := []string{}
	urows, err := db.Query("SELECT reaction FROM video_reactions WHERE video_id = ? AND user_id = ?", videoID, uid)
	if err == nil {
		defer urows.Close()
		for urows.Next() {
			var reaction string
			urows.Scan(&reaction)
			userReactions = append(userReactions, reaction)
		}
	}

	var watchCount int
	db.QueryRow("SELECT COALESCE(watch_count, 0) FROM video_watch_counts WHERE video_id = ?", videoID).Scan(&watchCount)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"comments":       comments,
		"reactions":      reactions,
		"user_reactions": userReactions,
		"watch_count":    watchCount,
	})
}

func handleWallRouter(w http.ResponseWriter, r *http.Request) {
	viewerID, viewerName, ok := authMiddleware(w, r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/wall/"), "/")
	if path == "" {
		writeJSONError(w, http.StatusBadRequest, "missing username")
		return
	}
	parts := strings.Split(path, "/")
	username, err := urlPathUnescape(parts[0])
	if err != nil || username == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid username")
		return
	}
	var wallUserID int
	var wallUsername string
	err = db.QueryRow("SELECT id, username FROM users WHERE username = ?", username).Scan(&wallUserID, &wallUsername)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "wall not found")
		return
	}

	if len(parts) == 2 && parts[1] == "comments" && r.Method == "POST" {
		var body struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid body")
			return
		}
		text := cleanSocialText(body.Body, 500)
		if text == "" {
			writeJSONError(w, http.StatusBadRequest, "comment required")
			return
		}
		name, anonymous := getPublicDisplayName(viewerID, viewerName)
		anonInt := 0
		if anonymous {
			anonInt = 1
		}
		db.Exec(`INSERT INTO wall_comments (wall_user_id, author_id, body, display_name, anonymous)
			VALUES (?, ?, ?, ?, ?)`, wallUserID, viewerID, text, name, anonInt)
		writeWall(w, wallUserID, wallUsername, viewerID)
		return
	}

	if len(parts) == 1 && r.Method == "GET" {
		writeWall(w, wallUserID, wallUsername, viewerID)
		return
	}

	writeJSONError(w, http.StatusNotFound, "not found")
}

func urlPathUnescape(s string) (string, error) {
	return url.PathUnescape(s)
}

func writeWall(w http.ResponseWriter, wallUserID int, username string, viewerID int) {
	w.Header().Set("Content-Type", "application/json")
	var displayName, anonymousName string
	var anonymous int
	db.QueryRow(`SELECT COALESCE(display_name, ''), anonymous_name, comment_anonymously
		FROM user_profiles WHERE user_id = ?`, wallUserID).Scan(&displayName, &anonymousName, &anonymous)
	publicName := firstNonEmpty(displayName, username)
	if anonymous == 1 {
		publicName = anonymousName
	}

	favCats := []string{}
	catRows, err := db.Query("SELECT category FROM fav_categories WHERE user_id = ? ORDER BY created_at DESC", wallUserID)
	if err == nil {
		defer catRows.Close()
		for catRows.Next() {
			var c string
			catRows.Scan(&c)
			favCats = append(favCats, c)
		}
	}

	favVideos := []Video{}
	videoRows, err := db.Query(`SELECT v.id, COALESCE(v.slug,''), COALESCE(v.title,''), COALESCE(v.description,''), v.categories,
			COALESCE(v.duration,0), COALESCE(v.views,0), COALESCE(v.thumb_uuid,''), COALESCE(v.preview_url,''),
			COALESCE(v.added_at,''), COALESCE(v.upload_date,''), COALESCE(v.source,'xnxx')
		FROM favorites f JOIN videos v ON f.video_id = v.id
		WHERE f.user_id = ? AND `+playableMediaSQLV+`
		ORDER BY f.created_at DESC LIMIT 24`, wallUserID)
	if err == nil {
		defer videoRows.Close()
		for videoRows.Next() {
			v := Video{}
			var cats string
			videoRows.Scan(&v.ID, &v.Slug, &v.Title, &v.Description, &cats, &v.Duration, &v.Views, &v.ThumbUUID, &v.PreviewURL, &v.AddedAt, &v.UploadDate, &v.Source)
			if cats != "" {
				v.Categories = strings.Split(cats, ",")
			}
			favVideos = append(favVideos, v)
		}
	}

	type wallComment struct {
		ID          int    `json:"id"`
		DisplayName string `json:"display_name"`
		Body        string `json:"body"`
		Anonymous   bool   `json:"anonymous"`
		CreatedAt   string `json:"created_at"`
	}
	comments := []wallComment{}
	rows, err := db.Query(`SELECT id, display_name, body, anonymous, created_at
		FROM wall_comments WHERE wall_user_id = ? ORDER BY created_at DESC LIMIT 80`, wallUserID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var c wallComment
			var anon int
			rows.Scan(&c.ID, &c.DisplayName, &c.Body, &anon, &c.CreatedAt)
			c.Anonymous = anon == 1
			comments = append(comments, c)
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"user": map[string]interface{}{
			"id":          wallUserID,
			"username":    username,
			"public_name": publicName,
			"is_self":     wallUserID == viewerID,
		},
		"favorite_categories": favCats,
		"favorite_videos":     favVideos,
		"comments":            comments,
		"privacy_note":        "Public activity is shown with the chosen commenter name. Aggregate watches and reactions are anonymous and used for quality improvements.",
	})
}

func incrementVideoWatch(userID int, videoID string) {
	tx, err := db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()
	tx.Exec(`INSERT INTO video_watch_counts (video_id, watch_count, updated_at)
		VALUES (?, 1, datetime('now'))
		ON CONFLICT(video_id) DO UPDATE SET watch_count = watch_count + 1, updated_at = datetime('now')`, videoID)
	tx.Exec(`INSERT INTO watch_history (user_id, video_id, position, play_count, watched_at, updated_at)
		VALUES (?, ?, 0, 1, datetime('now'), datetime('now'))
		ON CONFLICT(user_id, video_id) DO UPDATE SET play_count = COALESCE(play_count, 0) + 1, updated_at = datetime('now')`, userID, videoID)
	tx.Commit()
}
