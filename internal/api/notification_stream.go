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

    person := currentActor(r).ID
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

    for {
        ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
        var active bool
        authErr := s.db.QueryRow(ctx, "SELECT active FROM users WHERE id=$1", person).Scan(&active)
        cancel()
        if authErr != nil || !active { return }

        ctx, cancel = context.WithTimeout(r.Context(), 5*time.Second)
        rows, err := s.db.Query(ctx, `SELECT id,event_id,kind,title,message,occurrence_start,created_at
            FROM notifications
            WHERE user_id=$1 AND dismissed_at IS NULL AND read_at IS NULL AND created_at >= $2
            ORDER BY created_at ASC, id ASC LIMIT 200`, person, cursor)
        if err != nil { cancel(); return }
        type notice struct {
            ID uuid.UUID `json:"id"`
            EventID *uuid.UUID `json:"event_id"`
            Kind string `json:"kind"`
            Title string `json:"title"`
            Message string `json:"message"`
            OccurrenceStart *time.Time `json:"occurrence_start"`
            CreatedAt time.Time `json:"created_at"`
        }
        outgoing := make([]notice, 0)
        for rows.Next() {
            var n notice
            if rows.Scan(&n.ID,&n.EventID,&n.Kind,&n.Title,&n.Message,&n.OccurrenceStart,&n.CreatedAt) != nil { break }
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
