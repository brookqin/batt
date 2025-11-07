package gui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	githubAPIURL = "https://api.github.com/repos/charlie0129/batt/releases/latest"
)

const (
	updateCheckInterval = 24 * time.Hour // Check for updates daily
)

// GitHubRelease represents the latest release information from GitHub API
type GitHubRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
	Body        string    `json:"body"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
		ContentType        string `json:"content_type"`
	} `json:"assets"`
}

// UpdateInfo contains information about available updates
type UpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
	HasUpdate      bool
	ReleaseURL     string
	DownloadURL    string
	DownloadSize   int64
	ReleaseNotes   string
	PublishedAt    time.Time
}

// UpdateChecker handles automatic update checking
type UpdateChecker struct {
	currentVersion string
	httpClient     *http.Client
	lastCheck      time.Time
	lastUpdateInfo *UpdateInfo
}

// NewUpdateChecker creates a new update checker
func NewUpdateChecker(currentVersion string) *UpdateChecker {
	transport := NewProxyAwareTransport()

	return &UpdateChecker{
		currentVersion: currentVersion,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

// CheckForUpdate checks if a new version is available
func (uc *UpdateChecker) CheckForUpdate() (*UpdateInfo, error) {
	logrus.Debug("Checking for updates...")

	release, err := uc.fetchLatestRelease()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest release: %w", err)
	}

	// Skip prerelease and draft versions
	if release.Prerelease || release.Draft {
		logrus.Debug("Latest release is prerelease or draft, skipping")
		return nil, nil
	}

	updateInfo := &UpdateInfo{
		CurrentVersion: uc.currentVersion,
		LatestVersion:  release.TagName,
		ReleaseURL:     release.HTMLURL,
		ReleaseNotes:   release.Body,
		PublishedAt:    release.PublishedAt,
	}

	// Find the appropriate download asset for macOS DMG
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.Name, ".dmg") && strings.Contains(asset.Name, "batt") {
			updateInfo.DownloadURL = asset.BrowserDownloadURL
			updateInfo.DownloadSize = asset.Size
			break
		}
	}

	// Compare versions
	updateInfo.HasUpdate = uc.isNewerVersion(release.TagName, uc.currentVersion)

	uc.lastCheck = time.Now()
	uc.lastUpdateInfo = updateInfo

	logrus.WithFields(logrus.Fields{
		"current_version": uc.currentVersion,
		"latest_version":  release.TagName,
		"has_update":      updateInfo.HasUpdate,
	}).Debug("Update check completed")

	return updateInfo, nil
}

// fetchLatestRelease fetches the latest release information from GitHub API
func (uc *UpdateChecker) fetchLatestRelease() (*GitHubRelease, error) {
	req, err := http.NewRequest("GET", githubAPIURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set user agent to avoid rate limiting issues
	req.Header.Set("User-Agent", "batt-update-checker")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := uc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &release, nil
}

// isNewerVersion compares two version strings
// Returns true if latestVersion is newer than currentVersion
func (uc *UpdateChecker) isNewerVersion(latestVersion, currentVersion string) bool {
	// Remove 'v' prefix if present
	latest := strings.TrimPrefix(latestVersion, "v")
	current := strings.TrimPrefix(currentVersion, "v")

	// Simple version comparison - could be enhanced with semver library
	// For now, just compare strings directly
	return latest != current && latest > current
}

// ShouldCheckUpdate returns true if enough time has passed since last check
func (uc *UpdateChecker) ShouldCheckUpdate() bool {
	return time.Since(uc.lastCheck) > updateCheckInterval
}

// GetLastUpdateInfo returns the cached update information
func (uc *UpdateChecker) GetLastUpdateInfo() *UpdateInfo {
	return uc.lastUpdateInfo
}
