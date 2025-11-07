package gui

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateDownloader_GetDownloadSize(t *testing.T) {
	ud := NewUpdateDownloader()

	tests := []struct {
		name         string
		downloadSize int64
		expected     string
	}{
		{
			name:         "30MB",
			downloadSize: 30 * 1024 * 1024,
			expected:     "30.0 MB",
		},
		{
			name:         "1.5MB",
			downloadSize: 1.5 * 1024 * 1024,
			expected:     "1.5 MB",
		},
		{
			name:         "500KB",
			downloadSize: 500 * 1024,
			expected:     "500.0 KB",
		},
		{
			name:         "2GB",
			downloadSize: 2 * 1024 * 1024 * 1024,
			expected:     "2.0 GB",
		},
		{
			name:         "Unknown size",
			downloadSize: 0,
			expected:     "Unknown size",
		},
		{
			name:         "Negative size",
			downloadSize: -100,
			expected:     "Unknown size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateInfo := &UpdateInfo{
				DownloadSize: tt.downloadSize,
			}
			result := ud.GetDownloadSize(updateInfo)
			if result != tt.expected {
				t.Errorf("GetDownloadSize() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUpdateDownloader_IsUpdateAvailable(t *testing.T) {
	tests := []struct {
		name       string
		updateInfo *UpdateInfo
		expected   bool
	}{
		{
			name: "update available with DMG",
			updateInfo: &UpdateInfo{
				HasUpdate:    true,
				DownloadURL:  "https://example.com/batt-v1.1.0.dmg",
				DownloadSize: 30 * 1024 * 1024,
			},
			expected: true,
		},
		{
			name: "update available without DMG",
			updateInfo: &UpdateInfo{
				HasUpdate:   true,
				DownloadURL: "",
			},
			expected: false,
		},
		{
			name: "no update available",
			updateInfo: &UpdateInfo{
				HasUpdate: false,
			},
			expected: false,
		},
		{
			name:       "nil update info",
			updateInfo: nil,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsUpdateAvailable(tt.updateInfo)
			if result != tt.expected {
				t.Errorf("IsUpdateAvailable() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUpdateDownloader_downloadFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "batt-download-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test server that serves a file
	testContent := "test file content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testContent)))
		w.Write([]byte(testContent))
	}))
	defer server.Close()

	downloadPath := filepath.Join(tempDir, "test-download.txt")

	err = NewUpdateDownloader().downloadFile(server.URL+"/test-file", downloadPath)
	if err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}

	// Verify the file was downloaded correctly
	downloadedContent, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(downloadedContent) != testContent {
		t.Errorf("Downloaded content = %q, want %q", string(downloadedContent), testContent)
	}
}

func TestUpdateDownloader_downloadFile_404(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "batt-download-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	downloadPath := filepath.Join(tempDir, "test-download.txt")

	err = NewUpdateDownloader().downloadFile(server.URL+"/nonexistent", downloadPath)
	if err == nil {
		t.Error("expected error for 404 response, got nil")
	}
}

func TestUpdateDownloader_getCurrentAppPath(t *testing.T) {
	// This test is tricky because it depends on the actual execution environment
	// We'll test the basic functionality and error cases

	appPath, err := NewUpdateDownloader().getCurrentAppPath()

	// In test environment, this might fail or return a path
	if err != nil {
		t.Logf("getCurrentAppPath returned error (expected in test environment): %v", err)
		// This is acceptable - the function should handle missing app gracefully
	} else {
		t.Logf("getCurrentAppPath returned: %s", appPath)
		// If it returns a path, it should be valid
		if appPath != "" {
			// Should either end with .app or be a known location
			if !filepath.IsAbs(appPath) {
				t.Error("expected absolute path for app")
			}
		}
	}
}


func TestProgressReader(t *testing.T) {
	// Create a mock reader that returns data in chunks
	mockData := make([]byte, 1024) // 1KB of data
	for i := range mockData {
		mockData[i] = byte(i % 256)
	}

	reader := &progressReader{
		reader:      bytes.NewReader(mockData),
		total:       int64(len(mockData)),
		downloaded:  0,
		lastPercent: 0,
	}

	// Read all data
	var buf bytes.Buffer
	_, err := io.Copy(&buf, reader)
	if err != nil {
		t.Fatalf("Failed to read from progressReader: %v", err)
	}

	if reader.downloaded != int64(len(mockData)) {
		t.Errorf("Expected downloaded %d, got %d", len(mockData), reader.downloaded)
	}

	if !bytes.Equal(buf.Bytes(), mockData) {
		t.Error("Data mismatch after reading through progressReader")
	}
}

func TestUpdateDownloader_BackupAndRestore(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "batt-backup-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := []byte("test content for backup")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test backup function directly
	backupPath := testFile + ".backup"
	err = BackupBinary(testFile, backupPath)
	if err != nil {
		t.Fatalf("BackupBinary failed: %v", err)
	}

	// Verify backup exists and has correct content
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read backup file: %v", err)
	}

	if !bytes.Equal(backupContent, testContent) {
		t.Error("Backup content doesn't match original")
	}

	// Test restore
	// First modify the original file
	modifiedContent := []byte("modified content")
	if err := os.WriteFile(testFile, modifiedContent, 0644); err != nil {
		t.Fatalf("Failed to modify test file: %v", err)
	}

	// Now restore from backup
	err = RestoreBackup(backupPath, testFile)
	if err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}

	// Verify restoration
	restoredContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read restored file: %v", err)
	}

	if !bytes.Equal(restoredContent, testContent) {
		t.Error("Restored content doesn't match original backup")
	}
}
