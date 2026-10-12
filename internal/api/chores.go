package api

import (
    "errors"
    "net/http"
    "strings"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
)

// Chores are household work items, deliberately separate from appointments
// and bill payments. Disabling the feature never deletes existing chores.
type choreInput struct {
    Title string `json:"title"`
    Notes string `json:"notes"`
    DueDate string `json:"due_date"`
    AssignedTo string `json:"assigned_to"`
}
type choreStatusInput struct {
    Status string `json:"status"`
}

func (s *server) choresEnabled(r *http.Request) (bool, error) {
    var enabled bool
    err := s.db.QueryRow(r.Context(), `SELECT COALESCE((SELECT value='true' FROM app_settings WHERE key='chores_enabled'),false)`).Scan(&enabled)
    return enabled, err
}

func (s *server) requireChores(w http.ResponseWriter,r *http.Request) bool {
    enabled, err := s.choresEnabled(r)
    if err!=nil { writeError(w,500,"Could not load chores settings"); return false }
    if !enabled { writeError(w,403,"Tasks & Chores is disabled by your household administrator"); return false }
    return true
}

func (s *server) saveChoresEnabled(w http.ResponseWriter,r *http.Request) {
    var input struct { Enabled *bool `json:"enabled"` }
    if decode(r,&input)!=nil || input.Enabled==nil { writeError(w,400,"Choose whether Tasks & Chores is enabled"); return }
    value:="false"
    if *input.Enabled {value="true"}
    _,err:=s.db.Exec(r.Context(),`INSERT INTO app_settings(key,value) VALUES('chores_enabled',$1)
      ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=now()`,value)
    if err!=nil {writeError(w,500,"Could not save Tasks & Chores setting");return}
    s.audit(r,"update","settings",nil,"Changed Tasks & Chores availability",map[string]any{"enabled":*input.Enabled})
    writeJSON(w,200,map[string]bool{"enabled":*input.Enabled})
}

func validChore(in choreInput) (choreInput,uuid.UUID,string) {
    in.Title=cleanText(strings.TrimSpace(in.Title),200)
    in.Notes=cleanText(strings.TrimSpace(in.Notes),4000)
    if in.Title=="" {return in,uuid.Nil,"Enter a task or chore name"}
    if len(strings.TrimSpace(in.DueDate))!=10 {return in,uuid.Nil,"Choose a due date"}
    day,err:=time.Parse("2006-01-02",in.DueDate)
    if err!=nil || day.Format("2006-01-02")!=in.DueDate {return in,uuid.Nil,"Choose a valid due date"}
    id,err:=uuid.Parse(in.AssignedTo)
    if err!=nil {return in,uuid.Nil,"Assign this task to a household member"}
    return in,id,""
}

func canManageChores(role string) bool {return role=="admin"||role=="planner"}

func (s *server) listChores(w http.ResponseWriter,r *http.Request) {
    if !s.requireChores(w,r){return}
    rows,err:=s.db.Query(r.Context(),`SELECT ch.id,ch.title,ch.notes,to_char(ch.due_date,'YYYY-MM-DD'),
      ch.assigned_to,u.display_name,u.initials,u.avatar_url,ch.created_by,
      ch.status,ch.completed_at,ch.completed_by,ch.created_at,ch.updated_at
      FROM chores ch JOIN users u ON u.id=ch.assigned_to
      ORDER BY CASE WHEN ch.status='done' THEN 1 ELSE 0 END,
        ch.due_date ASC,ch.created_at DESC`)
    if err!=nil {writeError(w,500,"Could not load tasks and chores");return}
    defer rows.Close()
    output:=[]map[string]any{}
    for rows.Next(){
        var id,assignee,creator uuid.UUID
        var title,notes,due,name,initials,status string
        var avatar *string
        var completed *time.Time
        var completedBy *uuid.UUID
        var created,updated time.Time
        if err:=rows.Scan(&id,&title,&notes,&due,&assignee,&name,&initials,&avatar,
           &creator,&status,&completed,&completedBy,&created,&updated);err!=nil{
           writeError(w,500,"Could not read tasks and chores");return
        }
        output=append(output,map[string]any{
            "id":id,"title":title,"notes":notes,"due_date":due,
            "assigned_to":assignee,"assignee_name":name,"assignee_initials":initials,"assignee_avatar_url":avatar,
            "created_by":creator,"status":status,"completed_at":completed,
            "completed_by":completedBy,"created_at":created,"updated_at":updated,
        })
    }
    if rows.Err()!=nil{writeError(w,500,"Could not finish loading tasks and chores");return}
    writeJSON(w,200,map[string]any{"items":output})
}

