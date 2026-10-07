package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxRestoreUploadBytes int64 = 4 << 30

func (s *server) backupStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.backup.Status())
}

func (s *server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	path, filename, err := s.backup.CreateBundle(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create CalDen backup: "+err.Error())
		return
	}
	s.audit(r, "create", "backup", nil, "Created system backup", map[string]any{"filename": filename})
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	http.ServeFile(w, r, path)
}

func (s *server) stageRestore(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRestoreUploadBytes+(8<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "Could not read the backup upload")
		return
	}
	file, header, err := r.FormFile("backup")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Choose a CalDen backup file")
		return
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > maxRestoreUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "Backup file is too large")
		return
	}

	manifest, err := s.backup.ValidateAndStage(file, maxRestoreUploadBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "CalDen would not stage this restore: "+err.Error())
		return
	}
	s.audit(r, "stage_restore", "backup", nil, "Staged CalDen restore", map[string]any{
		"backup_version": manifest.Version, "created_at": manifest.CreatedAt, "source_name": filepath.Base(header.Filename),
	})
	writeJSON(w, http.StatusAccepted, map[string]any{
		"staged": true, "restart_required": true, "manifest": manifest,
		"message": "Backup validated and staged. Restart CalDen to apply the restore.",
	})
}

func (s *server) cancelRestore(w http.ResponseWriter, r *http.Request) {
	if err := s.backup.CancelPending(); err != nil {
		writeError(w, 500, "Could not cancel the staged restore")
		return
	}
	s.audit(r, "cancel_restore", "backup", nil, "Cancelled staged restore", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) downloadSavedBackup(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	path, err := s.backup.SavedPath(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, 404, "Backup not found")
		} else {
			writeError(w, 400, "Invalid backup")
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if strings.HasSuffix(strings.ToLower(path), ".tar.gz") {
		w.Header().Set("Content-Type", "application/gzip")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(path)))
	http.ServeFile(w, r, path)
}

func (s *server) deleteSavedBackup(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if err := s.backup.DeleteSaved(name); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, 404, "Backup not found")
		} else {
			writeError(w, 400, "Could not delete backup")
		}
		return
	}
	s.audit(r, "delete", "backup", nil, "Deleted saved backup "+filepath.Base(name), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) restartSystem(w http.ResponseWriter, r *http.Request) {
	s.audit(r, "restart", "system", nil, "Restarted CalDen", nil)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"restarting": true,
		"message": "CalDen is restarting.",
	})
	go func() {
		time.Sleep(1200 * time.Millisecond)
		os.Exit(0)
	}()
}
