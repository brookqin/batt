package gui

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	pkgerrors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// UpdateDownloader handles downloading and installing updates from DMG files
type UpdateDownloader struct {
	httpClient *http.Client
}

// NewUpdateDownloader creates a new update downloader
func NewUpdateDownloader() *UpdateDownloader {
	transport := NewProxyAwareTransport()

	return &UpdateDownloader{
		httpClient: &http.Client{
			Timeout:   300 * time.Second, // 5 minutes timeout for downloads
			Transport: transport,
		},
	}
}

// DownloadAndInstall downloads and installs an update from DMG
func (ud *UpdateDownloader) DownloadAndInstall(updateInfo *UpdateInfo) error {
	if updateInfo.DownloadURL == "" {
		return fmt.Errorf("no download URL available for update")
	}

	logrus.WithFields(logrus.Fields{
		"version": updateInfo.LatestVersion,
		"url":     updateInfo.DownloadURL,
		"size":    updateInfo.DownloadSize,
	}).Info("Downloading DMG update")

	// Create temporary directory for download
	tempDir, err := os.MkdirTemp(os.TempDir(), "batt-update-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Download the DMG file
	dmgPath := filepath.Join(tempDir, fmt.Sprintf("batt-%s.dmg", updateInfo.LatestVersion))
	if err := ud.downloadFile(updateInfo.DownloadURL, dmgPath); err != nil {
		return fmt.Errorf("failed to download DMG: %w", err)
	}

	logrus.Info("DMG download completed, mounting and installing...")

	// Mount the DMG and install the update
	if err := ud.installFromDMG(dmgPath); err != nil {
		return fmt.Errorf("failed to install from DMG: %w", err)
	}

	logrus.Info("Update installed successfully from DMG")
	return nil
}

// downloadFile downloads a file from the given URL
func (ud *UpdateDownloader) downloadFile(url, filepath string) error {
	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Get the data
	resp, err := ud.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Create a progress reader if content length is available
	var reader io.Reader = resp.Body
	if resp.ContentLength > 0 {
		reader = &progressReader{
			reader:      resp.Body,
			total:       resp.ContentLength,
			downloaded:  0,
			lastPercent: 0,
		}
	}

	// Write the body to file
	_, err = io.Copy(out, reader)
	if err != nil {
		return fmt.Errorf("failed to save file: %w", err)
	}

	return nil
}

// progressReader provides download progress tracking
type progressReader struct {
	reader      io.Reader
	total       int64
	downloaded  int64
	lastPercent int64
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.downloaded += int64(n)

	if pr.total > 0 {
		percent := (pr.downloaded * 100) / pr.total
		if percent > pr.lastPercent && percent%10 == 0 {
			logrus.WithField("percent", percent).Info("Download progress")
			pr.lastPercent = percent
		}
	}

	return n, err
}

// installFromDMG mounts the DMG and installs the app
func (ud *UpdateDownloader) installFromDMG(dmgPath string) error {
	logrus.Info("Mounting DMG file")

	// Mount the DMG
	mountPoint, err := ud.mountDMG(dmgPath)
	if err != nil {
		return fmt.Errorf("failed to mount DMG: %w", err)
	}
	defer ud.unmountDMG(mountPoint)

	logrus.WithField("mount_point", mountPoint).Info("DMG mounted successfully")

	// Find the app bundle in the mounted DMG
	appPath := filepath.Join(mountPoint, "batt.app")
	if _, err := os.Stat(appPath); err != nil {
		return fmt.Errorf("batt.app not found in DMG: %w", err)
	}

	// Get current app path
	currentAppPath, err := ud.getCurrentAppPath()
	if err != nil {
		return fmt.Errorf("failed to get current app path: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"current_app": currentAppPath,
		"new_app":     appPath,
	}).Info("Installing update")

	// Copy the new app bundle to Applications
	if err := ud.installAppBundle(appPath, currentAppPath); err != nil {
		return fmt.Errorf("failed to install app bundle: %w", err)
	}

	// Restart the application
	if err := ud.restartApplication(currentAppPath); err != nil {
		logrus.WithError(err).Warn("Failed to restart application automatically")
		// Don't fail the update if restart doesn't work - user can restart manually
	}

	return nil
}

// mountDMG mounts a DMG file and returns the mount point
func (ud *UpdateDownloader) mountDMG(dmgPath string) (string, error) {
	// Use hdiutil to mount the DMG
	cmd := exec.Command("hdiutil", "attach", "-nobrowse", "-noverify", dmgPath)
	output, err := cmd.Output()
	if err != nil {
		return "", pkgerrors.Wrapf(err, "failed to mount DMG: %s", output)
	}

	// Parse the mount point from hdiutil output
	lines := bytes.Split(output, []byte("\n"))
	for _, line := range lines {
		if bytes.Contains(line, []byte("/Volumes/")) {
			fields := bytes.Fields(line)
			for _, field := range fields {
				if bytes.HasPrefix(field, []byte("/Volumes/")) {
					return string(field), nil
				}
			}
		}
	}

	return "", fmt.Errorf("could not find mount point in hdiutil output")
}

// unmountDMG unmounts a DMG file
func (ud *UpdateDownloader) unmountDMG(mountPoint string) error {
	cmd := exec.Command("hdiutil", "detach", mountPoint)
	if err := cmd.Run(); err != nil {
		logrus.WithError(err).Warnf("Failed to unmount DMG at %s", mountPoint)
		return err
	}
	return nil
}

// getCurrentAppPath returns the path to the current batt.app
func (ud *UpdateDownloader) getCurrentAppPath() (string, error) {
	// Get the current executable path
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	// Navigate up to find the .app bundle
	appPath := execPath
	for i := 0; i < 10; i++ { // Prevent infinite loop
		if strings.HasSuffix(appPath, ".app") {
			return appPath, nil
		}
		parent := filepath.Dir(appPath)
		if parent == appPath {
			break // Reached root
		}
		appPath = parent
	}

	// If not found in path hierarchy, try common locations
	commonPaths := []string{
		"/Applications/batt.app",
		filepath.Join(os.Getenv("HOME"), "Applications", "batt.app"),
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("could not find batt.app in path hierarchy or common locations")
}

// installAppBundle copies the new app bundle without AppleScript or admin privileges
func (ud *UpdateDownloader) installAppBundle(sourceApp, targetApp string) error {
	logrus.WithFields(logrus.Fields{
		"source": sourceApp,
		"target": targetApp,
	}).Info("Installing app bundle using direct file operations")

	// 1. 首先尝试直接复制（适用于无sandbox环境）
	if err := ud.copyAppBundleDirectly(sourceApp, targetApp); err != nil {
		logrus.WithError(err).Warn("Direct copy failed, trying alternative methods")

		// 2. 备选方案：复制到临时位置然后移动
		return ud.copyAppBundleWithTemp(sourceApp, targetApp)
	}

	return nil
}

// restartApplication quits the current app and launches the new version
func (ud *UpdateDownloader) restartApplication(appPath string) error {
	logrus.Info("Restarting application with new version")

	// 启动新版本（在后台执行，避免阻塞退出过程）
	go func() {
		// 等待一小段时间确保安装完成
		time.Sleep(2 * time.Second)

		logrus.Info("Launching new version of batt")

		// 使用open命令启动新应用
		cmd := exec.Command("open", appPath)
		if err := cmd.Start(); err != nil {
			logrus.WithError(err).Error("Failed to launch new version")
			return
		}

		// 退出当前应用
		logrus.Info("Exiting current version")
		os.Exit(0)
	}()

	return nil
}

// copyAppBundleDirectly 直接复制应用包
func (ud *UpdateDownloader) copyAppBundleDirectly(sourceApp, targetApp string) error {
	logrus.Info("Attempting direct copy of app bundle")

	// 备份原应用
	backupPath := targetApp + ".backup"
	if err := os.Rename(targetApp, backupPath); err != nil {
		logrus.WithError(err).Warn("Failed to backup existing app, trying to remove instead")
		// 如果重命名失败，尝试删除
		if err := os.RemoveAll(targetApp); err != nil {
			return fmt.Errorf("failed to remove existing app: %w", err)
		}
	} else {
		// 备份成功，延迟删除备份
		defer func() {
			// 延迟清理备份文件
			time.Sleep(5 * time.Second)
			if err := os.RemoveAll(backupPath); err != nil {
				logrus.WithError(err).Warn("Failed to clean up backup")
			}
		}()
	}

	// 复制新应用
	if err := copyDir(sourceApp, targetApp); err != nil {
		logrus.WithError(err).Error("Failed to copy app bundle")
		// 如果复制失败，尝试恢复备份
		if _, statErr := os.Stat(backupPath); statErr == nil {
			if restoreErr := os.Rename(backupPath, targetApp); restoreErr != nil {
				logrus.WithError(restoreErr).Error("Failed to restore backup")
			}
		}
		return fmt.Errorf("failed to copy app bundle: %w", err)
	}

	logrus.Info("Direct copy completed successfully")
	return nil
}

// copyAppBundleWithTemp 使用临时位置的备选方案
func (ud *UpdateDownloader) copyAppBundleWithTemp(sourceApp, targetApp string) error {
	logrus.Info("Using temp location copy method")

	tempDir, err := os.MkdirTemp("", "batt-install-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// 复制到临时位置
	tempApp := filepath.Join(tempDir, "batt.app")
	if err := copyDir(sourceApp, tempApp); err != nil {
		return fmt.Errorf("failed to copy to temp location: %w", err)
	}

	// 重命名原应用
	backupPath := targetApp + ".backup"
	if err := os.Rename(targetApp, backupPath); err != nil {
		return fmt.Errorf("failed to backup existing app: %w", err)
	}

	// 移动新应用到目标位置
	if err := os.Rename(tempApp, targetApp); err != nil {
		// 恢复备份
		os.Rename(backupPath, targetApp)
		return fmt.Errorf("failed to move new app: %w", err)
	}

	// 延迟清理备份
	go func() {
		time.Sleep(5 * time.Second)
		if err := os.RemoveAll(backupPath); err != nil {
			logrus.WithError(err).Warn("Failed to clean up backup")
		}
	}()

	logrus.Info("Temp location copy completed successfully")
	return nil
}

// copyDir 递归复制目录
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 计算目标路径
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			// 创建目录
			return os.MkdirAll(dstPath, info.Mode())
		} else {
			// 复制文件
			return copyFile(path, dstPath, info.Mode())
		}
	})
}

