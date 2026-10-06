package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	writeJSON(w, 200, map[string]bool{"needs_setup": count == 0})
}

type credentials struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

func (s *server) setup(w http.ResponseWriter, r *http.Request) {
	var count int
	if err := s.db.QueryRow(r.Context(), "SELECT count(*) FROM users").Scan(&count); err != nil || count != 0 {
		writeError(w, http.StatusConflict, "Calden is already set up")
		return
	}
	var in credentials
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.DisplayName) == "" {
		writeError(w, 400, "Name, username and password are required")
		return
	}
	if err := validateLogin(in.Username, in.Password); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	var id uuid.UUID
	err := s.db.QueryRow(r.Context(), `INSERT INTO users(username,display_name,password_hash,role,initials)
		VALUES(lower($1),$2,$3,'admin',$4) RETURNING id`,
		strings.TrimSpace(in.Username), strings.TrimSpace(in.DisplayName), string(hash), initials(in.DisplayName)).Scan(&id)
	if err != nil {
		writeError(w, 500, "Could not create administrator")
		return
	}
	token, _ := s.token(id, "admin")
	writeJSON(w, 201, map[string]any{"token": token})
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
	err := s.db.QueryRow(r.Context(), "SELECT id,password_hash,role,active FROM users WHERE username=lower($1)", strings.TrimSpace(in.Username)).Scan(&id,&hash,&role,&active)
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
	claims := jwt.MapClaims{"sub": id.String(), "role": role, "iat": time.Now().Unix(), "exp": time.Now().Add(30*24*time.Hour).Unix()}
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
		token, err := jwt.Parse(raw, func(t *jwt.Token) (any,error) {
			if t.Method != jwt.SigningMethodHS256 { return nil, errors.New("invalid signing method") }
			return s.jwtSecret,nil
		})
		if err != nil || !token.Valid {
			writeError(w, 401, "Session expired. Please sign in again")
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok { writeError(w,401,"Please sign in"); return }
		id, err := uuid.Parse(asString(claims["sub"]))
		if err != nil { writeError(w,401,"Please sign in"); return }
		var role string
		var active bool
		if err := s.db.QueryRow(r.Context(),"SELECT role,active FROM users WHERE id=$1",id).Scan(&role,&active); err != nil || !active {
			writeError(w,401,"Please sign in"); return
		}
		ctx := withActor(r.Context(), actor{ID:id,Role:role})
		next.ServeHTTP(w,r.WithContext(ctx))
	})
}

func (s *server) admin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
		if currentActor(r).Role != "admin" { writeError(w,403,"Administrator access is required"); return }
		next.ServeHTTP(w,r)
	})
}

func validateLogin(username,password string) error {
	u := strings.TrimSpace(username)
	if len(u) < 3 || len(u) > 50 { return errors.New("Username must be 3 to 50 characters") }
	if len(password) < 8 { return errors.New("Password must be at least 8 characters") }
	return nil
}

func initials(name string) string {
	parts := strings.Fields(name)
	if len(parts)==0 { return "?" }
	out := strings.ToUpper(string([]rune(parts[0])[0]))
	if len(parts)>1 { out += strings.ToUpper(string([]rune(parts[len(parts)-1])[0])) }
	return out
}
