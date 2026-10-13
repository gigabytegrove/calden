package api

import (
	"net/http"
	"strings"
	"time"
)

type generalSettingsPayload struct {
	HouseholdName string `json:"household_name"`
	Timezone      string `json:"timezone"`
	WeekStart     string `json:"week_start"`
	DefaultView   int    `json:"default_view"`
	ChoresEnabled bool `json:"chores_enabled"`
    BillsEnabled bool `json:"bills_enabled"`
    AgendaEnabled bool `json:"agenda_enabled"`
}

func (s *server) generalSettings(w http.ResponseWriter, r *http.Request) {
	out := generalSettingsPayload{
        BillsEnabled: true, AgendaEnabled: true,
		HouseholdName: "My Family",
		Timezone:      "UTC",
		WeekStart:     "sunday",
		DefaultView:   7,
	}
	rows, err := s.db.Query(r.Context(), `SELECT key,value FROM app_settings WHERE key IN ('household_name','timezone','week_start','default_view','chores_enabled','bills_enabled','agenda_enabled')`)
	if err != nil {
		writeError(w, 500, "Could not load family settings")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if rows.Scan(&key, &value) != nil {
			continue
		}
		switch key {
		case "chores_enabled":
            out.ChoresEnabled = value == "true"
        case "bills_enabled":
            out.BillsEnabled = value != "false"
        case "agenda_enabled":
            out.AgendaEnabled = value != "false"
		case "household_name":
			out.HouseholdName = value
		case "timezone":
			out.Timezone = value
		case "week_start":
			out.WeekStart = value
		case "default_view":
			switch value {
			case "1":
				out.DefaultView = 1
			case "14":
				out.DefaultView = 14
			case "30":
				out.DefaultView = 30
			default:
				out.DefaultView = 7
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) saveGeneralSettings(w http.ResponseWriter, r *http.Request) {
	var in generalSettingsPayload
	if decode(r, &in) != nil {
		writeError(w, 400, "Check the household settings")
		return
	}
	in.HouseholdName = cleanText(strings.TrimSpace(in.HouseholdName), 100)
	in.Timezone = strings.TrimSpace(in.Timezone)
	in.WeekStart = strings.ToLower(strings.TrimSpace(in.WeekStart))
	if in.HouseholdName == "" {
		writeError(w, 400, "Family calendar name is required")
		return
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		writeError(w, 400, "Choose a valid time zone")
		return
	}
	if in.WeekStart != "sunday" && in.WeekStart != "monday" {
		writeError(w, 400, "Week must start on Sunday or Monday")
		return
	}
	if in.DefaultView != 1 && in.DefaultView != 7 && in.DefaultView != 14 && in.DefaultView != 30 {
		writeError(w, 400, "Choose a valid default calendar view")
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not save household settings")
		return
	}
	defer tx.Rollback(r.Context())
	values := map[string]string{
		"household_name": in.HouseholdName,
		"timezone":       in.Timezone,
		"week_start":     in.WeekStart,
		"default_view":   string(rune('0' + in.DefaultView)),
	}
	if in.DefaultView == 14 {
		values["default_view"] = "14"
	} else if in.DefaultView == 30 {
		values["default_view"] = "30"
	}
	for key, value := range values {
		if _, err = tx.Exec(r.Context(), `INSERT INTO app_settings(key,value) VALUES($1,$2)
			ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=now()`, key, value); err != nil {
			writeError(w, 500, "Could not save household settings")
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not save household settings")
		return
	}
	s.audit(r, "update", "settings", nil, "Updated household settings", map[string]any{
		"household_name": in.HouseholdName, "timezone": in.Timezone,
		"week_start": in.WeekStart, "default_view": in.DefaultView,
	})
	enabled, err := s.choresEnabled(r)
    if err != nil {writeError(w, 500, "Could not load Tasks & Chores setting");return}
    in.ChoresEnabled = enabled
    in.BillsEnabled = true
    in.AgendaEnabled = true
    _ = s.db.QueryRow(r.Context(), "SELECT COALESCE((SELECT value='true' FROM app_settings WHERE key='bills_enabled'),true),COALESCE((SELECT value='true' FROM app_settings WHERE key='agenda_enabled'),true)").Scan(&in.BillsEnabled,&in.AgendaEnabled)
	writeJSON(w, http.StatusOK, in)
}


// savePersonalWeekStart stores the authenticated user's cross-device override.
func (s *server) savePersonalWeekStart(w http.ResponseWriter, r *http.Request) {
    var in struct { WeekStart string `json:"week_start"` }
    if decode(r, &in) != nil {
        writeError(w, http.StatusBadRequest, "Choose a week start")
        return
    }
    in.WeekStart = strings.ToLower(strings.TrimSpace(in.WeekStart))
    if in.WeekStart != "default" && in.WeekStart != "sunday" && in.WeekStart != "monday" {
        writeError(w, http.StatusBadRequest, "Choose household default, Sunday, or Monday")
        return
    }
    _, err := s.db.Exec(r.Context(), "UPDATE users SET week_start=NULLIF($1,'default'),updated_at=now() WHERE id=$2", in.WeekStart, currentActor(r).ID)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "Could not save your week start")
        return
    }
    s.audit(r, "update", "settings", nil, "Updated personal week start", map[string]any{"week_start":in.WeekStart})
    writeJSON(w, http.StatusOK, map[string]string{"week_start":in.WeekStart})
}


// Feature switches are visibility controls. Existing bill events, payment
// records, agenda data, and reminders are never modified by this endpoint.
func (s *server) saveFeatureVisibility(w http.ResponseWriter, r *http.Request) {
    var in struct {
        BillsEnabled *bool `json:"bills_enabled"`
        AgendaEnabled *bool `json:"agenda_enabled"`
    }
    if decode(r,&in)!=nil || (in.BillsEnabled==nil && in.AgendaEnabled==nil) {
        writeError(w,400,"Choose Bills or Agenda visibility");return
    }
    tx,err:=s.db.Begin(r.Context())
    if err!=nil{writeError(w,500,"Could not save household features");return}
    defer tx.Rollback(r.Context())
    for _,change:=range []struct{Key string; Enabled *bool}{
        {"bills_enabled",in.BillsEnabled},{"agenda_enabled",in.AgendaEnabled},
    } {
        if change.Enabled==nil{continue}
        value:="false";if *change.Enabled{value="true"}
        if _,err=tx.Exec(r.Context(),
            "INSERT INTO app_settings(key,value) VALUES($1,$2) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=now()",
            change.Key,value);err!=nil{writeError(w,500,"Could not update household features");return}
    }
    if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save household features");return}
    s.audit(r,"update","settings",nil,"Changed household feature visibility",nil)
    s.generalSettings(w,r)
}