func (s *server) createChore(w http.ResponseWriter,r *http.Request) {
    if !s.requireChores(w,r){return}
    if !canManageChores(currentActor(r).Role){writeError(w,403,"Only administrators and planners can assign chores");return}
    var input choreInput
    if decode(r,&input)!=nil{writeError(w,400,"Check the task details");return}
    input,assigned,msg:=validChore(input)
    if msg!="" {writeError(w,400,msg);return}
    var id uuid.UUID
    err:=s.db.QueryRow(r.Context(),`INSERT INTO chores(title,notes,due_date,assigned_to,created_by)
      SELECT $1::text,$2::text,$3::date,u.id,$5::uuid FROM users u
      WHERE u.id=$4::uuid AND u.active=true RETURNING id`,
      input.Title,input.Notes,input.DueDate,assigned,currentActor(r).ID).Scan(&id)
    if errors.Is(err,pgx.ErrNoRows){writeError(w,400,"Choose an active household member");return}
    if err!=nil{writeError(w,500,"Could not create chore");return}
    s.audit(r,"create","chore",&id,"Created household chore",map[string]any{"assigned_to":assigned})
    writeJSON(w,201,map[string]any{"id":id})
}

func (s *server) updateChore(w http.ResponseWriter,r *http.Request) {
    if !s.requireChores(w,r){return}
    if !canManageChores(currentActor(r).Role){writeError(w,403,"Only administrators and planners can edit chores");return}
    id,err:=uuid.Parse(r.PathValue("id"))
    if err!=nil{writeError(w,400,"Invalid chore");return}
    var input choreInput
    if decode(r,&input)!=nil{writeError(w,400,"Check the task details");return}
    input,assigned,msg:=validChore(input)
    if msg!=""{writeError(w,400,msg);return}
    tag,err:=s.db.Exec(r.Context(),`UPDATE chores SET
       title=$2::text,notes=$3::text,due_date=$4::date,assigned_to=$5::uuid,
       status=CASE WHEN assigned_to IS DISTINCT FROM $5::uuid THEN 'pending' ELSE status END,
       completed_at=CASE WHEN assigned_to IS DISTINCT FROM $5::uuid THEN NULL ELSE completed_at END,
       completed_by=CASE WHEN assigned_to IS DISTINCT FROM $5::uuid THEN NULL ELSE completed_by END,
       updated_at=now()
       WHERE id=$1::uuid AND EXISTS(SELECT 1 FROM users WHERE id=$5::uuid AND active=true)`,
       id,input.Title,input.Notes,input.DueDate,assigned)
    if err!=nil{writeError(w,500,"Could not update chore");return}
    if tag.RowsAffected()==0{writeError(w,404,"Chore or active assignee not found");return}
    s.audit(r,"update","chore",&id,"Updated household chore",nil)
    writeJSON(w,200,map[string]any{"id":id})
}

func (s *server) changeChoreStatus(w http.ResponseWriter,r *http.Request) {
    if !s.requireChores(w,r){return}
    id,err:=uuid.Parse(r.PathValue("id"))
    if err!=nil{writeError(w,400,"Invalid chore");return}
    var input choreStatusInput
    if decode(r,&input)!=nil || (input.Status!="pending"&&input.Status!="in_progress"&&input.Status!="done"){
        writeError(w,400,"Choose Pending, In progress, or Done");return
    }
    a:=currentActor(r)
    tag,err:=s.db.Exec(r.Context(),`UPDATE chores SET status=$2::text,
      completed_at=CASE WHEN $2::text='done' THEN COALESCE(completed_at,now()) ELSE NULL END,
      completed_by=CASE WHEN $2::text='done' THEN $3::uuid ELSE NULL END,updated_at=now()
      WHERE id=$1::uuid AND (assigned_to=$3::uuid OR $4::bool)`,
      id,input.Status,a.ID,canManageChores(a.Role))
    if err!=nil {writeError(w,500,"Could not update chore status");return}
    if tag.RowsAffected()==0{writeError(w,403,"Only the assigned person or a household planner can update this chore");return}
    s.audit(r,"update","chore",&id,"Changed chore status",map[string]any{"status":input.Status})
    writeJSON(w,200,map[string]any{"id":id,"status":input.Status})
}

func (s *server) deleteChore(w http.ResponseWriter,r *http.Request) {
    if !s.requireChores(w,r){return}
    if !canManageChores(currentActor(r).Role){writeError(w,403,"Only administrators and planners can delete chores");return}
    id,err:=uuid.Parse(r.PathValue("id"))
    if err!=nil{writeError(w,400,"Invalid chore");return}
    tag,err:=s.db.Exec(r.Context(),`DELETE FROM chores WHERE id=$1::uuid`,id)
    if err!=nil{writeError(w,500,"Could not delete chore");return}
    if tag.RowsAffected()==0{writeError(w,404,"Chore not found");return}
    s.audit(r,"delete","chore",&id,"Deleted household chore",nil)
    w.WriteHeader(http.StatusNoContent)
}
