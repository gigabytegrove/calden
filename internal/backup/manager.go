package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	Product            = "CalDen"
	FormatVersion      = 1
	PendingRestoreName = ".calden-restore-pending.tar.gz"
	RestoreResultName  = ".calden-restore-result.json"
)

type Manager struct {
	DataDir string
	Version string
}

type Manifest struct {
	FormatVersion int       `json:"format_version"`
	Product       string    `json:"product"`
	Version       string    `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
	DatabaseFile  string    `json:"database_file"`
}

type RestoreStatus struct {
	Pending   bool           `json:"pending"`
	Last      map[string]any `json:"last,omitempty"`
	Backups   []SavedBackup  `json:"backups"`
}

type SavedBackup struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

func New(dataDir, version string) *Manager {
	return &Manager{DataDir: dataDir, Version: version}
}

func (m *Manager) CreateBundle(ctx context.Context) (string, string, error) {
	if err := os.MkdirAll(filepath.Join(m.DataDir, "backups"), 0700); err != nil {
		return "", "", err
	}
	work, err := os.MkdirTemp(m.DataDir, ".backup-work-*")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(work)

	dumpPath := filepath.Join(work, "calden.dump")
	if err := dumpDatabase(ctx, dumpPath); err != nil {
		return "", "", err
	}

	filename := "calden-backup-" + time.Now().UTC().Format("20060102-150405") + ".tar.gz"
	bundlePath := filepath.Join(m.DataDir, "backups", filename)
	tempPath := bundlePath + ".tmp"
	_ = os.Remove(tempPath)

	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return "", "", err
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	closeAll := func(primary error) error {
		tarErr := tw.Close()
		gzipErr := gz.Close()
		fileErr := file.Close()
		if primary != nil {
			return primary
		}
		if tarErr != nil {
			return tarErr
		}
		if gzipErr != nil {
			return gzipErr
		}
		return fileErr
	}

	manifest := Manifest{
		FormatVersion: FormatVersion,
		Product:       Product,
		Version:       m.Version,
		CreatedAt:     time.Now().UTC(),
		DatabaseFile:  "calden.dump",
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = closeAll(err)
		return "", "", err
	}
	if err := addBytes(tw, "manifest.json", body, 0600); err != nil {
		_ = closeAll(err)
		return "", "", err
	}
	if err := addFile(tw, dumpPath, "calden.dump"); err != nil {
		_ = closeAll(err)
		return "", "", err
	}

	for _, name := range []string{".jwt-secret", ".calden-update-preferences.json"} {
		path := filepath.Join(m.DataDir, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			if err := addFile(tw, path, filepath.ToSlash(filepath.Join("data", name))); err != nil {
				_ = closeAll(err)
				return "", "", err
			}
		}
	}
	uploads := filepath.Join(m.DataDir, "uploads")
	if info, err := os.Stat(uploads); err == nil && info.IsDir() {
		if err := addTree(tw, uploads, "data/uploads"); err != nil {
			_ = closeAll(err)
			return "", "", err
		}
	}

	if err := closeAll(nil); err != nil {
		_ = os.Remove(tempPath)
		return "", "", err
	}
	if err := os.Rename(tempPath, bundlePath); err != nil {
		_ = os.Remove(tempPath)
		return "", "", err
	}
	return bundlePath, filename, nil
}

func (m *Manager) ValidateAndStage(source io.Reader, maxBytes int64) (Manifest, error) {
	if err := os.MkdirAll(m.DataDir, 0700); err != nil {
		return Manifest{}, err
	}
	temp, err := os.CreateTemp(m.DataDir, ".restore-upload-*.tar.gz")
	if err != nil {
		return Manifest{}, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	written, copyErr := io.Copy(temp, io.LimitReader(source, maxBytes+1))
	closeErr := temp.Close()
	if copyErr != nil {
		return Manifest{}, copyErr
	}
	if closeErr != nil {
		return Manifest{}, closeErr
	}
	if written > maxBytes {
		return Manifest{}, errors.New("backup exceeds restore upload limit")
	}

	manifest, dumpPath, err := validateBundle(tempPath, m.DataDir)
	if err != nil {
		return Manifest{}, err
	}
	defer os.RemoveAll(filepath.Dir(dumpPath))
	if err := validateDump(dumpPath); err != nil {
		return Manifest{}, fmt.Errorf("database backup is invalid: %w", err)
	}

	pending := filepath.Join(m.DataDir, PendingRestoreName)
	if err := os.Rename(tempPath, pending); err != nil {
		return Manifest{}, err
	}
	_ = os.Remove(filepath.Join(m.DataDir, RestoreResultName))
	return manifest, nil
}

func (m *Manager) CancelPending() error {
	err := os.Remove(filepath.Join(m.DataDir, PendingRestoreName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (m *Manager) Status() RestoreStatus {
	status := RestoreStatus{Backups: []SavedBackup{}}
	if _, err := os.Stat(filepath.Join(m.DataDir, PendingRestoreName)); err == nil {
		status.Pending = true
	}
	if body, err := os.ReadFile(filepath.Join(m.DataDir, RestoreResultName)); err == nil {
		_ = json.Unmarshal(body, &status.Last)
	}
	entries, _ := os.ReadDir(filepath.Join(m.DataDir, "backups"))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.gz") && !strings.HasSuffix(entry.Name(), ".dump") {
			continue
		}
		if info, err := entry.Info(); err == nil {
			status.Backups = append(status.Backups, SavedBackup{Name: entry.Name(), Size: info.Size(), CreatedAt: info.ModTime().UTC()})
		}
	}
	return status
}


func (m *Manager) SavedPath(name string) (string, error) {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		return "", errors.New("invalid backup name")
	}
	if !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".dump") {
		return "", errors.New("invalid backup name")
	}
	path := filepath.Join(m.DataDir, "backups", name)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("backup is not a file")
	}
	return path, nil
}

func (m *Manager) DeleteSaved(name string) error {
	path, err := m.SavedPath(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func validateBundle(path, dataDir string) (Manifest, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, "", err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return Manifest{}, "", errors.New("file is not a gzip-compressed CalDen backup")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	work, err := os.MkdirTemp(dataDir, ".restore-validate-*")
	if err != nil {
		return Manifest{}, "", err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(work)
		}
	}()

	var manifest Manifest
	foundManifest := false
	foundDump := false
	dumpPath := filepath.Join(work, "calden.dump")

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Manifest{}, "", err
		}
		name := filepath.ToSlash(filepath.Clean(header.Name))
		if strings.HasPrefix(name, "../") || name == ".." || filepath.IsAbs(header.Name) {
			return Manifest{}, "", fmt.Errorf("backup contains unsafe path %q", header.Name)
		}
		if name != "manifest.json" && name != "calden.dump" && !strings.HasPrefix(name, "data/") {
			return Manifest{}, "", fmt.Errorf("backup contains unsupported entry %q", header.Name)
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			return Manifest{}, "", fmt.Errorf("backup contains unsupported entry type %q", header.Name)
		}
		switch name {
		case "manifest.json":
			if header.Size > 1<<20 {
				return Manifest{}, "", errors.New("backup manifest is too large")
			}
			if err := json.NewDecoder(io.LimitReader(tr, 1<<20)).Decode(&manifest); err != nil {
				return Manifest{}, "", errors.New("backup manifest is invalid")
			}
			foundManifest = true
		case "calden.dump":
			out, err := os.OpenFile(dumpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
			if err != nil {
				return Manifest{}, "", err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return Manifest{}, "", err
			}
			if err := out.Close(); err != nil {
				return Manifest{}, "", err
			}
			foundDump = true
		}
	}
	if !foundManifest || manifest.Product != Product || manifest.FormatVersion != FormatVersion {
		return Manifest{}, "", errors.New("file is not a supported CalDen backup")
	}
	if manifest.DatabaseFile != "calden.dump" || !foundDump {
		return Manifest{}, "", errors.New("CalDen database backup is missing")
	}
	cleanup = false
	return manifest, dumpPath, nil
}

func validateDump(path string) error {
	output, err := exec.Command("pg_restore", "--list", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func dumpDatabase(ctx context.Context, destination string) error {
	host := env("CALDEN_DB_HOST", "localhost")
	port := env("CALDEN_DB_PORT", "5432")
	name := env("CALDEN_DB_NAME", "calden")
	user := env("CALDEN_DB_USER", "calden")
	password, err := databasePassword()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "pg_dump", "-h", host, "-p", port, "-U", user, "-d", name, "-Fc", "-f", destination)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pg_dump failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func databasePassword() (string, error) {
	if file := strings.TrimSpace(os.Getenv("CALDEN_DB_PASSWORD_FILE")); file != "" {
		body, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(body)), nil
	}
	return strings.TrimSpace(os.Getenv("CALDEN_DB_PASSWORD")), nil
}

func addFile(tw *tar.Writer, source, name string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(name)
	header.Mode = int64(info.Mode().Perm())
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(tw, file)
	return err
}

func addBytes(tw *tar.Writer, name string, body []byte, mode int64) error {
	header := &tar.Header{Name: filepath.ToSlash(name), Mode: mode, Size: int64(len(body)), ModTime: time.Now().UTC(), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err := tw.Write(body)
	return err
}

func addTree(tw *tar.Writer, sourceRoot, archiveRoot string) error {
	return filepath.Walk(sourceRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == sourceRoot {
			return nil
		}
		rel, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(archiveRoot, rel))
		if info.IsDir() {
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			header.Name = name + "/"
			header.Typeflag = tar.TypeDir
			return tw.WriteHeader(header)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return addFile(tw, path, name)
	})
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
