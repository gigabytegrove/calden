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

	"github.com/gigabytegrove/calden/internal/recurrence"
)

type monitaSettings struct {
	ServerURL      string `json:"server_url"`
	Token          string `json:"token"`
	DefaultChannel string `json:"default_channel"`
}

type reminderSource struct {
	ReminderID     uuid.UUID
	EventID        uuid.UUID
	MinutesBefore  int
	Title          string
	Location       string
	SeriesStart    time.Time
	SeriesEnd      time.Time
	Destination    *string
	Rule           *recurrence.Rule
}

type dueReminder struct {
	ReminderID      uuid.UUID
	Title           string
	Location        string
	StartsAt        time.Time
	OccurrenceStart time.Time
	Destination     *string
}

func Start(ctx context.Context, db *pgxpool.Pool) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		run(ctx, db)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run(ctx, db)
			}
		}
	}()
}

func run(ctx context.Context, db *pgxpool.Pool) {
	cfg, ok := loadMonita(ctx, db)
	if !ok {
		return
	}

	sources, err := loadReminderSources(ctx, db)
	if err != nil {
		log.Printf("reminder worker query: %v", err)
		return
	}
	now := time.Now()
	due := []dueReminder{}
	for _, source := range sources {
		offset := time.Duration(source.MinutesBefore) * time.Minute
		duration := source.SeriesEnd.Sub(source.SeriesStart)
		windowFrom := now.Add(-24*time.Hour - duration)
		windowTo := now.Add(offset + duration + 2*time.Minute)
		occurrences := recurrence.Expand(source.SeriesStart, source.SeriesEnd, source.Rule, windowFrom, windowTo, 500)
		for _, occurrence := range occurrences {
			if occurrence.Start.Before(now.Add(-24 * time.Hour)) {
				continue
			}
			if occurrence.Start.Add(-offset).After(now) {
				continue
			}
			if source.Rule != nil && occurrenceExcepted(ctx, db, source.EventID, occurrence.Start) {
				continue
			}
			if alreadyDelivered(ctx, db, source.ReminderID, occurrence.Start) {
				continue
			}
			due = append(due, dueReminder{
				ReminderID: source.ReminderID, Title: source.Title, Location: source.Location,
				StartsAt: occurrence.Start, OccurrenceStart: occurrence.Start, Destination: source.Destination,
			})
		}
	}

	for _, d := range due {
		destination := cfg.DefaultChannel
		if d.Destination != nil && strings.TrimSpace(*d.Destination) != "" {
			destination = strings.TrimSpace(*d.Destination)
		}
		when := d.StartsAt.Local().Format("Mon Jan 2 at 3:04 PM")
		message := when
		if strings.TrimSpace(d.Location) != "" {
			message += " · " + strings.TrimSpace(d.Location)
		}
		err := sendMonita(cfg, destination, d.Title, message)
		if err != nil {
			_, _ = db.Exec(ctx, `INSERT INTO reminder_deliveries(reminder_id,occurrence_start,status,attempts,last_error,updated_at)
				VALUES($1,$2,'failed',1,$3,now())
				ON CONFLICT(reminder_id,occurrence_start) DO UPDATE
				SET status='failed',attempts=reminder_deliveries.attempts+1,last_error=$3,updated_at=now()`,
				d.ReminderID, d.OccurrenceStart, cleanError(err))
			log.Printf("Monita reminder %s occurrence %s failed: %v", d.ReminderID, d.OccurrenceStart.Format(time.RFC3339), err)
			continue
		}
		_, _ = db.Exec(ctx, `INSERT INTO reminder_deliveries(reminder_id,occurrence_start,status,attempts,last_error,sent_at,updated_at)
			VALUES($1,$2,'sent',1,NULL,now(),now())
			ON CONFLICT(reminder_id,occurrence_start) DO UPDATE
			SET status='sent',attempts=reminder_deliveries.attempts+1,last_error=NULL,sent_at=now(),updated_at=now()`,
			d.ReminderID, d.OccurrenceStart)
	}
}

