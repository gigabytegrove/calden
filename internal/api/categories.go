package api

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type categoryInput struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Icon        string `json:"icon"`
	Description string `json:"description"`
}

func (s *server) listCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `SELECT id,name,color,icon,description,active
		FROM categories WHERE active=true ORDER BY lower(name)`)
	if err != nil {
		writeError(w, 500, "Could not load categories")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var name, color, icon, description string
		var active bool
		if rows.Scan(&id, &name, &color, &icon, &description, &active) == nil {
			out = append(out, map[string]any{
				"id": id, "name": name, "color": color, "icon": icon,
				"description": description, "active": active,
			})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) createCategory(w http.ResponseWriter, r *http.Request) {
	var in categoryInput
	if decode(r, &in) != nil {
		writeError(w, 400, "Check the category details")
		return
	}
	in.Name = cleanText(strings.TrimSpace(in.Name), 100)
	in.Description = cleanText(in.Description, 500)
	in.Icon = cleanText(strings.TrimSpace(in.Icon), 40)
	if in.Icon == "" {
		in.Icon = "tag"
	}
	if in.Name == "" || !validColor(in.Color) {
		writeError(w, 400, "Category name and color are required")
		return
	}
	var id uuid.UUID
	if err := s.db.QueryRow(r.Context(), `INSERT INTO categories(name,color,icon,description,created_by)
		VALUES($1,$2,$3,$4,$5) RETURNING id`,
		in.Name, in.Color, in.Icon, in.Description, currentActor(r).ID).Scan(&id); err != nil {
		writeError(w, http.StatusConflict, "That category name is already in use")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *server) updateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid category")
		return
	}
	var in categoryInput
	if decode(r, &in) != nil {
		writeError(w, 400, "Check the category details")
		return
	}
	in.Name = cleanText(strings.TrimSpace(in.Name), 100)
	in.Description = cleanText(in.Description, 500)
	in.Icon = cleanText(strings.TrimSpace(in.Icon), 40)
	if in.Icon == "" {
		in.Icon = "tag"
	}
	if in.Name == "" || !validColor(in.Color) {
		writeError(w, 400, "Category name and color are required")
		return
	}
	tag, err := s.db.Exec(r.Context(), `UPDATE categories
		SET name=$2,color=$3,icon=$4,description=$5,updated_at=now()
		WHERE id=$1 AND active=true`, id, in.Name, in.Color, in.Icon, in.Description)
	if err != nil {
		writeError(w, http.StatusConflict, "That category name is already in use")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "Category not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (s *server) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid category")
		return
	}
	tag, err := s.db.Exec(r.Context(), `UPDATE categories SET active=false,updated_at=now()
		WHERE id=$1 AND active=true`, id)
	if err != nil {
		writeError(w, 500, "Could not archive category")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "Category not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
