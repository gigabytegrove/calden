package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gigabytegrove/calden/internal/api"
	"github.com/gigabytegrove/calden/internal/reminders"
	"github.com/gigabytegrove/calden/internal/store"
)

var version = "0.1.0-alpha10.1"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("CalDen\nVersion: %s\n", version)
		return
	}
	port := env("CALDEN_PORT", "8787")
	databaseURL, err := databaseURLFromEnv()
	if err != nil {
		log.Fatalf("database configuration: %v", err)
	}
	dataDir := env("CALDEN_DATA_DIR", "./data")
	webDir := env("CALDEN_WEB_DIR", "./web")

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		log.Fatalf("create data directory: %v", err)
	}

	jwtSecret, err := loadOrCreateSecret(dataDir)
	if err != nil {
		log.Fatalf("load signing secret: %v", err)
	}

	ctx := context.Background()
	db, err := store.Open(ctx, databaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	if err := store.Migrate(ctx, db); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	reminders.Start(workerCtx, db)

	handler := api.New(api.Config{
		DB:        db,
		JWTSecret: jwtSecret,
		WebDir:    webDir,
		DataDir:   dataDir,
		Version:   version,
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("CalDen %s listening on :%s", version, port)
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	workerCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func databaseURLFromEnv() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("CALDEN_DATABASE_URL")); explicit != "" {
		return explicit, nil
	}

	host := env("CALDEN_DB_HOST", "localhost")
	port := env("CALDEN_DB_PORT", "5432")
	name := env("CALDEN_DB_NAME", "calden")
	user := env("CALDEN_DB_USER", "calden")
	password := strings.TrimSpace(os.Getenv("CALDEN_DB_PASSWORD"))
	if passwordFile := strings.TrimSpace(os.Getenv("CALDEN_DB_PASSWORD_FILE")); passwordFile != "" {
		b, err := os.ReadFile(passwordFile)
		if err != nil {
			return "", fmt.Errorf("read CALDEN_DB_PASSWORD_FILE: %w", err)
		}
		password = strings.TrimSpace(string(b))
	}
	if password == "" {
		password = "calden"
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   host + ":" + port,
		Path:   name,
	}
	q := u.Query()
	q.Set("sslmode", env("CALDEN_DB_SSLMODE", "disable"))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func loadOrCreateSecret(dataDir string) ([]byte, error) {
	path := filepath.Join(dataDir, ".jwt-secret")
	if b, err := os.ReadFile(path); err == nil {
		decoded, decErr := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(b)))
		if decErr == nil && len(decoded) >= 32 {
			return decoded, nil
		}
	}

	secret := make([]byte, 48)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(secret)
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0600); err != nil {
		return nil, fmt.Errorf("persist secret: %w", err)
	}
	return secret, nil
}
