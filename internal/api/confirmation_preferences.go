package api

import "net/http"

// Only the authenticated family member can set their own confirmation preference.
func (s *server) updateConfirmationPreference(w http.ResponseWriter, r *http.Request) {
    var in struct {
        Enabled *bool `json:"enabled"`
    }
    if decode(r, &in) != nil || in.Enabled == nil {
        writeError(w, http.StatusBadRequest, "Choose whether confirmations are enabled")
        return
    }
    _, err := s.db.Exec(r.Context(),
        "UPDATE users SET confirmation_enabled=$1,updated_at=now() WHERE id=$2",
        *in.Enabled, currentActor(r).ID)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "Could not save confirmation preference")
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"enabled": *in.Enabled})
}
