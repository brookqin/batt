package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUpdateChecker_isNewerVersion(t *testing.T) {
	tests := []struct {
		name           string
		latestVersion  string
		currentVersion string
		expected       bool
	}{
		{
			name:           "newer version available",
			latestVersion:  "v1.1.0",
			currentVersion: "v1.0.0",
			expected:       true,
		},
		{
			name:           "same version",
			latestVersion:  "v1.0.0",
			currentVersion: "v1.0.0",
			expected:       false,
		},
		{
			name:           "older version",
			latestVersion:  "v1.0.0",
			currentVersion: "v1.1.0",
			expected:       false,
		},
		{
			name:           "version without v prefix",
			latestVersion:  "1.1.0",
			currentVersion: "1.0.0",
			expected:       true,
		},
		{
			name:           "complex version numbers",
			latestVersion:  "v1.2.3",
			currentVersion: "v1.2.2",
			expected:       true,
		},
	}

	uc := NewUpdateChecker("v1.0.0")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := uc.isNewerVersion(tt.latestVersion, tt.currentVersion)
			if result != tt.expected {
				t.Errorf("isNewerVersion(%s, %s) = %v, want %v",
					tt.latestVersion, tt.currentVersion, result, tt.expected)
			}
		})
	}
}

func TestUpdateChecker_CheckForUpdate(t *testing.T) {
	// Create a mock server that returns a GitHub release
	mockRelease := GitHubRelease{
		TagName:     "v1.1.0",
		Name:        "batt v1.1.0",
		PublishedAt: time.Now(),
		HTMLURL:     "https://github.com/charlie0129/batt/releases/tag/v1.1.0",
		Body:        "Bug fixes and improvements",
		Prerelease:  false,
		Draft:       false,
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
			ContentType        string `json:"content_type"`
		}{
			{
				Name:               "batt-v1.1.0.dmg",
				BrowserDownloadURL: "https://example.com/batt-v1.1.0.dmg",
				Size:               1024 * 1024 * 30, // 30MB
				ContentType:        "application/x-apple-diskimage",
			},
			{
				Name:               "batt-v1.1.0-darwin-arm64.tar.gz",
				BrowserDownloadURL: "https://example.com/batt-v1.1.0-darwin-arm64.tar.gz",
				Size:               1024 * 1024 * 15, // 15MB
				ContentType:        "application/gzip",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/charlie0129/batt/releases/latest" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockRelease)
	}))
	defer server.Close()

	// Temporarily override the API URL for testing
	originalURL := githubAPIURL
	githubAPIURL = server.URL + "/repos/charlie0129/batt/releases/latest"
	defer func() {
		githubAPIURL = originalURL
	}()

	uc := NewUpdateChecker("v1.0.0")
	updateInfo, err := uc.CheckForUpdate()
	if err != nil {
		t.Fatalf("CheckForUpdate failed: %v", err)
	}

	if updateInfo == nil {
		t.Fatal("expected update info, got nil")
	}

	if !updateInfo.HasUpdate {
		t.Error("expected HasUpdate to be true")
	}

	if updateInfo.LatestVersion != "v1.1.0" {
		t.Errorf("expected LatestVersion to be v1.1.0, got %s", updateInfo.LatestVersion)
	}

	if updateInfo.CurrentVersion != "v1.0.0" {
		t.Errorf("expected CurrentVersion to be v1.0.0, got %s", updateInfo.CurrentVersion)
	}

	if updateInfo.DownloadURL != "https://example.com/batt-v1.1.0.dmg" {
		t.Errorf("expected DMG download URL, got %s", updateInfo.DownloadURL)
	}

	if updateInfo.DownloadSize != 1024*1024*30 {
		t.Errorf("expected download size 30MB, got %d", updateInfo.DownloadSize)
	}
}

