package reminders

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "net/url"
    "strings"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"
)

type monitaSettings struct {
    ServerURL string `json:"server_url"`
    Token string `json:"token"`
    DefaultChannel string `json:"default_channel"`
}

type dueReminder struct {
    ReminderID uuid.UUID
    Title string
    Location string
    StartsAt time.Time
    Destination *string
}

func Start(ctx context.Context, db *pgxpool.Pool) {
    go func() {
        ticker:=time.NewTicker(30*time.Second)
        defer ticker.Stop()
        run(ctx,db)
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                run(ctx,db)
            }
        }
    }()
}

func run(ctx context.Context, db *pgxpool.Pool) {
    cfg,ok:=loadMonita(ctx,db)
    if !ok { return }

    rows,err:=db.Query(ctx,`
        SELECT r.id,e.title,e.location,e.starts_at,r.destination
        FROM reminders r
        JOIN events e ON e.id=r.event_id
        LEFT JOIN reminder_deliveries d ON d.reminder_id=r.id
        WHERE r.enabled=true
          AND r.kind='system'
          AND r.provider='monita'
          AND e.status='confirmed'
          AND (e.starts_at - make_interval(mins => r.minutes_before)) <= now()
          AND e.starts_at >= now() - interval '1 day'
          AND COALESCE(d.status,'pending') <> 'sent'
          AND COALESCE(d.attempts,0) < 5
        ORDER BY e.starts_at
        LIMIT 50`)
    if err!=nil { log.Printf("reminder worker query: %v",err); return }
    defer rows.Close()

    var due []dueReminder
    for rows.Next() {
        var d dueReminder
        if err:=rows.Scan(&d.ReminderID,&d.Title,&d.Location,&d.StartsAt,&d.Destination); err==nil {
            due=append(due,d)
        }
    }

    for _,d:=range due {
        destination:=cfg.DefaultChannel
        if d.Destination!=nil && strings.TrimSpace(*d.Destination)!="" { destination=strings.TrimSpace(*d.Destination) }
        when:=d.StartsAt.Local().Format("Mon Jan 2 at 3:04 PM")
        message:=when
        if strings.TrimSpace(d.Location)!="" { message+=" · "+strings.TrimSpace(d.Location) }
        err:=sendMonita(cfg,destination,d.Title,message)
        if err!=nil {
            _,_=db.Exec(ctx,`INSERT INTO reminder_deliveries(reminder_id,status,attempts,last_error,updated_at)
                VALUES($1,'failed',1,$2,now())
                ON CONFLICT(reminder_id) DO UPDATE SET status='failed',attempts=reminder_deliveries.attempts+1,last_error=$2,updated_at=now()`,
                d.ReminderID,cleanError(err))
            log.Printf("Monita reminder %s failed: %v",d.ReminderID,err)
            continue
        }
        _,_=db.Exec(ctx,`INSERT INTO reminder_deliveries(reminder_id,status,attempts,last_error,sent_at,updated_at)
            VALUES($1,'sent',1,NULL,now(),now())
            ON CONFLICT(reminder_id) DO UPDATE SET status='sent',attempts=reminder_deliveries.attempts+1,last_error=NULL,sent_at=now(),updated_at=now()`,d.ReminderID)
    }
}

func loadMonita(ctx context.Context,db *pgxpool.Pool)(monitaSettings,bool){
    var raw []byte
    var enabled bool
    if err:=db.QueryRow(ctx,`SELECT config,enabled FROM integrations WHERE kind='monita' ORDER BY created_at LIMIT 1`).Scan(&raw,&enabled); err!=nil || !enabled {
        return monitaSettings{},false
    }
    var cfg monitaSettings
    if json.Unmarshal(raw,&cfg)!=nil || cfg.ServerURL=="" || cfg.Token=="" { return monitaSettings{},false }
    return cfg,true
}

func sendMonita(cfg monitaSettings,channel,title,message string) error {
    endpoint:=strings.TrimRight(cfg.ServerURL,"/")+"/message?token="+url.QueryEscape(cfg.Token)
    payload:=map[string]any{"title":title,"message":message,"priority":5}
    if channel!="" { payload["channel"]=channel }
    raw,_:=json.Marshal(payload)
    req,err:=http.NewRequest(http.MethodPost,endpoint,bytes.NewReader(raw))
    if err!=nil { return err }
    req.Header.Set("Content-Type","application/json")
    req.Header.Set("User-Agent","CalDen/0.1")
    resp,err:=(&http.Client{Timeout:10*time.Second}).Do(req)
    if err!=nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode<200 || resp.StatusCode>=300 { return fmt.Errorf("Monita returned %s",resp.Status) }
    return nil
}

func cleanError(err error) string {
    if err==nil { return "" }
    s:=err.Error()
    if len(s)>500 { s=s[:500] }
    return s
}
