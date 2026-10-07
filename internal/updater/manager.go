package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const defaultRepository = "gigabytegrove/calden"

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

type Manager struct {
	DataDir        string
	StatusFile     string
	PreferencesFile string
	RuntimeDir     string
	Repository     string
	CurrentVersion string
	HTTPClient     *http.Client
	Exec           func(string, []string, []string) error
}

type Preferences struct {
	Channel   string `json:"channel"`
	AutoCheck bool   `json:"auto_check"`
}

type Activity struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
}

type Status struct {
	Ready          bool       `json:"ready"`
	State          string     `json:"state"`
	Version        string     `json:"version,omitempty"`
	Message        string     `json:"message,omitempty"`
	Step           string     `json:"step,omitempty"`
	Progress       int        `json:"progress"`
	BackupPath     string     `json:"backup_path,omitempty"`
	RollbackReady  bool       `json:"rollback_ready"`
	Activity       []Activity `json:"activity,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

type Release struct {
	Version      string    `json:"version"`
	Tag          string    `json:"tag"`
	Name         string    `json:"name"`
	Notes        string    `json:"notes"`
	Prerelease   bool      `json:"prerelease"`
	PublishedAt  time.Time `json:"published_at"`
	HTMLURL      string    `json:"html_url"`
}

type CheckResult struct {
	CurrentVersion string      `json:"current_version"`
	Preferences    Preferences `json:"preferences"`
	CurrentChannel string      `json:"current_channel"`
	Available      bool        `json:"available"`
	Latest         *Release    `json:"latest,omitempty"`
	Status         Status      `json:"status"`
}

type releasePayload struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

type previousRuntime struct {
	Kind    string `json:"kind"`
	Version string `json:"version"`
	Path    string `json:"path,omitempty"`
}

func New(dataDir, currentVersion string) *Manager {
	m := &Manager{
		DataDir:         dataDir,
		StatusFile:      filepath.Join(dataDir, ".calden-update-status.json"),
		PreferencesFile: filepath.Join(dataDir, ".calden-update-preferences.json"),
		RuntimeDir:      filepath.Join(dataDir, ".calden-runtime"),
		Repository:      defaultRepository,
		CurrentVersion:  strings.TrimSpace(currentVersion),
		HTTPClient:      &http.Client{Timeout: 10 * time.Minute},
		Exec:            syscall.Exec,
	}
	if v := strings.TrimSpace(os.Getenv("CALDEN_UPDATE_REPOSITORY")); v != "" {
		m.Repository = v
	}
	m.finalizeRestart()
	return m
}

func (m *Manager) Preferences() (Preferences, error) {
	p := Preferences{Channel: defaultChannelForVersion(m.CurrentVersion), AutoCheck: true}
	body, err := os.ReadFile(m.PreferencesFile)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return Preferences{}, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return p, nil
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return Preferences{}, err
	}
	p.Channel = normalizeChannel(p.Channel)
	if p.Channel == "" {
		p.Channel = defaultChannelForVersion(m.CurrentVersion)
	}
	return p, nil
}

func (m *Manager) SavePreferences(p Preferences) (Preferences, error) {
	p.Channel = normalizeChannel(p.Channel)
	if p.Channel == "" {
		return Preferences{}, errors.New("update channel must be stable, rc, beta, alpha, or preview")
	}
	if err := os.MkdirAll(filepath.Dir(m.PreferencesFile), 0700); err != nil {
		return Preferences{}, err
	}
	body, err := json.Marshal(p)
	if err != nil {
		return Preferences{}, err
	}
	if err := atomicWrite(m.PreferencesFile, body, 0600); err != nil {
		return Preferences{}, err
	}
	return p, nil
}

func (m *Manager) Status() Status {
	status, err := m.readStatus()
	if err != nil {
		return Status{Ready: false, State: "unavailable", Message: "Update status could not be read."}
	}
	status.Ready = true
	_, err = os.Stat(filepath.Join(m.RuntimeDir, "previous.json"))
	status.RollbackReady = err == nil
	return status
}

func (m *Manager) Check(ctx context.Context) (CheckResult, error) {
	prefs, err := m.Preferences()
	if err != nil {
		return CheckResult{}, err
	}
	releases, err := m.fetchReleases(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	latest := latestEligible(releases, prefs.Channel)
	result := CheckResult{
		CurrentVersion: m.CurrentVersion,
		Preferences:    prefs,
		CurrentChannel: releaseChannel(m.CurrentVersion),
		Status:         m.Status(),
	}
	if latest != nil {
		result.Latest = &Release{
			Version: strings.TrimPrefix(latest.TagName, "v"),
			Tag: latest.TagName,
			Name: latest.Name,
			Notes: latest.Body,
			Prerelease: latest.Prerelease,
			PublishedAt: latest.PublishedAt,
			HTMLURL: latest.HTMLURL,
		}
		result.Available = compareVersions(result.Latest.Version, m.CurrentVersion) > 0
	}
	return result, nil
}

func (m *Manager) Install(ctx context.Context, version string) (Status, error) {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if !versionPattern.MatchString(version) {
		return Status{}, errors.New("invalid release version")
	}
	status, err := m.readStatus()
	if err != nil {
		return Status{}, err
	}
	if activeState(status.State) {
		return status, errors.New("an update is already in progress")
	}

	release, err := m.releaseByVersion(ctx, version)
	if err != nil {
		return Status{}, err
	}
	prefs, err := m.Preferences()
	if err != nil {
		return Status{}, err
	}
	if !eligibleForChannel(release, prefs.Channel) {
		return Status{}, errors.New("release is not allowed by the selected update channel")
	}

	started := time.Now().UTC()
	initial := Status{
		Ready: true, State: "preparing", Version: version,
		Message: "Preparing update", Step: "Preparing update", Progress: 2, StartedAt: &started,
		Activity: []Activity{{Timestamp: started, Message: "Update started"}},
	}
	if err := m.writeStatus(initial); err != nil {
		return Status{}, err
	}
	go m.performInstall(version, *release, started)
	return initial, nil
}

func (m *Manager) Rollback() (Status, error) {
	current, err := m.readStatus()
	if err != nil {
		return Status{}, err
	}
	if activeState(current.State) {
		return current, errors.New("an update is already in progress")
	}
	body, err := os.ReadFile(filepath.Join(m.RuntimeDir, "previous.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Status{}, errors.New("there is no previous runtime to roll back to")
		}
		return Status{}, err
	}
	var previous previousRuntime
	if err := json.Unmarshal(body, &previous); err != nil {
		return Status{}, err
	}
	started := time.Now().UTC()
	status := Status{
		Ready: true, State: "restarting", Version: previous.Version,
		Message: "Rolling back to the previous CalDen runtime", Step: "Restarting CalDen",
		Progress: 98, StartedAt: &started,
		Activity: []Activity{{Timestamp: started, Message: "Rollback started"}, {Timestamp: time.Now().UTC(), Message: "Restarting CalDen"}},
	}
	if err := m.writeStatus(status); err != nil {
		return Status{}, err
	}
	go func() {
		time.Sleep(900 * time.Millisecond)
		if previous.Kind == "base" {
			_ = os.Remove(filepath.Join(m.RuntimeDir, "current"))
			m.execRuntime("/app/calden", "/app/web")
			return
		}
		if previous.Kind == "runtime" && previous.Path != "" {
			currentLink := filepath.Join(m.RuntimeDir, "current")
			_ = os.Remove(currentLink)
			_ = os.Symlink(previous.Path, currentLink)
			m.execRuntime(filepath.Join(previous.Path, "calden"), filepath.Join(previous.Path, "web"))
		}
	}()
	return status, nil
}

func (m *Manager) performInstall(version string, release releasePayload, started time.Time) {
	assetName, err := runtimeAssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		m.fail(version, started, "This system architecture is not supported by automatic updates.", err)
		return
	}
	assetURL, checksumURL, err := releaseAssets(release, assetName)
	if err != nil {
		m.fail(version, started, "The release is missing a required update file.", err)
		return
	}

	if err := os.MkdirAll(m.RuntimeDir, 0700); err != nil {
		m.fail(version, started, "The update directory could not be prepared.", err)
		return
	}
	tempArchive := filepath.Join(m.RuntimeDir, "."+assetName+".tmp")
	_ = os.Remove(tempArchive)

	m.progress("backup", "Creating safety backup", "Creating PostgreSQL safety backup", 8)
	backupPath, err := m.createDatabaseBackup(version)
	if err != nil {
		m.fail(version, started, "CalDen would not install the update because a safety backup could not be created.", err)
		return
	}
	status, _ := m.readStatus()
	status.BackupPath = backupPath
	_ = m.writeStatus(status)

	m.progress("downloading", "Downloading update", "Downloading release bundle", 14)
	if err := m.downloadFile(assetURL, tempArchive, 14, 62); err != nil {
		_ = os.Remove(tempArchive)
		m.fail(version, started, "The update could not be downloaded.", err)
		return
	}

	m.progress("verifying", "Verifying update", "Checking SHA-256 integrity", 68)
	checksums, err := m.downloadText(checksumURL)
	if err != nil {
		_ = os.Remove(tempArchive)
		m.fail(version, started, "The release checksum could not be downloaded.", err)
		return
	}
	if err := verifyChecksum(tempArchive, checksums, assetName); err != nil {
		_ = os.Remove(tempArchive)
		m.fail(version, started, "The downloaded update failed integrity verification.", err)
		return
	}

	m.progress("installing", "Staging update", "Extracting verified release", 76)
	releasesDir := filepath.Join(m.RuntimeDir, "releases")
	if err := os.MkdirAll(releasesDir, 0700); err != nil {
		m.fail(version, started, "The release directory could not be prepared.", err)
		return
	}
	stage := filepath.Join(releasesDir, version+".tmp")
	target := filepath.Join(releasesDir, version)
	_ = os.RemoveAll(stage)
	if err := os.MkdirAll(stage, 0700); err != nil {
		m.fail(version, started, "The release staging directory could not be created.", err)
		return
	}
	if err := extractBundle(tempArchive, stage); err != nil {
		_ = os.RemoveAll(stage)
		m.fail(version, started, "The release bundle could not be extracted.", err)
		return
	}
	_ = os.Remove(tempArchive)
	binary := filepath.Join(stage, "calden")
	webDir := filepath.Join(stage, "web")
	if err := os.Chmod(binary, 0755); err != nil {
		_ = os.RemoveAll(stage)
		m.fail(version, started, "The staged CalDen binary could not be prepared.", err)
		return
	}
	if info, err := os.Stat(webDir); err != nil || !info.IsDir() {
		_ = os.RemoveAll(stage)
		m.fail(version, started, "The release does not contain the CalDen web interface.", errors.New("web directory missing"))
		return
	}
	if err := verifyRuntime(binary, version); err != nil {
		_ = os.RemoveAll(stage)
		m.fail(version, started, "The staged runtime did not pass verification.", err)
		return
	}

	m.progress("installing", "Installing update", "Activating verified release", 88)
	_ = os.RemoveAll(target)
	if err := os.Rename(stage, target); err != nil {
		m.fail(version, started, "The verified release could not be activated.", err)
		return
	}
	if err := m.rememberPrevious(); err != nil {
		m.fail(version, started, "The previous runtime could not be preserved for rollback.", err)
		return
	}
	currentLink := filepath.Join(m.RuntimeDir, "current")
	_ = os.Remove(currentLink)
	if err := os.Symlink(target, currentLink); err != nil {
		m.fail(version, started, "The new runtime could not be selected.", err)
		return
	}

	m.progress("restarting", "Restarting CalDen", "Restarting into the new runtime", 98)
	time.Sleep(900 * time.Millisecond)
	m.execRuntime(filepath.Join(target, "calden"), filepath.Join(target, "web"))
}

func (m *Manager) rememberPrevious() error {
	currentLink := filepath.Join(m.RuntimeDir, "current")
	meta := previousRuntime{Kind: "base", Version: m.CurrentVersion}
	if target, err := os.Readlink(currentLink); err == nil {
		meta = previousRuntime{Kind: "runtime", Version: m.CurrentVersion, Path: target}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	body, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(m.RuntimeDir, "previous.json"), body, 0600)
}

func (m *Manager) execRuntime(binary, webDir string) {
	execFn := m.Exec
	if execFn == nil {
		execFn = syscall.Exec
	}
	env := append([]string{}, os.Environ()...)
	env = setEnv(env, "CALDEN_WEB_DIR", webDir)
	args := append([]string{binary}, os.Args[1:]...)
	if err := execFn(binary, args, env); err != nil {
		started := time.Now().UTC()
		m.fail("", started, "CalDen could not restart into the selected runtime.", err)
	}
}

func (m *Manager) createDatabaseBackup(targetVersion string) (string, error) {
	backupDir := filepath.Join(m.DataDir, "backups")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(backupDir, fmt.Sprintf("pre-update-%s-to-%s-%s.dump", safeName(m.CurrentVersion), safeName(targetVersion), time.Now().UTC().Format("20060102-150405")))
	host := env("CALDEN_DB_HOST", "localhost")
	port := env("CALDEN_DB_PORT", "5432")
	name := env("CALDEN_DB_NAME", "calden")
	user := env("CALDEN_DB_USER", "calden")
	password, err := databasePassword()
	if err != nil {
		return "", err
	}
	args := []string{"-h", host, "-p", port, "-U", user, "-d", name, "-Fc", "-f", path}
	cmd := exec.Command("pg_dump", args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("pg_dump failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return path, nil
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

func (m *Manager) fetchReleases(ctx context.Context) ([]releasePayload, error) {
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=100", m.Repository)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "CalDen/"+m.CurrentVersion)
	resp, err := m.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub release lookup returned HTTP %d", resp.StatusCode)
	}
	var releases []releasePayload
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

func (m *Manager) releaseByVersion(ctx context.Context, version string) (*releasePayload, error) {
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/v%s", m.Repository, version)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "CalDen/"+m.CurrentVersion)
	resp, err := m.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release v%s was not found", version)
	}
	var release releasePayload
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&release); err != nil {
		return nil, err
	}
	if release.Draft {
		return nil, errors.New("draft releases cannot be installed")
	}
	return &release, nil
}

func latestEligible(releases []releasePayload, channel string) *releasePayload {
	var eligible []releasePayload
	for _, release := range releases {
		if release.Draft || !eligibleForChannel(&release, channel) {
			continue
		}
		version := strings.TrimPrefix(release.TagName, "v")
		if !versionPattern.MatchString(version) {
			continue
		}
		eligible = append(eligible, release)
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		return compareVersions(strings.TrimPrefix(eligible[i].TagName, "v"), strings.TrimPrefix(eligible[j].TagName, "v")) > 0
	})
	if len(eligible) == 0 {
		return nil
	}
	return &eligible[0]
}

func eligibleForChannel(release *releasePayload, preference string) bool {
	channel := releaseChannel(strings.TrimPrefix(release.TagName, "v"))
	return channelRank(channel) <= channelRank(normalizeChannel(preference))
}

func releaseChannel(version string) string {
	lower := strings.ToLower(version)
	if i := strings.Index(lower, "-"); i >= 0 {
		pre := lower[i+1:]
		switch {
		case strings.HasPrefix(pre, "alpha"):
			return "alpha"
		case strings.HasPrefix(pre, "beta"):
			return "beta"
		case strings.HasPrefix(pre, "rc"):
			return "rc"
		default:
			return "preview"
		}
	}
	return "stable"
}

func defaultChannelForVersion(version string) string {
	return releaseChannel(version)
}

func normalizeChannel(channel string) string {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "stable", "rc", "beta", "alpha", "preview":
		return strings.ToLower(strings.TrimSpace(channel))
	default:
		return ""
	}
}

func channelRank(channel string) int {
	switch channel {
	case "stable":
		return 0
	case "rc":
		return 1
	case "beta":
		return 2
	case "alpha":
		return 3
	default:
		return 4
	}
}

type parsedVersion struct {
	major, minor, patch int
	pre                 []string
}

func parseVersion(v string) (parsedVersion, bool) {
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	parts := strings.SplitN(v, "+", 2)
	core := parts[0]
	var pre []string
	if p := strings.SplitN(core, "-", 2); len(p) == 2 {
		core = p[0]
		pre = strings.Split(p[1], ".")
	}
	n := strings.Split(core, ".")
	if len(n) != 3 {
		return parsedVersion{}, false
	}
	major, e1 := strconv.Atoi(n[0])
	minor, e2 := strconv.Atoi(n[1])
	patch, e3 := strconv.Atoi(n[2])
	if e1 != nil || e2 != nil || e3 != nil {
		return parsedVersion{}, false
	}
	return parsedVersion{major: major, minor: minor, patch: patch, pre: pre}, true
}

func compareVersions(a, b string) int {
	left, ok1 := parseVersion(a)
	right, ok2 := parseVersion(b)
	if !ok1 || !ok2 {
		return strings.Compare(a, b)
	}
	if left.major != right.major {
		return cmpInt(left.major, right.major)
	}
	if left.minor != right.minor {
		return cmpInt(left.minor, right.minor)
	}
	if left.patch != right.patch {
		return cmpInt(left.patch, right.patch)
	}
	if len(left.pre) == 0 && len(right.pre) == 0 {
		return 0
	}
	if len(left.pre) == 0 {
		return 1
	}
	if len(right.pre) == 0 {
		return -1
	}
	max := len(left.pre)
	if len(right.pre) > max {
		max = len(right.pre)
	}
	for i := 0; i < max; i++ {
		if i >= len(left.pre) {
			return -1
		}
		if i >= len(right.pre) {
			return 1
		}
		li, le := strconv.Atoi(left.pre[i])
		ri, re := strconv.Atoi(right.pre[i])
		switch {
		case le == nil && re == nil && li != ri:
			return cmpInt(li, ri)
		case le == nil && re != nil:
			return -1
		case le != nil && re == nil:
			return 1
		case left.pre[i] != right.pre[i]:
			return strings.Compare(left.pre[i], right.pre[i])
		}
	}
	return 0
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func runtimeAssetName(goos, goarch string) (string, error) {
	if goos != "linux" {
		return "", fmt.Errorf("unsupported operating system %s", goos)
	}
	switch goarch {
	case "amd64", "arm64":
		return fmt.Sprintf("calden-linux-%s.tar.gz", goarch), nil
	default:
		return "", fmt.Errorf("unsupported architecture %s", goarch)
	}
}

func releaseAssets(release releasePayload, assetName string) (string, string, error) {
	var assetURL, checksumURL string
	for _, asset := range release.Assets {
		switch asset.Name {
		case assetName:
			assetURL = asset.BrowserDownloadURL
		case "SHA256SUMS":
			checksumURL = asset.BrowserDownloadURL
		}
	}
	if assetURL == "" || checksumURL == "" {
		return "", "", errors.New("release is missing the runtime bundle or SHA256SUMS")
	}
	return assetURL, checksumURL, nil
}

func (m *Manager) downloadFile(url, path string, start, end int) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "CalDen/"+m.CurrentVersion)
	resp, err := m.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if resp.ContentLength <= 0 {
		if _, err := io.Copy(file, resp.Body); err != nil {
			return err
		}
		m.progress("downloading", "Downloading update", "Downloading release bundle", end)
		return file.Sync()
	}
	buf := make([]byte, 128*1024)
	var written int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := file.Write(buf[:n]); err != nil {
				return err
			}
			written += int64(n)
			fraction := float64(written) / float64(resp.ContentLength)
			m.progress("downloading", "Downloading update", "Downloading release bundle", start+int(fraction*float64(end-start)))
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	return file.Sync()
}

func (m *Manager) downloadText(url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "CalDen/"+m.CurrentVersion)
	resp, err := m.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return string(body), err
}

func verifyChecksum(path, content, expectedName string) error {
	var expected string
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if filepath.Base(strings.TrimPrefix(fields[len(fields)-1], "*")) == expectedName {
			expected = strings.ToLower(fields[0])
			break
		}
	}
	if len(expected) != 64 {
		return errors.New("SHA256SUMS does not contain the release bundle")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != expected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}

func extractBundle(path, destination string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	root := filepath.Clean(destination) + string(os.PathSeparator)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(destination, header.Name)
		clean := filepath.Clean(target)
		if !strings.HasPrefix(clean+string(os.PathSeparator), root) {
			return fmt.Errorf("release contains invalid path %q", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(clean, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(clean), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(clean, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode)&0777)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("unsupported release entry %q", header.Name)
		}
	}
	return nil
}

func verifyRuntime(binary, version string) error {
	output, err := exec.Command(binary, "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("runtime verification failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if !strings.Contains(string(output), "Version: "+version) {
		return fmt.Errorf("runtime reported an unexpected version: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *Manager) client() *http.Client {
	if m.HTTPClient != nil {
		return m.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

func (m *Manager) readStatus() (Status, error) {
	status := Status{Ready: true, State: "idle"}
	body, err := os.ReadFile(m.StatusFile)
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return Status{}, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return status, nil
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return Status{}, err
	}
	return status, nil
}

func (m *Manager) writeStatus(status Status) error {
	if err := os.MkdirAll(filepath.Dir(m.StatusFile), 0700); err != nil {
		return err
	}
	body, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return atomicWrite(m.StatusFile, body, 0600)
}

func (m *Manager) progress(state, step, message string, progress int) {
	status, err := m.readStatus()
	if err != nil {
		return
	}
	if progress < status.Progress {
		progress = status.Progress
	}
	if progress > 100 {
		progress = 100
	}
	if step != "" && step != status.Step {
		status.Activity = append(status.Activity, Activity{Timestamp: time.Now().UTC(), Message: step})
		if len(status.Activity) > 60 {
			status.Activity = append([]Activity(nil), status.Activity[len(status.Activity)-60:]...)
		}
	}
	status.Ready = true
	status.State = state
	status.Step = step
	status.Message = message
	status.Progress = progress
	_ = m.writeStatus(status)
}

func (m *Manager) fail(version string, started time.Time, message string, err error) {
	status, readErr := m.readStatus()
	if readErr != nil {
		status = Status{Ready: true, Version: version, StartedAt: &started}
	}
	finished := time.Now().UTC()
	status.State = "failed"
	status.Step = "Update stopped"
	status.Message = message
	status.FinishedAt = &finished
	status.Activity = append(status.Activity, Activity{Timestamp: finished, Message: "Update stopped: " + err.Error()})
	_ = m.writeStatus(status)
}

func (m *Manager) finalizeRestart() {
	status, err := m.readStatus()
	if err != nil || status.State != "restarting" {
		return
	}
	if status.Version != "" && compareVersions(m.CurrentVersion, status.Version) < 0 {
		return
	}
	finished := time.Now().UTC()
	status.Ready = true
	status.State = "completed"
	status.Step = "Update complete"
	status.Message = "CalDen is running the selected version."
	status.Progress = 100
	status.FinishedAt = &finished
	status.Activity = append(status.Activity, Activity{Timestamp: finished, Message: "CalDen restarted successfully"})
	_ = m.writeStatus(status)
}

func activeState(state string) bool {
	switch state {
	case "backup", "preparing", "downloading", "verifying", "installing", "restarting":
		return true
	default:
		return false
	}
}

func atomicWrite(path string, body []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, body, mode); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			out = append(out, entry)
		}
	}
	return append(out, prefix+value)
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func safeName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
			return r
		}
		return '_'
	}, value)
	if value == "" {
		return "unknown"
	}
	return value
}
