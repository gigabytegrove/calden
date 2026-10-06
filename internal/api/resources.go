package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *server) me(w http.ResponseWriter,r *http.Request) {
	a:=currentActor(r)
	var out struct {
		ID uuid.UUID `json:"id"`
		Username string `json:"username"`
		DisplayName string `json:"display_name"`
		Role string `json:"role"`
		Initials string `json:"initials"`
		AvatarURL *string `json:"avatar_url"`
	}
	if err:=s.db.QueryRow(r.Context(),`SELECT id,username,display_name,role,initials,avatar_url FROM users WHERE id=$1`,a.ID).
		Scan(&out.ID,&out.Username,&out.DisplayName,&out.Role,&out.Initials,&out.AvatarURL); err!=nil {
		writeError(w,500,"Could not load your account"); return
	}
	writeJSON(w,200,out)
}

func (s *server) listUsers(w http.ResponseWriter,r *http.Request) {
	rows,err:=s.db.Query(r.Context(),`SELECT id,username,display_name,role,initials,avatar_url FROM users WHERE active=true ORDER BY display_name`)
	if err!=nil { writeError(w,500,"Could not load people"); return }
	defer rows.Close()
	out:=[]map[string]any{}
	for rows.Next() {
		var id uuid.UUID; var username,name,role,ini string; var avatar *string
		if rows.Scan(&id,&username,&name,&role,&ini,&avatar)==nil {
			out=append(out,map[string]any{"id":id,"username":username,"display_name":name,"role":role,"initials":ini,"avatar_url":avatar})
		}
	}
	writeJSON(w,200,out)
}

func (s *server) createUser(w http.ResponseWriter,r *http.Request) {
	var in struct { Username,DisplayName,Password,Role string }
	if decode(r,&in)!=nil { writeError(w,400,"Check the account details"); return }
	if err:=validateLogin(in.Username,in.Password); err!=nil { writeError(w,400,err.Error()); return }
	in.Role=strings.ToLower(strings.TrimSpace(in.Role)); if in.Role=="" { in.Role="member" }
	if in.Role!="member" && in.Role!="restricted" && in.Role!="admin" { writeError(w,400,"Choose a valid account type"); return }
	hash,err:=hashPassword(in.Password); if err!=nil { writeError(w,500,"Could not create account"); return }
	var id uuid.UUID
	err=s.db.QueryRow(r.Context(),`INSERT INTO users(username,display_name,password_hash,role,initials) VALUES(lower($1),$2,$3,$4,$5) RETURNING id`,
		in.Username,cleanText(in.DisplayName,100),hash,in.Role,initials(in.DisplayName)).Scan(&id)
	if err!=nil { writeError(w,409,"That username is already in use"); return }
	writeJSON(w,201,map[string]any{"id":id})
}

func hashPassword(password string)(string,error) { return bcryptHash(password) }

func (s *server) listCalendars(w http.ResponseWriter,r *http.Request) {
	a:=currentActor(r)
	rows,err:=s.db.Query(r.Context(),`SELECT c.id,c.name,c.color,c.icon,c.description,
		COALESCE(p.can_edit,false),COALESCE(p.can_delete,false)
		FROM calendars c
		LEFT JOIN calendar_permissions p ON p.calendar_id=c.id AND p.user_id=$1
		WHERE $2='admin' OR COALESCE(p.can_view,false)=true ORDER BY c.name`,a.ID,a.Role)
	if err!=nil { writeError(w,500,"Could not load calendars"); return }
	defer rows.Close()
	out:=[]map[string]any{}
	for rows.Next() {
		var id uuid.UUID; var name,color,icon,description string; var canEdit,canDelete bool
		if rows.Scan(&id,&name,&color,&icon,&description,&canEdit,&canDelete)==nil {
			if a.Role=="admin" { canEdit=true; canDelete=true }
			out=append(out,map[string]any{"id":id,"name":name,"color":color,"icon":icon,"description":description,"can_edit":canEdit,"can_delete":canDelete})
		}
	}
	writeJSON(w,200,out)
}

