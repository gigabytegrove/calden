package api

import "net/http"

func (s *server) generalSettings(w http.ResponseWriter, r *http.Request) {
	out := map[string]string{
		"household_name": "My Family",
		"timezone":       "UTC",
	}
	rows, err := s.db.Query(r.Context(), `SELECT key,value FROM app_settings WHERE key IN ('household_name','timezone')`)
	if err != nil {
		writeError(w, 500, "Could not load family settings")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if rows.Scan(&key, &value) == nil {
			out[key] = value
		}
	}
	writeJSON(w, http.StatusOK, out)
}
