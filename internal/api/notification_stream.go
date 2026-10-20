package api

import (
    "context"
    "net/http"
    "net/url"
    "strings"
    "time"

    "github.com/google/uuid"
    "github.com/gorilla/websocket"
)

// streamNotifications supplies a live transport over the durable notifications
// table. Clients also retain their REST inbox for catch-up after disconnection.
// Authorization comes from the existing bearer-token middleware.
func (s *server) streamNotifications(w http.ResponseWriter, r *http.Request) {
    if !websocket.IsWebSocketUpgrade(r) {
        writeError(w, http.StatusUpgradeRequired, "WebSocket upgrade required")
        return
    }
    // Android WebSocket clients need not send Origin; browsers must be same-origin.
    origin := r.Header.Get("Origin")
    if origin != "" {
        parsed, err := url.Parse(origin)
        if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") ||
            !strings.EqualFold(parsed.Host, r.Host) || parsed.User != nil ||
            parsed.Path != "" || parsed.RawQuery != "" {
            writeError(w, http.StatusForbidden, "WebSocket origin not permitted")
            return
        }
    }

    session := currentActor(r)
    person := session.ID
    connection, err := (&websocket.Upgrader{
        ReadBufferSize: 1024, WriteBufferSize: 2048,
        Subprotocols: []string{"calden"},
        CheckOrigin: func(*http.Request) bool { return true }, // checked above
    }).Upgrade(w, r, nil)
    if err != nil { return }
    defer connection.Close()
    connection.SetReadLimit(1024)
    _ = connection.SetReadDeadline(time.Now().Add(60 * time.Second))
    connection.SetPongHandler(func(string) error {
        return connection.SetReadDeadline(time.Now().Add(60 * time.Second))
    })
    done := make(chan struct{})
    go func() {
        defer close(done)
        for {
            if _, _, err := connection.ReadMessage(); err != nil { return }
        }
    }()

    // Start with a bounded recent snapshot. A reconnecting client reconciles
    // against GET /api/notifications so no notifications depend on socket uptime.
    cursor := time.Now().Add(-24 * time.Hour)
    seen := make(map[uuid.UUID]struct{}, 100)
    ticker := time.NewTicker(2 * time.Second)
    defer ticker.Stop()
    ping := time.NewTicker(20 * time.Second)
    defer ping.Stop()

    previousRevision := ""
    for {
        if !session.ExpiresAt.IsZero() && !time.Now().Before(session.ExpiresAt) { return }
        ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
        var active,liveChild bool
        var liveRole string
        authErr := s.db.QueryRow(ctx, "SELECT active,role,is_child FROM users WHERE id=$1", person).
            Scan(&active,&liveRole,&liveChild)
        cancel()
        if authErr != nil || !active { return }

        ctx, cancel = context.WithTimeout(r.Context(), 5*time.Second)
        rows, err := s.db.Query(ctx, `SELECT n.id,n.event_id,n.calendar_id,n.kind,n.title,n.message,n.occurrence_start,n.created_at
            FROM notifications n
            LEFT JOIN events e ON e.id=n.event_id
            LEFT JOIN calendars c ON c.id=COALESCE(n.calendar_id,e.calendar_id)
            LEFT JOIN calendar_permissions p ON p.calendar_id=c.id AND p.user_id=$1
            WHERE n.user_id=$1 AND n.dismissed_at IS NULL AND n.read_at IS NULL AND n.created_at >= $2
              AND (c.id IS NULL OR $4 IN ('admin','planner') OR COALESCE(p.can_view,false)=true)
              AND (NOT $3 OR c.id IS NULL OR c.calendar_type<>'bill_pay')
            ORDER BY n.created_at ASC,n.id ASC LIMIT 200`, person, cursor, liveChild,liveRole)
        if err != nil { cancel(); return }
        type notice struct {
            ID uuid.UUID `json:"id"`
            EventID *uuid.UUID `json:"event_id"`
            CalendarID *uuid.UUID `json:"calendar_id"`
            Kind string `json:"kind"`
            Title string `json:"title"`
            Message string `json:"message"`
            OccurrenceStart *time.Time `json:"occurrence_start"`
            CreatedAt time.Time `json:"created_at"`
        }
        outgoing := make([]notice, 0)
        for rows.Next() {
            var n notice
            if rows.Scan(&n.ID,&n.EventID,&n.CalendarID,&n.Kind,&n.Title,&n.Message,&n.OccurrenceStart,&n.CreatedAt) != nil { break }
            if _, found := seen[n.ID]; !found {
                outgoing = append(outgoing, n)
                seen[n.ID] = struct{}{}
            }
            if n.CreatedAt.After(cursor) { cursor = n.CreatedAt }
        }
        queryErr := rows.Err()
        rows.Close()
        cancel()
        if queryErr != nil { return }
        for _, n := range outgoing {
            _ = connection.SetWriteDeadline(time.Now().Add(5*time.Second))
            if err := connection.WriteJSON(n); err != nil { return }
        }
        // Invalidate the app/widget caches even when an event change did not
        // create a notification. The revision contains no private calendar data.
        ctx, cancel = context.WithTimeout(r.Context(), 5*time.Second)
        var revision string
        versionErr := s.db.QueryRow(ctx, `SELECT value FROM app_settings WHERE key='data_revision'`).Scan(&revision)
        cancel()
        if versionErr == nil && revision != previousRevision {
            previousRevision = revision
            _ = connection.SetWriteDeadline(time.Now().Add(5*time.Second))
            if err := connection.WriteJSON(map[string]any{"type":"data_changed","revision":revision}); err != nil { return }
        }
        select {
        case <-done: return
        case <-r.Context().Done(): return
        case <-ticker.C:
        case <-ping.C:
            _ = connection.SetWriteDeadline(time.Now().Add(5*time.Second))
            if err := connection.WriteMessage(websocket.PingMessage, nil); err != nil { return }
        }
    }
}