func loadReminderSources(ctx context.Context, db *pgxpool.Pool) ([]reminderSource, error) {
	rows, err := db.Query(ctx, `
		SELECT r.id,e.id,r.minutes_before,e.title,e.location,e.starts_at,e.ends_at,r.destination,
		       er.frequency,er.interval_value,er.weekdays,er.until_at,er.occurrence_count
		FROM reminders r
		JOIN events e ON e.id=r.event_id
		LEFT JOIN event_recurrence er ON er.event_id=e.id
		WHERE r.enabled=true
		  AND r.kind='system'
		  AND r.provider='monita'
		  AND e.status='confirmed'
		  AND (er.event_id IS NOT NULL OR e.starts_at >= now() - interval '1 day')
		  AND (er.until_at IS NULL OR er.until_at >= now() - interval '1 day')
		ORDER BY e.starts_at
		LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []reminderSource{}
	for rows.Next() {
		var source reminderSource
		var frequency *string
		var interval *int
		var weekdaysRaw []byte
		var until *time.Time
		var count *int
		if err := rows.Scan(
			&source.ReminderID, &source.EventID, &source.MinutesBefore, &source.Title, &source.Location,
			&source.SeriesStart, &source.SeriesEnd, &source.Destination,
			&frequency, &interval, &weekdaysRaw, &until, &count,
		); err != nil {
			continue
		}
		if frequency != nil && interval != nil {
			weekdays := []int{}
			if len(weekdaysRaw) > 0 {
				_ = json.Unmarshal(weekdaysRaw, &weekdays)
			}
			source.Rule = recurrence.Normalize(&recurrence.Rule{
				Frequency: *frequency, Interval: *interval, Weekdays: weekdays,
				Until: until, OccurrenceCount: count,
			}, source.SeriesStart)
		}
		out = append(out, source)
	}
	return out, rows.Err()
}

func occurrenceExcepted(ctx context.Context, db *pgxpool.Pool, eventID uuid.UUID, occurrenceStart time.Time) bool {
	var exists bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM event_occurrence_exceptions
		WHERE event_id=$1 AND original_start=$2
	)`, eventID, occurrenceStart).Scan(&exists); err != nil {
		return false
	}
	return exists
}

func alreadyDelivered(ctx context.Context, db *pgxpool.Pool, reminderID uuid.UUID, occurrenceStart time.Time) bool {
	var status string
	var attempts int
	err := db.QueryRow(ctx, `SELECT status,attempts FROM reminder_deliveries
		WHERE reminder_id=$1 AND occurrence_start=$2`, reminderID, occurrenceStart).Scan(&status, &attempts)
	if err != nil {
		return false
	}
	return status == "sent" || attempts >= 5
}

func loadMonita(ctx context.Context, db *pgxpool.Pool) (monitaSettings, bool) {
	var raw []byte
	var enabled bool
	if err := db.QueryRow(ctx, `SELECT config,enabled FROM integrations WHERE kind='monita' ORDER BY created_at LIMIT 1`).Scan(&raw, &enabled); err != nil || !enabled {
		return monitaSettings{}, false
	}
	var cfg monitaSettings
	if json.Unmarshal(raw, &cfg) != nil || cfg.ServerURL == "" || cfg.Token == "" {
		return monitaSettings{}, false
	}
	return cfg, true
}

func sendMonita(cfg monitaSettings, channel, title, message string) error {
	endpoint := strings.TrimRight(cfg.ServerURL, "/") + "/message?token=" + url.QueryEscape(cfg.Token)
	payload := map[string]any{"title": title, "message": message, "priority": 5}
	if channel != "" {
		payload["channel"] = channel
	}
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "CalDen")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Monita returned %s", resp.Status)
	}
	return nil
}

func cleanError(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}
