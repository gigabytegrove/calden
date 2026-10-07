package api

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type userUpdatePayload struct {
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Active      *bool  `json:"active"`
	Password    string `json:"password"`
}

func (s *server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid person")
		return
	}
	var in userUpdatePayload
	if decode(r, &in) != nil {
		writeError(w, 400, "Check the account details")
		return
	}
	in.DisplayName = cleanText(strings.TrimSpace(in.DisplayName), 100)
	in.Role = strings.ToLower(strings.TrimSpace(in.Role))
	if in.DisplayName == "" {
		writeError(w, 400, "Name is required")
		return
	}
	if in.Role != "admin" && in.Role != "member" && in.Role != "restricted" {
		writeError(w, 400, "Choose a valid account type")
		return
	}
	if id == currentActor(r).ID && in.Active != nil && !*in.Active {
		writeError(w, 400, "You cannot deactivate your own account")
		return
	}
	if id == currentActor(r).ID && in.Role != "admin" {
		writeError(w, 400, "You cannot remove your own administrator access")
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not update account")
		return
	}
	defer tx.Rollback(r.Context())

	active := true
	if in.Active != nil {
		active = *in.Active
	} else {
		_ = tx.QueryRow(r.Context(), "SELECT active FROM users WHERE id=$1", id).Scan(&active)
	}
	tag, err := tx.Exec(r.Context(), `UPDATE users SET display_name=$2,role=$3,active=$4,initials=$5,updated_at=now() WHERE id=$1`,
		id, in.DisplayName, in.Role, active, initials(in.DisplayName))
	if err != nil {
		writeError(w, 500, "Could not update account")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "Person not found")
		return
	}
	if strings.TrimSpace(in.Password) != "" {
		if len(in.Password) < 8 {
			writeError(w, 400, "Password must be at least 8 characters")
			return
		}
		hash, err := hashPassword(in.Password)
		if err != nil {
			writeError(w, 500, "Could not secure password")
			return
		}
		if _, err = tx.Exec(r.Context(), "UPDATE users SET password_hash=$2,updated_at=now() WHERE id=$1", id, hash); err != nil {
			writeError(w, 500, "Could not update password")
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not save account")
		return
	}
	s.audit(r, "update", "user", &id, "Updated "+in.DisplayName, map[string]any{
		"role": in.Role, "active": active, "password_reset": strings.TrimSpace(in.Password) != "",
	})
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (s *server) deactivateUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid person")
		return
	}
	if id == currentActor(r).ID {
		writeError(w, 400, "You cannot deactivate your own account")
		return
	}
	tag, err := s.db.Exec(r.Context(), "UPDATE users SET active=false,updated_at=now() WHERE id=$1", id)
	if err != nil {
		writeError(w, 500, "Could not deactivate account")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "Person not found")
		return
	}
	s.audit(r, "deactivate", "user", &id, "Deactivated account", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) calendarPermissions(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid calendar")
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT user_id,can_view,can_edit,can_delete FROM calendar_permissions WHERE calendar_id=$1`, id)
	if err != nil {
		writeError(w, 500, "Could not load calendar access")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var userID uuid.UUID
		var view, edit, del bool
		if rows.Scan(&userID, &view, &edit, &del) == nil {
			out = append(out, map[string]any{"user_id": userID, "can_view": view, "can_edit": edit, "can_delete": del})
		}
	}
	writeJSON(w, http.StatusOK, out)
}
