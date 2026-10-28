package api

import (
	"errors"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/gigabytegrove/calden/internal/backup"
	"github.com/gigabytegrove/calden/internal/updater"
)

type Config struct {
	DB        *pgxpool.Pool
	JWTSecret []byte
	WebDir    string
	DataDir   string
	Version   string
    PushAccountFile string
    PushProjectID string
    PushSenderID string
    PushAppID string
    PushAPIKey string
}

type server struct {
	db        *pgxpool.Pool
	jwtSecret []byte
	webDir    string
	dataDir   string
	version   string
    pushAccountFile string
    pushProjectID string
    pushSenderID string
    pushAppID string
    pushAPIKey string
	updater   *updater.Manager
	backup    *backup.Manager
}

type actor struct {
	ID   uuid.UUID
	Role string
    IsChild bool
    ChildModules []string
    ChildLayout string
	ExpiresAt time.Time
}

type contextKey string

const actorKey contextKey = "actor"

func New(cfg Config) http.Handler {
	s := &server{db: cfg.DB, jwtSecret: cfg.JWTSecret, webDir: cfg.WebDir, dataDir: cfg.DataDir, version: cfg.Version,
    pushAccountFile:cfg.PushAccountFile,pushProjectID:cfg.PushProjectID,
    pushSenderID:cfg.PushSenderID,pushAppID:cfg.PushAppID,pushAPIKey:cfg.PushAPIKey}
	s.updater = updater.New(cfg.DataDir, cfg.Version)
	s.backup = backup.New(cfg.DataDir, cfg.Version)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
    mux.HandleFunc("POST /api/packages/inbound-email", s.inboundPackageEmail)
	mux.Handle("POST /api/client-errors", s.auth(http.HandlerFunc(s.clientError)))
	mux.HandleFunc("GET /api/setup/status", s.setupStatus)
	mux.HandleFunc("POST /api/setup", s.setup)
	mux.HandleFunc("POST /api/login", s.login)
	mux.Handle("GET /api/me", s.auth(http.HandlerFunc(s.me)))
    mux.Handle("GET /api/family/appearance", s.auth(http.HandlerFunc(s.loadFamilyAppearance)))
    mux.Handle("PUT /api/family/appearance", s.auth(s.admin(http.HandlerFunc(s.saveFamilyAppearance))))
    mux.Handle("GET /api/family/care/{kind}", s.auth(http.HandlerFunc(s.listCareSubjects)))
    mux.Handle("POST /api/family/care/{kind}", s.auth(http.HandlerFunc(s.saveCareSubject)))
    mux.Handle("PUT /api/family/care/{kind}/{id}", s.auth(http.HandlerFunc(s.saveCareSubject)))
    mux.Handle("GET /api/family/care/{kind}/{id}/items", s.auth(http.HandlerFunc(s.listCareItems)))
    mux.Handle("POST /api/family/care/{kind}/{id}/items", s.auth(http.HandlerFunc(s.saveCareItem)))
    mux.Handle("PUT /api/family/care/{kind}/{id}/items/{item_id}", s.auth(http.HandlerFunc(s.saveCareItem)))
    mux.Handle("PUT /api/family/care/{kind}/{id}/items/{item_id}/occurrences/{date}", s.auth(http.HandlerFunc(s.careOccurrence)))
    mux.Handle("GET /api/family/care/{kind}/{id}/history", s.auth(http.HandlerFunc(s.careHistory)))
    mux.Handle("GET /api/family/care/{kind}/{id}/terms", s.auth(http.HandlerFunc(s.careTerms)))
    mux.Handle("POST /api/family/care/{kind}/{id}/terms", s.auth(http.HandlerFunc(s.careTerms)))
    mux.Handle("GET /api/family/children", s.auth(http.HandlerFunc(s.listChildren)))
    mux.Handle("POST /api/family/children", s.auth(s.admin(http.HandlerFunc(s.createChild))))
    mux.Handle("PUT /api/family/children/{id}", s.auth(s.admin(http.HandlerFunc(s.updateChild))))
    mux.Handle("GET /api/me/notification-preferences", s.auth(http.HandlerFunc(s.getPersonalNotificationPreferences)))
    mux.Handle("PUT /api/me/notification-preferences", s.auth(http.HandlerFunc(s.savePersonalNotificationPreferences)))
	mux.Handle("GET /api/me/dashboard", s.auth(http.HandlerFunc(s.getDashboardPreferences)))
	mux.Handle("PUT /api/me/dashboard", s.auth(http.HandlerFunc(s.saveDashboardPreferences)))
	mux.Handle("PUT /api/me/week-start", s.auth(http.HandlerFunc(s.savePersonalWeekStart)))
	mux.Handle("PUT /api/me/confirmation-preferences", s.auth(http.HandlerFunc(s.updateConfirmationPreference)))
	mux.Handle("GET /api/settings/general", s.auth(http.HandlerFunc(s.generalSettings)))
	mux.Handle("GET /api/activity", s.auth(s.admin(http.HandlerFunc(s.listAudit))))
	mux.Handle("PUT /api/settings/general", s.auth(s.admin(http.HandlerFunc(s.saveGeneralSettings))))
    mux.Handle("PUT /api/settings/chores", s.auth(s.admin(http.HandlerFunc(s.saveChoresEnabled))))
    mux.Handle("PUT /api/settings/features", s.auth(s.admin(http.HandlerFunc(s.saveFeatureVisibility))))
    mux.Handle("GET /api/rewards/overview", s.auth(http.HandlerFunc(s.rewardsOverview)))
    mux.Handle("POST /api/rewards/catalog", s.auth(http.HandlerFunc(s.rewardCatalogCreate)))
    mux.Handle("PUT /api/rewards/catalog/{id}", s.auth(http.HandlerFunc(s.rewardCatalogUpdate)))
    mux.Handle("POST /api/rewards/catalog/{id}/claim", s.auth(http.HandlerFunc(s.claimReward)))
    mux.Handle("PUT /api/rewards/claims/{id}", s.auth(http.HandlerFunc(s.resolveRewardClaim)))
    mux.Handle("POST /api/rewards/chores/{id}/review", s.auth(http.HandlerFunc(s.choreReviewReward)))
    mux.Handle("POST /api/rewards/points/adjust", s.auth(http.HandlerFunc(s.pointAdjustment)))
    mux.Handle("PUT /api/rewards/allowances/plan", s.auth(http.HandlerFunc(s.saveAllowancePlan)))
    mux.Handle("POST /api/rewards/allowances/payment", s.auth(http.HandlerFunc(s.recordAllowancePayment)))
    mux.Handle("GET /api/routines", s.auth(http.HandlerFunc(s.routinesList)))
    mux.Handle("POST /api/routines", s.auth(http.HandlerFunc(s.createRoutine)))
    mux.Handle("PUT /api/routines/{id}", s.auth(http.HandlerFunc(s.updateRoutine)))
    mux.Handle("DELETE /api/routines/{id}", s.auth(http.HandlerFunc(s.archiveRoutine)))
    mux.Handle("PUT /api/routines/{id}/steps/{step_id}/status", s.auth(http.HandlerFunc(s.updateRoutineStep)))
    mux.Handle("GET /api/integrations/recipecove/status", s.auth(http.HandlerFunc(s.coveStatus)))
    mux.Handle("GET /api/integrations/recipecove/recipes", s.auth(http.HandlerFunc(s.coveRecipes)))
    mux.Handle("GET /api/integrations/recipecove/meals", s.auth(http.HandlerFunc(s.coveMeals)))
    mux.Handle("POST /api/integrations/recipecove/meals", s.auth(http.HandlerFunc(s.coveSaveMeal)))
    mux.Handle("PUT /api/integrations/recipecove/meals/{id}", s.auth(http.HandlerFunc(s.coveSaveMeal)))
    mux.Handle("DELETE /api/integrations/recipecove/meals/{id}", s.auth(http.HandlerFunc(s.coveDeleteMeal)))
    mux.Handle("GET /api/packages/provider", s.auth(s.admin(http.HandlerFunc(s.trackingProviderInfo))))
    mux.Handle("PUT /api/packages/provider", s.auth(s.admin(http.HandlerFunc(s.saveTrackingProvider))))
    mux.Handle("DELETE /api/packages/provider", s.auth(s.admin(http.HandlerFunc(s.deleteTrackingProvider))))
    mux.Handle("POST /api/packages/{id}/refresh", s.auth(http.HandlerFunc(s.refreshPackageTracking)))
    mux.Handle("GET /api/packages/mailbox", s.auth(http.HandlerFunc(s.packageMailboxInfo)))
    mux.Handle("PUT /api/packages/mailbox", s.auth(http.HandlerFunc(s.savePackageMailbox)))
    mux.Handle("DELETE /api/packages/mailbox", s.auth(http.HandlerFunc(s.deletePackageMailbox)))
    mux.Handle("POST /api/packages/mailbox/check", s.auth(http.HandlerFunc(s.checkPackageMailbox)))
    mux.Handle("GET /api/packages", s.auth(http.HandlerFunc(s.listPackages)))
    mux.Handle("POST /api/packages", s.auth(http.HandlerFunc(s.createPackage)))
    mux.Handle("GET /api/packages/{id}", s.auth(http.HandlerFunc(s.getPackage)))
    mux.Handle("GET /api/packages/{id}/history", s.auth(http.HandlerFunc(s.packageHistory)))
    mux.Handle("PUT /api/packages/{id}", s.auth(http.HandlerFunc(s.updatePackage)))
    mux.Handle("DELETE /api/packages/{id}", s.auth(http.HandlerFunc(s.deletePackage)))
    mux.Handle("POST /api/packages/import-email", s.auth(http.HandlerFunc(s.importPackageEmail)))
    mux.Handle("GET /api/packages/email-token", s.auth(http.HandlerFunc(s.packageEmailTokenInfo)))
    mux.Handle("POST /api/packages/email-token", s.auth(http.HandlerFunc(s.rotatePackageEmailToken)))
    mux.Handle("DELETE /api/packages/email-token", s.auth(http.HandlerFunc(s.revokePackageEmailToken)))
    mux.Handle("GET /api/shopping/lists", s.auth(http.HandlerFunc(s.listShoppingLists)))
    mux.Handle("POST /api/shopping/lists", s.auth(http.HandlerFunc(s.createShoppingList)))
    mux.Handle("PUT /api/shopping/lists/{id}", s.auth(http.HandlerFunc(s.saveShoppingList)))
    mux.Handle("DELETE /api/shopping/lists/{id}", s.auth(http.HandlerFunc(s.deleteShoppingList)))
    mux.Handle("GET /api/shopping/lists/{id}/history", s.auth(http.HandlerFunc(s.listShoppingHistory)))
    mux.Handle("GET /api/shopping/lists/{id}/items", s.auth(http.HandlerFunc(s.listShoppingItems)))
    mux.Handle("POST /api/shopping/lists/{id}/items", s.auth(http.HandlerFunc(s.createShoppingItem)))
    mux.Handle("PUT /api/shopping/lists/{id}/items/{item_id}", s.auth(http.HandlerFunc(s.saveShoppingItem)))
    mux.Handle("DELETE /api/shopping/lists/{id}/items/{item_id}", s.auth(http.HandlerFunc(s.deleteShoppingItem)))
    mux.Handle("GET /api/chores", s.auth(http.HandlerFunc(s.listChores)))
    mux.Handle("POST /api/chores", s.auth(http.HandlerFunc(s.createChore)))
    mux.Handle("PUT /api/chores/{id}", s.auth(http.HandlerFunc(s.updateChore)))
    mux.Handle("PUT /api/chores/{id}/status", s.auth(http.HandlerFunc(s.changeChoreStatus)))
    mux.Handle("DELETE /api/chores/{id}", s.auth(http.HandlerFunc(s.deleteChore)))
	mux.Handle("GET /api/users", s.auth(http.HandlerFunc(s.listUsers)))
	mux.Handle("POST /api/users", s.auth(s.admin(http.HandlerFunc(s.createUser))))
	mux.Handle("PUT /api/users/{id}", s.auth(s.admin(http.HandlerFunc(s.updateUser))))
	mux.Handle("DELETE /api/users/{id}", s.auth(s.admin(http.HandlerFunc(s.deactivateUser))))
	mux.Handle("POST /api/users/{id}/avatar", s.auth(http.HandlerFunc(s.uploadUserAvatar)))
	mux.Handle("DELETE /api/users/{id}/avatar", s.auth(http.HandlerFunc(s.deleteUserAvatar)))
	mux.Handle("GET /api/calendars", s.auth(http.HandlerFunc(s.listCalendars)))
	mux.Handle("POST /api/calendars", s.auth(s.calendarManager(http.HandlerFunc(s.createCalendar))))
	mux.Handle("GET /api/categories", s.auth(http.HandlerFunc(s.listCategories)))
	mux.Handle("POST /api/categories", s.auth(s.calendarManager(http.HandlerFunc(s.createCategory))))
	mux.Handle("PUT /api/categories/{id}", s.auth(s.calendarManager(http.HandlerFunc(s.updateCategory))))
	mux.Handle("DELETE /api/categories/{id}", s.auth(s.calendarManager(http.HandlerFunc(s.deleteCategory))))
	mux.Handle("GET /api/calendars/{id}/permissions", s.auth(s.calendarManager(http.HandlerFunc(s.calendarPermissions))))
	mux.Handle("GET /api/calendars/{id}/export.ics", s.auth(http.HandlerFunc(s.exportCalendarICS)))
	mux.Handle("POST /api/calendars/{id}/import.ics", s.auth(http.HandlerFunc(s.importCalendarICS)))
	mux.Handle("PUT /api/calendars/{id}/permissions", s.auth(s.calendarManager(http.HandlerFunc(s.setCalendarPermissions))))
	mux.Handle("GET /api/events", s.auth(http.HandlerFunc(s.listEvents)))
    mux.Handle("POST /api/scheduling/conflicts", s.auth(http.HandlerFunc(s.checkScheduleConflicts)))
    mux.Handle("GET /api/scheduling/busy", s.auth(http.HandlerFunc(s.listBusyBlocks)))
    mux.Handle("POST /api/scheduling/busy", s.auth(http.HandlerFunc(s.createBusyBlock)))
    mux.Handle("DELETE /api/scheduling/busy/{id}", s.auth(http.HandlerFunc(s.deleteBusyBlock)))
	mux.Handle("POST /api/events", s.auth(http.HandlerFunc(s.createEvent)))
	mux.Handle("GET /api/events/{id}/confirmations", s.auth(http.HandlerFunc(s.getEventConfirmations)))
	mux.Handle("PUT /api/events/{id}/confirmation", s.auth(http.HandlerFunc(s.respondToConfirmation)))
	mux.Handle("PUT /api/events/{id}", s.auth(http.HandlerFunc(s.updateEvent)))
	mux.Handle("DELETE /api/events/{id}", s.auth(http.HandlerFunc(s.deleteEvent)))
	mux.Handle("PUT /api/bills/{id}/payment", s.auth(http.HandlerFunc(s.setBillPayment)))
	mux.Handle("PUT /api/bills/{id}/allocation", s.auth(http.HandlerFunc(s.setBillAllocation)))
	mux.Handle("POST /api/bills/{id}/payments", s.auth(http.HandlerFunc(s.addBillPayment)))
	mux.Handle("PUT /api/bills/{id}/payments/{payment_id}", s.auth(http.HandlerFunc(s.updateBillPayment)))
	mux.Handle("DELETE /api/bills/{id}/payments/{payment_id}", s.auth(http.HandlerFunc(s.deleteBillPayment)))
	mux.Handle("PUT /api/bills/{id}/no-balance", s.auth(http.HandlerFunc(s.setBillNoBalance)))
	mux.Handle("GET /api/devices", s.auth(http.HandlerFunc(s.listDevices)))
    mux.Handle("GET /api/devices/household", s.auth(s.admin(http.HandlerFunc(s.listHouseholdDevices))))
	mux.Handle("POST /api/devices", s.auth(http.HandlerFunc(s.registerDevice)))
    mux.Handle("GET /api/notifications/push-config", s.auth(http.HandlerFunc(s.pushClientConfig)))
    mux.Handle("GET /api/notifications/push-diagnostics", s.auth(s.admin(http.HandlerFunc(s.pushDiagnostics))))
	mux.Handle("DELETE /api/devices/{id}", s.auth(http.HandlerFunc(s.revokeDevice)))
	mux.Handle("GET /api/notifications/stream", s.auth(http.HandlerFunc(s.streamNotifications)))
	mux.Handle("GET /api/notifications", s.auth(http.HandlerFunc(s.listNotifications)))
	mux.Handle("PUT /api/notifications/read-all", s.auth(http.HandlerFunc(s.markAllNotificationsRead)))
	mux.Handle("PUT /api/notifications/{id}/read", s.auth(http.HandlerFunc(s.markNotificationRead)))
	mux.Handle("DELETE /api/notifications/{id}", s.auth(http.HandlerFunc(s.dismissNotification)))
	mux.Handle("PUT /api/events/{id}/occurrences", s.auth(http.HandlerFunc(s.updateOccurrence)))
	mux.Handle("DELETE /api/events/{id}/occurrences", s.auth(http.HandlerFunc(s.deleteOccurrence)))
	mux.Handle("POST /api/events/{id}/occurrences/restore", s.auth(http.HandlerFunc(s.restoreOccurrence)))
	mux.Handle("PUT /api/calendars/{id}", s.auth(s.calendarManager(http.HandlerFunc(s.updateCalendar))))
	mux.Handle("DELETE /api/calendars/{id}", s.auth(s.calendarManager(http.HandlerFunc(s.deleteCalendar))))
	mux.Handle("GET /api/integrations/monita", s.auth(s.admin(http.HandlerFunc(s.getMonitaIntegration))))
	mux.Handle("PUT /api/integrations/monita", s.auth(s.admin(http.HandlerFunc(s.saveMonitaIntegration))))
	mux.Handle("POST /api/integrations/monita/test", s.auth(s.admin(http.HandlerFunc(s.testMonitaIntegration))))
    mux.Handle("POST /api/integrations/monita/channels", s.auth(s.admin(http.HandlerFunc(s.getMonitaChannels))))
    mux.Handle("POST /api/integrations/monita/connect", s.auth(s.admin(http.HandlerFunc(s.connectMonita))))
    mux.Handle("POST /api/integrations/monita/disconnect", s.auth(s.admin(http.HandlerFunc(s.disconnectMonita))))
	mux.Handle("POST /api/integrations/google/preview", s.auth(s.admin(http.HandlerFunc(s.previewGoogleCalendarExport))))
	mux.Handle("POST /api/integrations/google/import", s.auth(s.admin(http.HandlerFunc(s.importGoogleCalendarExport))))
	mux.Handle("GET /api/integrations/google/duplicates", s.auth(s.admin(http.HandlerFunc(s.scanGoogleDuplicates))))
	mux.Handle("POST /api/integrations/google/duplicates/preview", s.auth(s.admin(http.HandlerFunc(s.previewGoogleDuplicateRepair))))
	mux.Handle("POST /api/integrations/google/duplicates/repair", s.auth(s.admin(http.HandlerFunc(s.repairGoogleDuplicatesWithExport))))
	mux.Handle("GET /api/system/update", s.auth(s.admin(http.HandlerFunc(s.updateCheck))))
	mux.Handle("GET /api/system/update/status", s.auth(s.admin(http.HandlerFunc(s.updateStatus))))
	mux.Handle("GET /api/system/update/preferences", s.auth(s.admin(http.HandlerFunc(s.updatePreferences))))
	mux.Handle("PUT /api/system/update/preferences", s.auth(s.admin(http.HandlerFunc(s.saveUpdatePreferences))))
	mux.Handle("POST /api/system/update/install", s.auth(s.admin(http.HandlerFunc(s.installUpdate))))
	mux.Handle("POST /api/system/update/rollback", s.auth(s.admin(http.HandlerFunc(s.rollbackUpdate))))
	mux.Handle("GET /api/system/backup/status", s.auth(s.admin(http.HandlerFunc(s.backupStatus))))
	mux.Handle("GET /api/system/backup/download", s.auth(s.admin(http.HandlerFunc(s.downloadBackup))))
	mux.Handle("POST /api/system/backup/restore", s.auth(s.admin(http.HandlerFunc(s.stageRestore))))
	mux.Handle("DELETE /api/system/backup/restore", s.auth(s.admin(http.HandlerFunc(s.cancelRestore))))
	mux.Handle("GET /api/system/backups/{name}", s.auth(s.admin(http.HandlerFunc(s.downloadSavedBackup))))
	mux.Handle("DELETE /api/system/backups/{name}", s.auth(s.admin(http.HandlerFunc(s.deleteSavedBackup))))
	mux.Handle("POST /api/system/restart", s.auth(s.admin(http.HandlerFunc(s.restartSystem))))
	mux.HandleFunc("GET /media/avatars/{name}", s.avatarMedia)
	mux.Handle("/", s.static())
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data: blob: https:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
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

		// The web UI is updated in-place. All application entry points and
		// static files must bypass browser and intermediary caches.
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Header().Set("Surrogate-Control", "no-store")

		path := filepath.Join(s.webDir, filepath.Clean(r.URL.Path))
		if r.URL.Path != "/" {
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				fs.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store, max-age=0")
		w.Header().Set("Pragma", "no-cache")
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
	Type        string
}

var starterCalendars = []starterCalendar{
	{Key: "family", Name: "Family", Color: "#2563EB", Icon: "home", Description: "Plans and events for the whole family", Type: "standard"},
	{Key: "bills", Name: "Bills", Color: "#16A34A", Icon: "receipt", Description: "Bills, payments, and household due dates", Type: "bill_pay"},
	{Key: "appointments", Name: "Appointments", Color: "#7C3AED", Icon: "calendar", Description: "Appointments and scheduled visits", Type: "standard"},
	{Key: "school", Name: "School", Color: "#EAB308", Icon: "school", Description: "School events, activities, and deadlines", Type: "standard"},
	{Key: "birthdays", Name: "Birthdays", Color: "#EC4899", Icon: "cake", Description: "Birthdays and celebrations", Type: "standard"},
	{Key: "work", Name: "Work", Color: "#64748B", Icon: "briefcase", Description: "Work schedules and commitments", Type: "standard"},
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
		"household_name":  in.HouseholdName,
		"timezone":        in.Timezone,
		"setup_completed": "true",
		"setup_version":   "1",
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
		if err = tx.QueryRow(r.Context(), `INSERT INTO calendars(name,color,icon,description,calendar_type,created_by)
			VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
			calendar.Name, calendar.Color, calendar.Icon, calendar.Description, calendar.Type, id).Scan(&calendarID); err != nil {
			writeError(w, 500, "Could not create starter calendars")
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO calendar_permissions(calendar_id,user_id,can_view,can_edit,can_delete)
			VALUES($1,$2,true,true,true)`, calendarID, id); err != nil {
			writeError(w, 500, "Could not finish starter calendar access")
			return
		}
	}

	defaultCategories := []struct {
		Name  string
		Color string
		Icon  string
	}{
		{Name: "General", Color: "#64748B", Icon: "tag"},
		{Name: "Medical", Color: "#DC2626", Icon: "medical"},
		{Name: "Bills", Color: "#16A34A", Icon: "receipt"},
		{Name: "Birthday", Color: "#EC4899", Icon: "cake"},
		{Name: "School", Color: "#EAB308", Icon: "school"},
		{Name: "Work", Color: "#2563EB", Icon: "briefcase"},
	}
	for _, category := range defaultCategories {
		if _, err = tx.Exec(r.Context(), `INSERT INTO categories(name,color,icon,created_by)
			VALUES($1,$2,$3,$4)`, category.Name, category.Color, category.Icon, id); err != nil {
			writeError(w, 500, "Could not create starter categories")
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
		// Browsers cannot send Authorization headers during WebSocket upgrade.
		// Accept a scoped auth subprotocol ONLY on the notification stream.
		if auth == "" && r.URL.Path == "/api/notifications/stream" {
			for _, protocol := range websocket.Subprotocols(r) {
				if strings.HasPrefix(protocol, "calden.auth.") {
					auth = "Bearer " + strings.TrimPrefix(protocol, "calden.auth.")
					break
				}
			}
		}
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
		var active,isChild bool
		if err := s.db.QueryRow(r.Context(), "SELECT role,active,is_child FROM users WHERE id=$1", id).Scan(&role, &active,&isChild); err != nil || !active {
			writeError(w, 401, "Please sign in")
			return
		}
		expiry, expErr := claims.GetExpirationTime()
        if expErr != nil || expiry == nil {
            writeError(w, 401, "Session expiration is missing")
            return
        }
        var childModules []string
        var childLayout string
        if isChild {
            module:=childRequestModule(r.Method,r.URL.Path)
            if module=="" {
                writeError(w,403,"This action is not available for child accounts");return
            }
            err:=s.db.QueryRow(r.Context(),`SELECT allowed_modules,display_mode
              FROM family_child_profiles WHERE user_id=$1`,id).Scan(&childModules,&childLayout)
            if err!=nil {writeError(w,403,"Child account setup is incomplete");return}
            if !childModuleAllowed(childModules,module) {
                writeError(w,403,"A guardian has not enabled this feature");return
            }
        }
        ctx := withActor(r.Context(), actor{ID: id, Role: role, IsChild:isChild,
            ChildModules:childModules,ChildLayout:childLayout,ExpiresAt: expiry.Time})
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

// calendarManager authorizes calendar management, not system administration.
func (s *server) calendarManager(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        role := currentActor(r).Role
        if role != "admin" && role != "planner" {
            writeError(w, http.StatusForbidden, "Calendar manager access is required")
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

// clientError sends browser-only crashes to server stderr for Dokploy logs.
func (s *server) clientError(w http.ResponseWriter, r *http.Request) {
  defer r.Body.Close()
  var in struct {
    Message string `json:"message"`
    Version string `json:"version"`
  }
  if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&in); err != nil {
    writeError(w, http.StatusBadRequest, "Invalid error report")
    return
  }
  message := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(in.Message)
  version := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(in.Version)
  if len(message)>400 {message=message[:400]}
  if len(version)>60 {version=version[:60]}
  if strings.TrimSpace(message)=="" {
    writeError(w, http.StatusBadRequest, "Missing error message")
    return
  }
  log.Printf("CalDen browser error (client version %s): %s",version,message)
  w.WriteHeader(http.StatusNoContent)
}