// copyFile 复制单个文件
func copyFile(src, dst string, mode os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// IsUpdateAvailable checks if an update is available for the current platform
func IsUpdateAvailable(updateInfo *UpdateInfo) bool {
	if updateInfo == nil || !updateInfo.HasUpdate {
		return false
	}

	// Check if we have a download URL for DMG
	if updateInfo.DownloadURL == "" {
		logrus.Warn("No DMG download URL available for update")
		return false
	}

	return true
}

// GetDownloadSize returns human-readable download size
func (ud *UpdateDownloader) GetDownloadSize(updateInfo *UpdateInfo) string {
	if updateInfo.DownloadSize <= 0 {
		return "Unknown size"
	}

	size := float64(updateInfo.DownloadSize)
	units := []string{"B", "KB", "MB", "GB"}
	unitIndex := 0

	for size >= 1024 && unitIndex < len(units)-1 {
		size /= 1024
		unitIndex++
	}

	return fmt.Sprintf("%.1f %s", size, units[unitIndex])
}

// BackupBinary creates a backup of the current binary
func BackupBinary(source, backup string) error {
	input, err := os.ReadFile(source)
	if err != nil {
		return err
	}

	return os.WriteFile(backup, input, 0755)
}

// RestoreBackup restores the backup binary
func RestoreBackup(backup, target string) error {
	input, err := os.ReadFile(backup)
	if err != nil {
		return err
	}

	return os.WriteFile(target, input, 0755)
}
