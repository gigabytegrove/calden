package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Config struct {
	DB        *pgxpool.Pool
	JWTSecret []byte
	WebDir    string
	Version   string
}

type server struct {
	db        *pgxpool.Pool
	jwtSecret []byte
	webDir    string
	version   string
}

type actor struct {
	ID   uuid.UUID
	Role string
}

type contextKey string

const actorKey contextKey = "actor"

func New(cfg Config) http.Handler {
	s := &server{db: cfg.DB, jwtSecret: cfg.JWTSecret, webDir: cfg.WebDir, version: cfg.Version}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/setup/status", s.setupStatus)
	mux.HandleFunc("POST /api/setup", s.setup)
	mux.HandleFunc("POST /api/login", s.login)
	mux.Handle("GET /api/me", s.auth(http.HandlerFunc(s.me)))
	mux.Handle("GET /api/settings/general", s.auth(http.HandlerFunc(s.generalSettings)))
	mux.Handle("GET /api/users", s.auth(http.HandlerFunc(s.listUsers)))
	mux.Handle("POST /api/users", s.auth(s.admin(http.HandlerFunc(s.createUser))))
	mux.Handle("GET /api/calendars", s.auth(http.HandlerFunc(s.listCalendars)))
	mux.Handle("POST /api/calendars", s.auth(s.admin(http.HandlerFunc(s.createCalendar))))
	mux.Handle("PUT /api/calendars/{id}/permissions", s.auth(s.admin(http.HandlerFunc(s.setCalendarPermissions))))
	mux.Handle("GET /api/events", s.auth(http.HandlerFunc(s.listEvents)))
	mux.Handle("POST /api/events", s.auth(http.HandlerFunc(s.createEvent)))
	mux.Handle("PUT /api/events/{id}", s.auth(http.HandlerFunc(s.updateEvent)))
	mux.Handle("DELETE /api/events/{id}", s.auth(http.HandlerFunc(s.deleteEvent)))
	mux.Handle("PUT /api/calendars/{id}", s.auth(s.admin(http.HandlerFunc(s.updateCalendar))))
	mux.Handle("DELETE /api/calendars/{id}", s.auth(s.admin(http.HandlerFunc(s.deleteCalendar))))
	mux.Handle("GET /api/integrations/monita", s.auth(s.admin(http.HandlerFunc(s.getMonitaIntegration))))
	mux.Handle("PUT /api/integrations/monita", s.auth(s.admin(http.HandlerFunc(s.saveMonitaIntegration))))
	mux.Handle("POST /api/integrations/monita/test", s.auth(s.admin(http.HandlerFunc(s.testMonitaIntegration))))
	mux.Handle("/", s.static())
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data: https:; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *server) static() http.Handler {
	fs := http.FileServer(http.Dir(s.webDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		path := filepath.Join(s.webDir, filepath.Clean(r.URL.Path))
		if r.URL.Path != "/" {
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				fs.ServeHTTP(w, r)
				return
			}
		}
		http.ServeFile(w, r, filepath.Join(s.webDir, "index.html"))
	})
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": s.version})
}

func (s *server) setupStatus(w http.ResponseWriter, r *http.Request) {
	var count int
	if err := s.db.QueryRow(r.Context(), "SELECT count(*) FROM users").Scan(&count); err != nil {
		writeError(w, 500, "Could not check setup status")
		return
	}
	writeJSON(w, 200, map[string]any{"needs_setup": count == 0, "setup_version": 1})
}

type credentials struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type setupRequest struct {
	HouseholdName    string   `json:"household_name"`
	Timezone         string   `json:"timezone"`
	DisplayName      string   `json:"display_name"`
	Username         string   `json:"username"`
	Password         string   `json:"password"`
	StarterCalendars []string `json:"starter_calendars"`
}

type starterCalendar struct {
	Key         string
	Name        string
	Color       string
	Icon        string
	Description string
}

var starterCalendars = []starterCalendar{
	{Key: "family", Name: "Family", Color: "#2563EB", Icon: "home", Description: "Plans and events for the whole family"},
	{Key: "bills", Name: "Bills", Color: "#16A34A", Icon: "receipt", Description: "Bills, payments, and household due dates"},
	{Key: "appointments", Name: "Appointments", Color: "#7C3AED", Icon: "calendar", Description: "Appointments and scheduled visits"},
	{Key: "school", Name: "School", Color: "#EAB308", Icon: "school", Description: "School events, activities, and deadlines"},
	{Key: "birthdays", Name: "Birthdays", Color: "#EC4899", Icon: "cake", Description: "Birthdays and celebrations"},
	{Key: "work", Name: "Work", Color: "#64748B", Icon: "briefcase", Description: "Work schedules and commitments"},
}

