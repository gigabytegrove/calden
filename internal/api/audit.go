package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

func (s *server) audit(r *http.Request, action, entityType string, entityID *uuid.UUID, summary string, metadata map[string]any) {
	if strings.TrimSpace(action) == "" || strings.TrimSpace(entityType) == "" {
		return
	}
	raw := []byte("{}")
	if metadata != nil {
		if encoded, err := json.Marshal(metadata); err == nil {
			raw = encoded
		}
	}
	actor := currentActor(r)
	_, _ = s.db.Exec(r.Context(), `INSERT INTO audit_log(actor_user_id,action,entity_type,entity_id,summary,metadata)
		VALUES($1,$2,$3,$4,$5,$6)`,
		actor.ID, cleanText(action, 80), cleanText(entityType, 80), entityID, cleanText(summary, 500), raw)
}

func (s *server) listAudit(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			if parsed < 1 {
				parsed = 1
			}
			if parsed > 500 {
				parsed = 500
			}
			limit = parsed
		}
	}

	entityType := cleanText(strings.TrimSpace(r.URL.Query().Get("entity_type")), 80)
	var rows interface {
		Next() bool
		Scan(...any) error
		Close()
		Err() error
	}
	var err error

	if entityType != "" {
		rows, err = s.db.Query(r.Context(), `SELECT a.id,a.action,a.entity_type,a.entity_id,a.summary,a.metadata,a.created_at,
			u.id,u.display_name,u.initials
			FROM audit_log a LEFT JOIN users u ON u.id=a.actor_user_id
			WHERE a.entity_type=$1
			ORDER BY a.created_at DESC LIMIT $2`, entityType, limit)
	} else {
		rows, err = s.db.Query(r.Context(), `SELECT a.id,a.action,a.entity_type,a.entity_id,a.summary,a.metadata,a.created_at,
			u.id,u.display_name,u.initials
			FROM audit_log a LEFT JOIN users u ON u.id=a.actor_user_id
			ORDER BY a.created_at DESC LIMIT $1`, limit)
	}
	if err != nil {
		writeError(w, 500, "Could not load activity history")
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var action, typ, summary string
		var entityID *uuid.UUID
		var metadata []byte
		var createdAt any
		var actorID *uuid.UUID
		var actorName, actorInitials *string
		if rows.Scan(&id, &action, &typ, &entityID, &summary, &metadata, &createdAt, &actorID, &actorName, &actorInitials) != nil {
			continue
		}
		var decoded map[string]any
		_ = json.Unmarshal(metadata, &decoded)
		out = append(out, map[string]any{
			"id": id, "action": action, "entity_type": typ, "entity_id": entityID,
			"summary": summary, "metadata": decoded, "created_at": createdAt,
			"actor": map[string]any{"id": actorID, "display_name": actorName, "initials": actorInitials},
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, "Could not finish loading activity history")
		return
	}
	writeJSON(w, http.StatusOK, out)
}