func (s *server) createCalendar(w http.ResponseWriter,r *http.Request) {
	var in struct { Name,Color,Icon,Description string; VisibleTo,EditableBy []uuid.UUID `json:"visible_to"` }
	// custom decode below because EditableBy tag differs
	var raw struct {
		Name string `json:"name"`; Color string `json:"color"`; Icon string `json:"icon"`; Description string `json:"description"`
		VisibleTo []uuid.UUID `json:"visible_to"`; EditableBy []uuid.UUID `json:"editable_by"`
	}
	if decode(r,&raw)!=nil { writeError(w,400,"Check the calendar details"); return }
	if strings.TrimSpace(raw.Name)=="" || !validColor(raw.Color) { writeError(w,400,"Calendar name and color are required"); return }
	if raw.Icon=="" { raw.Icon="calendar" }
	tx,err:=s.db.Begin(r.Context()); if err!=nil { writeError(w,500,"Could not create calendar"); return }
	defer tx.Rollback(r.Context())
	var id uuid.UUID
	if err=tx.QueryRow(r.Context(),`INSERT INTO calendars(name,color,icon,description,created_by) VALUES($1,$2,$3,$4,$5) RETURNING id`,
		cleanText(raw.Name,100),raw.Color,cleanText(raw.Icon,40),cleanText(raw.Description,500),currentActor(r).ID).Scan(&id); err!=nil {
		writeError(w,500,"Could not create calendar"); return
	}
	edit:=map[uuid.UUID]bool{}; for _,u:=range raw.EditableBy { edit[u]=true }
	seen:=map[uuid.UUID]bool{}
	for _,u:=range append(raw.VisibleTo,raw.EditableBy...) {
		if seen[u] { continue }; seen[u]=true
		_,err=tx.Exec(r.Context(),`INSERT INTO calendar_permissions(calendar_id,user_id,can_view,can_edit,can_delete) VALUES($1,$2,true,$3,$3)`,id,u,edit[u])
		if err!=nil { writeError(w,400,"One of the selected people is invalid"); return }
	}
	if err=tx.Commit(r.Context()); err!=nil { writeError(w,500,"Could not save calendar"); return }
	writeJSON(w,201,map[string]any{"id":id})
}

