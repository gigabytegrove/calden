package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}
func withActor(ctx context.Context, a actor) context.Context { return context.WithValue(ctx, actorKey, a) }
func currentActor(r *http.Request) actor { a,_ := r.Context().Value(actorKey).(actor); return a }
func asString(v any) string { s,_ := v.(string); return s }

func decode(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type","application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w,status,map[string]string{"error":message})
}
func cleanText(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s)>max { return s[:max] }
	return s
}
