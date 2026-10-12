package api

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"github.com/gigabytegrove/calden/internal/secrets"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type monitaSettings struct {
	ServerURL      string `json:"server_url"`
	Token          string `json:"token,omitempty"`
	DefaultChannel string `json:"default_channel,omitempty"`
	ChannelID      uint64 `json:"channel_id,omitempty"`
	Enabled        bool   `json:"enabled"`
}

func (s *server) getMonitaIntegration(w http.ResponseWriter, r *http.Request) {
	var raw []byte
	var enabled bool
	err := s.db.QueryRow(r.Context(), `SELECT config,enabled FROM integrations WHERE kind='monita' ORDER BY created_at LIMIT 1`).Scan(&raw, &enabled)
	if err != nil {
		writeJSON(w, 200, monitaSettings{})
		return
	}
	var cfg monitaSettings
	_ = json.Unmarshal(raw, &cfg)
	cfg.Enabled = enabled
	if cfg.Token != "" {
		cfg.Token = "********"
	}
	writeJSON(w, 200, cfg)
}

func (s *server) saveMonitaIntegration(w http.ResponseWriter, r *http.Request) {
	var in monitaSettings
	if decode(r, &in) != nil {
		writeError(w, 400, "Check the Monita settings")
		return
	}
	in.ServerURL = strings.TrimRight(strings.TrimSpace(in.ServerURL), "/")
	if in.ServerURL == "" {
		writeError(w, 400, "Enter your Monita server address")
		return
	}
	if _, err := url.ParseRequestURI(in.ServerURL); err != nil {
		writeError(w, 400, "Enter a valid Monita server address")
		return
	}

	var existingToken string
	var existingRaw []byte
	_ = s.db.QueryRow(r.Context(), `SELECT config FROM integrations WHERE kind='monita' ORDER BY created_at LIMIT 1`).Scan(&existingRaw)
	if len(existingRaw) > 0 {
		var existing monitaSettings
		_ = json.Unmarshal(existingRaw, &existing)
		existingToken = existing.Token
	}
	if in.Token == "********" || strings.TrimSpace(in.Token) == "" {
		in.Token = existingToken
	}
	if in.Enabled && in.Token == "" {
        writeError(w, 400, "Enter the Monita token CalDen should use")
        return
    }
    // Monita service-account credentials use scoped channel-read/message-write
    // permissions and must select a channel ID. Legacy application tokens remain supported.
    plainToken, err := secrets.Open(s.jwtSecret, in.Token)
    if err != nil { writeError(w, 400, "Stored Monita token could not be read"); return }
    if in.Enabled && strings.HasPrefix(plainToken, "SA.") && in.ChannelID == 0 {
        writeError(w, 400, "Select a Monita channel first"); return
    }
    if plainToken != "" {
        in.Token, err = secrets.Seal(s.jwtSecret, plainToken)
        if err != nil { writeError(w, 500, "Could not secure Monita token"); return }
    }

    raw, _ := json.Marshal(in)
	var id string
	err := s.db.QueryRow(r.Context(), `SELECT id::text FROM integrations WHERE kind='monita' ORDER BY created_at LIMIT 1`).Scan(&id)
	if err == nil {
		_, err = s.db.Exec(r.Context(), `UPDATE integrations SET name='Monita',config=$2,enabled=$3,updated_at=now() WHERE id=$1::uuid`, id, raw, in.Enabled)
	} else {
		_, err = s.db.Exec(r.Context(), `INSERT INTO integrations(kind,name,config,enabled) VALUES('monita','Monita',$1,$2)`, raw, in.Enabled)
	}
	if err != nil {
		writeError(w, 500, "Could not save Monita settings")
		return
	}
	s.audit(r, "update", "integration", nil, "Updated Monita integration", map[string]any{
		"enabled": in.Enabled, "server_url": in.ServerURL, "default_channel": in.DefaultChannel,
	})
	writeJSON(w, 200, map[string]bool{"saved": true})
}

func (s *server) testMonitaIntegration(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadMonitaSettings(r)
	if err != nil || !cfg.Enabled {
		writeError(w, 400, "Turn on the Monita connection first")
		return
	}
	if err = sendMonita(cfg, "CalDen is connected", "This is a test reminder from CalDen."); err != nil {
		writeError(w, 502, "Monita did not accept the test reminder: "+err.Error())
		return
	}
	s.audit(r, "test", "integration", nil, "Sent Monita connection test", nil)
	writeJSON(w, 200, map[string]bool{"sent": true})
}

func (s *server) loadMonitaSettings(r *http.Request) (monitaSettings, error) {
	var raw []byte
	var enabled bool
	if err := s.db.QueryRow(r.Context(), `SELECT config,enabled FROM integrations WHERE kind='monita' ORDER BY created_at LIMIT 1`).Scan(&raw, &enabled); err != nil {
		return monitaSettings{}, err
	}
	var cfg monitaSettings
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	cfg.Enabled = enabled
    cfg.Token, err = secrets.Open(s.jwtSecret, cfg.Token)
	return cfg, err
}

func sendMonita(cfg monitaSettings, title, message string) error {
	if cfg.ServerURL == "" || cfg.Token == "" {
		return fmt.Errorf("connection is incomplete")
	}
	endpoint := strings.TrimRight(cfg.ServerURL, "/") + "/message?token=" + url.QueryEscape(cfg.Token)
    body := map[string]any{"title": title, "message": message, "priority": 5}
    serviceMode := strings.HasPrefix(cfg.Token, "SA.")
    if serviceMode {
        if cfg.ChannelID == 0 { return fmt.Errorf("select a Monita service channel") }
        endpoint = strings.TrimRight(cfg.ServerURL, "/") + "/service/application/" + strconv.FormatUint(cfg.ChannelID, 10) + "/message"
    } else if cfg.DefaultChannel != "" {
        body["channel"] = cfg.DefaultChannel
    }
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
    if serviceMode { req.Header.Set("Authorization", "Bearer "+cfg.Token) }
	req.Header.Set("User-Agent", "CalDen")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	return nil
}

type monitaChannel struct {
    ID uint64 `json:"id"`
    Name string `json:"name"`
    Description string `json:"description"`
}

// List available channels through Monita's scoped service-account API.
// This endpoint does not transmit service tokens to the browser.
func (s *server) getMonitaChannels(w http.ResponseWriter, r *http.Request) {
    cfg, err := s.loadMonitaSettings(r)
    if err != nil || !strings.HasPrefix(cfg.Token, "SA.") {
        writeError(w, 400, "Save a Monita service-account token to load channels")
        return
    }
    address := strings.TrimRight(cfg.ServerURL, "/") + "/service/channels"
    req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, address, nil)
    if err != nil { writeError(w, 400, "Invalid Monita server address"); return }
    req.Header.Set("Authorization", "Bearer " + cfg.Token)
    response, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
    if err != nil { writeError(w, 502, "Could not contact Monita: " + err.Error()); return }
    defer response.Body.Close()
    if response.StatusCode != http.StatusOK {
        writeError(w, 502, "Monita channel lookup returned " + response.Status)
        return
    }
    var channels []monitaChannel
    if err := json.NewDecoder(io.LimitReader(response.Body, 128<<10)).Decode(&channels); err != nil {
        writeError(w, 502, "Monita returned an invalid channel list")
        return
    }
    if channels == nil { channels = []monitaChannel{} }
    writeJSON(w, 200, map[string]any{"channels":channels})
}