func (s *server) setCalendarPermissions(w http.ResponseWriter,r *http.Request) {
	id,err:=uuid.Parse(r.PathValue("id")); if err!=nil { writeError(w,400,"Invalid calendar"); return }
	var raw struct { VisibleTo []uuid.UUID `json:"visible_to"`; EditableBy []uuid.UUID `json:"editable_by"` }
	if decode(r,&raw)!=nil { writeError(w,400,"Check the people selected"); return }
	tx,err:=s.db.Begin(r.Context()); if err!=nil { writeError(w,500,"Could not update access"); return }
	defer tx.Rollback(r.Context())
	if _,err=tx.Exec(r.Context(),"DELETE FROM calendar_permissions WHERE calendar_id=$1",id); err!=nil { writeError(w,500,"Could not update access"); return }
	edit:=map[uuid.UUID]bool{}; for _,u:=range raw.EditableBy { edit[u]=true }
	seen:=map[uuid.UUID]bool{}
	for _,u:=range append(raw.VisibleTo,raw.EditableBy...) {
		if seen[u] {continue}; seen[u]=true
		if _,err=tx.Exec(r.Context(),`INSERT INTO calendar_permissions(calendar_id,user_id,can_view,can_edit,can_delete) VALUES($1,$2,true,$3,$3)`,id,u,edit[u]); err!=nil {
			writeError(w,400,"One of the selected people is invalid"); return
		}
	}
	if err=tx.Commit(r.Context()); err!=nil { writeError(w,500,"Could not update access"); return }
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listEvents(w http.ResponseWriter,r *http.Request) {
	a:=currentActor(r)
	from:=time.Now().AddDate(0,-1,0); to:=time.Now().AddDate(0,3,0)
	if v:=r.URL.Query().Get("from"); v!="" { if t,e:=time.Parse(time.RFC3339,v); e==nil {from=t} }
	if v:=r.URL.Query().Get("to"); v!="" { if t,e:=time.Parse(time.RFC3339,v); e==nil {to=t} }
	rows,err:=s.db.Query(r.Context(),`SELECT e.id,e.calendar_id,e.title,e.notes,e.location,e.starts_at,e.ends_at,e.all_day,e.status,
		c.name,c.color FROM events e JOIN calendars c ON c.id=e.calendar_id
		LEFT JOIN calendar_permissions p ON p.calendar_id=c.id AND p.user_id=$1
		WHERE ($2='admin' OR COALESCE(p.can_view,false)=true) AND e.starts_at < $4 AND e.ends_at >= $3
		ORDER BY e.starts_at`,a.ID,a.Role,from,to)
	if err!=nil { writeError(w,500,"Could not load events"); return }
	defer rows.Close()
	out:=[]map[string]any{}
	for rows.Next() {
		var id,calID uuid.UUID; var title,notes,location,status,calName,color string; var start,end time.Time; var allDay bool
		if rows.Scan(&id,&calID,&title,&notes,&location,&start,&end,&allDay,&status,&calName,&color)!=nil {continue}
		assignees:=[]map[string]any{}
		arows,_:=s.db.Query(r.Context(),`SELECT u.id,u.display_name,u.initials,u.avatar_url FROM event_assignees ea JOIN users u ON u.id=ea.user_id WHERE ea.event_id=$1 ORDER BY u.display_name`,id)
		if arows!=nil {
			for arows.Next(){ var uid uuid.UUID; var n,ini string; var avatar *string; if arows.Scan(&uid,&n,&ini,&avatar)==nil {assignees=append(assignees,map[string]any{"id":uid,"display_name":n,"initials":ini,"avatar_url":avatar})} }
			arows.Close()
		}
		out=append(out,map[string]any{"id":id,"calendar_id":calID,"title":title,"notes":notes,"location":location,"starts_at":start,"ends_at":end,"all_day":allDay,"status":status,"calendar_name":calName,"color":color,"assignees":assignees})
	}
	writeJSON(w,200,out)
}

func (s *server) createEvent(w http.ResponseWriter,r *http.Request) {
	var in struct {
		CalendarID uuid.UUID `json:"calendar_id"`; Title string `json:"title"`; Notes string `json:"notes"`; Location string `json:"location"`
		StartsAt time.Time `json:"starts_at"`; EndsAt time.Time `json:"ends_at"`; AllDay bool `json:"all_day"`
		AssigneeIDs []uuid.UUID `json:"assignee_ids"`
		Reminders []struct { Kind string `json:"kind"`; Provider string `json:"provider"`; MinutesBefore int `json:"minutes_before"`; Destination string `json:"destination"` } `json:"reminders"`
	}
	if decode(r,&in)!=nil || strings.TrimSpace(in.Title)=="" || in.CalendarID==uuid.Nil || in.StartsAt.IsZero() || in.EndsAt.Before(in.StartsAt) {
		writeError(w,400,"Check the event details"); return
	}
	a:=currentActor(r)
	var allowed bool
	if a.Role=="admin" { allowed=true } else { _=s.db.QueryRow(r.Context(),`SELECT COALESCE(can_edit,false) FROM calendar_permissions WHERE calendar_id=$1 AND user_id=$2`,in.CalendarID,a.ID).Scan(&allowed) }
	if !allowed { writeError(w,403,"You cannot add events to this calendar"); return }

	tx,err:=s.db.Begin(r.Context()); if err!=nil { writeError(w,500,"Could not create event"); return }; defer tx.Rollback(r.Context())
	var id uuid.UUID
	err=tx.QueryRow(r.Context(),`INSERT INTO events(calendar_id,title,notes,location,starts_at,ends_at,all_day,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		in.CalendarID,cleanText(in.Title,200),cleanText(in.Notes,5000),cleanText(in.Location,500),in.StartsAt,in.EndsAt,in.AllDay,a.ID).Scan(&id)
	if err!=nil { writeError(w,400,"Could not create event"); return }
	for _,uid:=range in.AssigneeIDs {
		if _,err=tx.Exec(r.Context(),`INSERT INTO event_assignees(event_id,user_id) VALUES($1,$2)`,id,uid); err!=nil { writeError(w,400,"One of the selected people is invalid"); return }
	}
	for _,rm:=range in.Reminders {
		if (rm.Kind!="personal"&&rm.Kind!="system") || (rm.Provider!="android"&&rm.Provider!="monita") || rm.MinutesBefore<0 { writeError(w,400,"Check reminder settings"); return }
		if _,err=tx.Exec(r.Context(),`INSERT INTO reminders(event_id,kind,provider,minutes_before,destination) VALUES($1,$2,$3,$4,NULLIF($5,''))`,id,rm.Kind,rm.Provider,rm.MinutesBefore,cleanText(rm.Destination,200)); err!=nil { writeError(w,400,"Could not save reminder"); return }
	}
	if err=tx.Commit(r.Context()); err!=nil { writeError(w,500,"Could not save event"); return }
	writeJSON(w,201,map[string]any{"id":id})
}

func validColor(v string) bool {
	if len(v)!=7 || v[0]!='#' {return false}
	for _,c:=range v[1:] { if !strings.ContainsRune("0123456789abcdefABCDEF",c){return false} }
	return true
}