func TestUpdateChecker_CheckForUpdate_Prerelease(t *testing.T) {
	// Create a mock server that returns a prerelease
	mockRelease := GitHubRelease{
		TagName:     "v1.1.0-beta.1",
		Name:        "batt v1.1.0-beta.1",
		PublishedAt: time.Now(),
		HTMLURL:     "https://github.com/charlie0129/batt/releases/tag/v1.1.0-beta.1",
		Body:        "Beta release",
		Prerelease:  true,
		Draft:       false,
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
			ContentType        string `json:"content_type"`
		}{
			{
				Name:               "batt-v1.1.0-beta.1.dmg",
				BrowserDownloadURL: "https://example.com/batt-v1.1.0-beta.1.dmg",
				Size:               1024 * 1024 * 30, // 30MB
				ContentType:        "application/x-apple-diskimage",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockRelease)
	}))
	defer server.Close()

	// Temporarily override the API URL for testing
	originalURL := githubAPIURL
	githubAPIURL = server.URL + "/repos/charlie0129/batt/releases/latest"
	defer func() {
		githubAPIURL = originalURL
	}()

	uc := NewUpdateChecker("v1.0.0")
	updateInfo, err := uc.CheckForUpdate()
	if err != nil {
		t.Fatalf("CheckForUpdate failed: %v", err)
	}

	// Should return nil for prerelease versions
	if updateInfo != nil {
		t.Error("expected nil for prerelease version, got update info")
	}
}

func TestUpdateChecker_CheckForUpdate_NoDMG(t *testing.T) {
	// Create a mock server that returns a release without DMG
	mockRelease := GitHubRelease{
		TagName:     "v1.1.0",
		Name:        "batt v1.1.0",
		PublishedAt: time.Now(),
		HTMLURL:     "https://github.com/charlie0129/batt/releases/tag/v1.1.0",
		Body:        "Bug fixes and improvements",
		Prerelease:  false,
		Draft:       false,
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
			ContentType        string `json:"content_type"`
		}{
			{
				Name:               "batt-v1.1.0-windows.exe",
				BrowserDownloadURL: "https://example.com/batt-v1.1.0-windows.exe",
				Size:               1024 * 1024 * 30, // 30MB
				ContentType:        "application/x-msdownload",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockRelease)
	}))
	defer server.Close()

	// Temporarily override the API URL for testing
	originalURL := githubAPIURL
	githubAPIURL = server.URL + "/repos/charlie0129/batt/releases/latest"
	defer func() {
		githubAPIURL = originalURL
	}()

	uc := NewUpdateChecker("v1.0.0")
	updateInfo, err := uc.CheckForUpdate()
	if err != nil {
		t.Fatalf("CheckForUpdate failed: %v", err)
	}

	if updateInfo == nil {
		t.Fatal("expected update info, got nil")
	}

	// Should still report update available but without download URL
	if !updateInfo.HasUpdate {
		t.Error("expected HasUpdate to be true")
	}

	if updateInfo.DownloadURL != "" {
		t.Error("expected empty download URL when no DMG is available")
	}
}

func TestUpdateChecker_CheckForUpdate_APIError(t *testing.T) {
	// Create a mock server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	// Temporarily override the API URL for testing
	originalURL := githubAPIURL
	githubAPIURL = server.URL + "/repos/charlie0129/batt/releases/latest"
	defer func() {
		githubAPIURL = originalURL
	}()

	uc := NewUpdateChecker("v1.0.0")
	_, err := uc.CheckForUpdate()
	if err == nil {
		t.Error("expected error for API failure, got nil")
	}
}

func TestUpdateChecker_ShouldCheckUpdate(t *testing.T) {
	uc := NewUpdateChecker("v1.0.0")

	// Initially should check (never checked before)
	if !uc.ShouldCheckUpdate() {
		t.Error("expected ShouldCheckUpdate to be true for first check")
	}

	// Set last check to now
	uc.lastCheck = time.Now()
	if uc.ShouldCheckUpdate() {
		t.Error("expected ShouldCheckUpdate to be false immediately after checking")
	}

	// Set last check to 25 hours ago
	uc.lastCheck = time.Now().Add(-25 * time.Hour)
	if !uc.ShouldCheckUpdate() {
		t.Error("expected ShouldCheckUpdate to be true after 25 hours")
	}
}

func TestUpdateChecker_GetLastUpdateInfo(t *testing.T) {
	uc := NewUpdateChecker("v1.0.0")

	// Initially should be nil
	if uc.GetLastUpdateInfo() != nil {
		t.Error("expected GetLastUpdateInfo to return nil initially")
	}

	// Set some update info
	updateInfo := &UpdateInfo{
		CurrentVersion: "v1.0.0",
		LatestVersion:  "v1.1.0",
		HasUpdate:      true,
	}
	uc.lastUpdateInfo = updateInfo

	retrieved := uc.GetLastUpdateInfo()
	if retrieved != updateInfo {
		t.Error("expected GetLastUpdateInfo to return the set update info")
	}
}
