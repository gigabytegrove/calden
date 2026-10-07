package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const maxAvatarBytes int64 = 10 << 20

func (s *server) avatarMedia(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(strings.TrimSpace(r.PathValue("name")))
	if name == "." || name == "" || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.dataDir, "avatars", name)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, path)
}

func (s *server) uploadUserAvatar(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid person")
		return
	}
	actor := currentActor(r)
	if actor.Role != "admin" && actor.ID != userID {
		writeError(w, 403, "You cannot change this profile image")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes+(512<<10))
	if err := r.ParseMultipartForm(maxAvatarBytes); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "Profile image must be 10 MB or smaller")
		return
	}
	file, header, err := r.FormFile("avatar")
	if err != nil {
		writeError(w, 400, "Choose a profile image")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, maxAvatarBytes+1))
	if err != nil {
		writeError(w, 400, "Could not read profile image")
		return
	}
	if int64(len(content)) > maxAvatarBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "Profile image must be 10 MB or smaller")
		return
	}
	if len(content) < 16 {
		writeError(w, 400, "That file is not a valid image")
		return
	}

	contentType := http.DetectContentType(content)
	ext := ""
	switch contentType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/webp":
		ext = ".webp"
	default:
		writeError(w, 400, "Use a JPG, PNG, or WebP profile image")
		return
	}

	var exists bool
	if err := s.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)", userID).Scan(&exists); err != nil || !exists {
		writeError(w, 404, "Person not found")
		return
	}

	dir := filepath.Join(s.dataDir, "avatars")
	if err := os.MkdirAll(dir, 0700); err != nil {
		writeError(w, 500, "Could not prepare profile image storage")
		return
	}
	filename := userID.String() + ext
	target := filepath.Join(dir, filename)
	temp := target + ".tmp"
	if err := os.WriteFile(temp, content, 0600); err != nil {
		writeError(w, 500, "Could not save profile image")
		return
	}
	if err := os.Rename(temp, target); err != nil {
		_ = os.Remove(temp)
		writeError(w, 500, "Could not save profile image")
		return
	}

	entries, _ := os.ReadDir(dir)
	prefix := userID.String() + "."
	for _, entry := range entries {
		if entry.Name() != filename && strings.HasPrefix(entry.Name(), prefix) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}

	avatarURL := "/media/avatars/" + filename
	if _, err := s.db.Exec(r.Context(), "UPDATE users SET avatar_url=$2,updated_at=now() WHERE id=$1", userID, avatarURL); err != nil {
		_ = os.Remove(target)
		writeError(w, 500, "Could not attach profile image to this person")
		return
	}

	s.audit(r, "update_avatar", "user", &userID, "Updated profile image", map[string]any{
		"filename": header.Filename,
		"content_type": contentType,
	})
	writeJSON(w, http.StatusOK, map[string]any{"avatar_url": avatarURL})
}

func (s *server) deleteUserAvatar(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid person")
		return
	}
	actor := currentActor(r)
	if actor.Role != "admin" && actor.ID != userID {
		writeError(w, 403, "You cannot change this profile image")
		return
	}

	var avatarURL *string
	err = s.db.QueryRow(r.Context(), "SELECT avatar_url FROM users WHERE id=$1", userID).Scan(&avatarURL)
	if err != nil {
		writeError(w, 404, "Person not found")
		return
	}
	if avatarURL != nil && strings.HasPrefix(*avatarURL, "/media/avatars/") {
		name := filepath.Base(strings.TrimPrefix(*avatarURL, "/media/avatars/"))
		if name != "." && !strings.Contains(name, "..") {
			_ = os.Remove(filepath.Join(s.dataDir, "avatars", name))
		}
	}
	if _, err := s.db.Exec(r.Context(), "UPDATE users SET avatar_url=NULL,updated_at=now() WHERE id=$1", userID); err != nil {
		writeError(w, 500, "Could not remove profile image")
		return
	}
	s.audit(r, "delete_avatar", "user", &userID, "Removed profile image", nil)
	w.WriteHeader(http.StatusNoContent)
}

