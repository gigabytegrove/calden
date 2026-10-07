package api

import (
	"net/http"
	"strings"

	"github.com/gigabytegrove/calden/internal/updater"
)

func (s *server) updateCheck(w http.ResponseWriter, r *http.Request) {
	result, err := s.updater.Check(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "CalDen could not check for updates: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) updateStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.updater.Status())
}

func (s *server) updatePreferences(w http.ResponseWriter, r *http.Request) {
	prefs, err := s.updater.Preferences()
	if err != nil {
		writeError(w, 500, "Could not load update preferences")
		return
	}
	writeJSON(w, http.StatusOK, prefs)
}

func (s *server) saveUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	var in updater.Preferences
	if decode(r, &in) != nil {
		writeError(w, 400, "Check the update preferences")
		return
	}
	saved, err := s.updater.SavePreferences(in)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	s.audit(r, "preferences", "update", nil, "Changed update preferences", map[string]any{
		"channel": saved.Channel, "auto_check": saved.AutoCheck,
	})
	writeJSON(w, http.StatusOK, saved)
}

func (s *server) installUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Version string `json:"version"`
	}
	if decode(r, &in) != nil || strings.TrimSpace(in.Version) == "" {
		writeError(w, 400, "Choose a release to install")
		return
	}
	status, err := s.updater.Install(r.Context(), in.Version)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	s.audit(r, "install", "update", nil, "Started CalDen update "+strings.TrimSpace(in.Version), map[string]any{"version": strings.TrimSpace(in.Version)})
	writeJSON(w, http.StatusAccepted, status)
}

func (s *server) rollbackUpdate(w http.ResponseWriter, r *http.Request) {
	status, err := s.updater.Rollback()
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	s.audit(r, "rollback", "update", nil, "Started CalDen rollback", map[string]any{"version": status.Version})
	writeJSON(w, http.StatusAccepted, status)
}