func (s *server) setup(w http.ResponseWriter, r *http.Request) {
	var count int
	if err := s.db.QueryRow(r.Context(), "SELECT count(*) FROM users").Scan(&count); err != nil {
		writeError(w, 500, "Could not check setup status")
		return
	}
	if count != 0 {
		writeError(w, http.StatusConflict, "CalDen is already set up")
		return
	}

	var in setupRequest
	if err := decode(r, &in); err != nil {
		writeError(w, 400, "Check the setup details")
		return
	}

	in.HouseholdName = strings.TrimSpace(in.HouseholdName)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Username = strings.TrimSpace(in.Username)
	in.Timezone = strings.TrimSpace(in.Timezone)

	if in.HouseholdName == "" {
		writeError(w, 400, "Give your family calendar a name")
		return
	}
	if len(in.HouseholdName) > 100 {
		writeError(w, 400, "Family name must be 100 characters or fewer")
		return
	}
	if in.DisplayName == "" {
		writeError(w, 400, "Enter your name")
		return
	}
	if in.Timezone == "" {
		in.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		writeError(w, 400, "Choose a valid time zone")
		return
	}
	if err := validateLogin(in.Username, in.Password); err != nil {
		writeError(w, 400, err.Error())
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, 500, "Could not secure the administrator account")
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not begin setup")
		return
	}
	defer tx.Rollback(r.Context())

	var id uuid.UUID
	err = tx.QueryRow(r.Context(), `INSERT INTO users(username,display_name,password_hash,role,initials)
		VALUES(lower($1),$2,$3,'admin',$4) RETURNING id`,
		in.Username, in.DisplayName, string(hash), initials(in.DisplayName)).Scan(&id)
	if err != nil {
		writeError(w, 500, "Could not create the administrator")
		return
	}

	settings := map[string]string{
		"household_name": in.HouseholdName,
		"timezone": in.Timezone,
		"setup_completed": "true",
		"setup_version": "1",
	}
	for key, value := range settings {
		if _, err = tx.Exec(r.Context(), `INSERT INTO app_settings(key,value) VALUES($1,$2)
			ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=now()`, key, value); err != nil {
			writeError(w, 500, "Could not save family settings")
			return
		}
	}

	selected := map[string]bool{}
	for _, key := range in.StarterCalendars {
		selected[strings.ToLower(strings.TrimSpace(key))] = true
	}
	if len(selected) == 0 {
		selected["family"] = true
	}

	for _, calendar := range starterCalendars {
		if !selected[calendar.Key] {
			continue
		}
		var calendarID uuid.UUID
		if err = tx.QueryRow(r.Context(), `INSERT INTO calendars(name,color,icon,description,created_by)
			VALUES($1,$2,$3,$4,$5) RETURNING id`,
			calendar.Name, calendar.Color, calendar.Icon, calendar.Description, id).Scan(&calendarID); err != nil {
			writeError(w, 500, "Could not create starter calendars")
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO calendar_permissions(calendar_id,user_id,can_view,can_edit,can_delete)
			VALUES($1,$2,true,true,true)`, calendarID, id); err != nil {
			writeError(w, 500, "Could not finish starter calendar access")
			return
		}
	}

	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not finish setup")
		return
	}

	token, err := s.token(id, "admin")
	if err != nil {
		writeError(w, 500, "Setup finished, but CalDen could not sign you in")
		return
	}
	writeJSON(w, 201, map[string]any{"token": token, "household_name": in.HouseholdName})
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if decode(r, &in) != nil {
		writeError(w, 400, "Username and password are required")
		return
	}
	var id uuid.UUID
	var hash, role string
	var active bool
	err := s.db.QueryRow(r.Context(), "SELECT id,password_hash,role,active FROM users WHERE username=lower($1)", strings.TrimSpace(in.Username)).Scan(&id, &hash, &role, &active)
	if err != nil || !active || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		writeError(w, 401, "Incorrect username or password")
		return
	}
	token, err := s.token(id, role)
	if err != nil {
		writeError(w, 500, "Could not sign in")
		return
	}
	writeJSON(w, 200, map[string]any{"token": token})
}

func (s *server) token(id uuid.UUID, role string) (string, error) {
	claims := jwt.MapClaims{"sub": id.String(), "role": role, "iat": time.Now().Unix(), "exp": time.Now().Add(30 * 24 * time.Hour).Unix()}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

func (s *server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, 401, "Please sign in")
			return
		}
		raw := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		token, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
			if t.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("invalid signing method")
			}
			return s.jwtSecret, nil
		})
		if err != nil || !token.Valid {
			writeError(w, 401, "Session expired. Please sign in again")
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			writeError(w, 401, "Please sign in")
			return
		}
		id, err := uuid.Parse(asString(claims["sub"]))
		if err != nil {
			writeError(w, 401, "Please sign in")
			return
		}
		var role string
		var active bool
		if err := s.db.QueryRow(r.Context(), "SELECT role,active FROM users WHERE id=$1", id).Scan(&role, &active); err != nil || !active {
			writeError(w, 401, "Please sign in")
			return
		}
		ctx := withActor(r.Context(), actor{ID: id, Role: role})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *server) admin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentActor(r).Role != "admin" {
			writeError(w, 403, "Administrator access is required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func validateLogin(username, password string) error {
	u := strings.TrimSpace(username)
	if len(u) < 3 || len(u) > 50 {
		return errors.New("Username must be 3 to 50 characters")
	}
	if len(password) < 8 {
		return errors.New("Password must be at least 8 characters")
	}
	return nil
}

func initials(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "?"
	}
	out := strings.ToUpper(string([]rune(parts[0])[0]))
	if len(parts) > 1 {
		out += strings.ToUpper(string([]rune(parts[len(parts)-1])[0]))
	}
	return out
}
