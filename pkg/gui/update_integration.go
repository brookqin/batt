package gui

import (
	"fmt"

	"github.com/progrium/darwinkit/dispatch"
	"github.com/progrium/darwinkit/macos/appkit"
	"github.com/progrium/darwinkit/objc"
	"github.com/sirupsen/logrus"
)

// UpdateMenuController manages update-related menu items
type UpdateMenuController struct {
	scheduler         *UpdateScheduler
	downloader        *UpdateDownloader
	updateMenuItem    appkit.MenuItem
	hasUpdate         bool
	currentUpdateInfo *UpdateInfo
}

// NewUpdateMenuController creates a new update menu controller
func NewUpdateMenuController(currentVersion string) *UpdateMenuController {
	controller := &UpdateMenuController{
		downloader: NewUpdateDownloader(),
		hasUpdate:  false,
	}

	// Create update scheduler with callback
	controller.scheduler = NewUpdateScheduler(currentVersion, controller.onUpdateFound)

	return controller
}

// Start begins automatic update checking
func (umc *UpdateMenuController) Start() {
	logrus.Info("Starting update checker")
	umc.scheduler.Start()
}

// Stop stops the update checker
func (umc *UpdateMenuController) Stop() {
	logrus.Info("Stopping update checker")
	umc.scheduler.Stop()
}

// CreateMenuItems creates the update-related menu items
func (umc *UpdateMenuController) CreateMenuItems(menu appkit.Menu) {
	// Add separator before update items
	menu.AddItem(appkit.MenuItem_SeparatorItem())

	// Create main update menu item (no darwinkit Go closure bridging)
	umc.updateMenuItem = appkit.NewMenuItemWithAction("Check for Updates", "", func(sender objc.Object) {
		umc.handleCheckForUpdates()
	})
	umc.updateMenuItem.SetToolTip("Check for available updates to batt")
	menu.AddItem(umc.updateMenuItem)
}

// onUpdateFound is called when an update is found
func (umc *UpdateMenuController) onUpdateFound(updateInfo *UpdateInfo) {
	logrus.WithFields(logrus.Fields{
		"latest_version":  updateInfo.LatestVersion,
		"current_version": updateInfo.CurrentVersion,
	}).Info("Update found, updating menu")

	umc.hasUpdate = true
	umc.currentUpdateInfo = updateInfo

	// Get download size for display
	downloadSize := umc.downloader.GetDownloadSize(updateInfo)

	// Always dispatch UI updates to main thread for thread safety
	dispatch.MainQueue().DispatchAsync(func() {
		if umc.updateMenuItem.IsNil() {
			logrus.Error("updateMenuItem is nil, cannot update UI")
			return
		}

		// Update main menu item to show update available
		title := fmt.Sprintf("🔄 Update Available: %s", updateInfo.LatestVersion)
		if downloadSize != "Unknown size" {
			title = fmt.Sprintf("🔄 Update Available: %s (%s)", updateInfo.LatestVersion, downloadSize)
		}
		umc.updateMenuItem.SetTitle(title)
		umc.updateMenuItem.SetToolTip(fmt.Sprintf("A new version %s is available. Click to update.", updateInfo.LatestVersion))
	})

	// Show notification
	umc.showUpdateNotification(updateInfo)
}

// handleCheckForUpdates handles the main update menu item click
func (umc *UpdateMenuController) handleCheckForUpdates() {
	if umc.hasUpdate && umc.currentUpdateInfo != nil {
		umc.handleDownloadAndInstall()
	} else {
		umc.handleCheckNow()
	}
}

// handleCheckNow handles immediate update check
func (umc *UpdateMenuController) handleCheckNow() {
	logrus.Info("Manual update check requested")

	// Show checking status - dispatch to main thread for safety
	dispatch.MainQueue().DispatchAsync(func() {
		umc.updateMenuItem.SetTitle("Checking for Updates...")
		umc.updateMenuItem.SetEnabled(false)
	})

	// Check for updates in background
	go func() {
		updateInfo, err := umc.scheduler.CheckNow()

		// Dispatch UI updates to main thread for thread safety
		dispatch.MainQueue().DispatchAsync(func() {
			umc.updateMenuItem.SetEnabled(true)

			if err != nil {
				logrus.WithError(err).Error("Failed to check for updates")
				showAlert("Update Check Failed", fmt.Sprintf("Failed to check for updates: %v", err))
				umc.updateMenuItem.SetTitle("Check for Updates")
				return
			}

			if updateInfo != nil && updateInfo.HasUpdate {
				umc.onUpdateFound(updateInfo)
			} else {
				umc.updateMenuItem.SetTitle("Check for Updates")
				showAlert("No Updates Available", "You are running the latest version of batt.")
			}
		})
	}()
}

// handleDownloadAndInstall handles the download and install action
func (umc *UpdateMenuController) handleDownloadAndInstall() {
	if umc.currentUpdateInfo == nil {
		return
	}

	// Get download size for display
	downloadSize := umc.downloader.GetDownloadSize(umc.currentUpdateInfo)

	// Show confirmation dialog
	alert := appkit.NewAlert()
	alert.SetIcon(appkit.Image_ImageWithSystemSymbolNameAccessibilityDescription("arrow.down.circle", "s"))
	alert.SetAlertStyle(appkit.AlertStyleInformational)
	alert.SetMessageText("Install Update?")

	infoText := fmt.Sprintf("Version %s is available. Would you like to download and install it now?", umc.currentUpdateInfo.LatestVersion)
	if downloadSize != "Unknown size" {
		infoText = fmt.Sprintf("Version %s (%s) is available. Would you like to download and install it now?\n\nThis will download the update, install it, and restart the application.", umc.currentUpdateInfo.LatestVersion, downloadSize)
	}

	alert.SetInformativeText(infoText)
	alert.AddButtonWithTitle("Install")
	alert.AddButtonWithTitle("Cancel")

	response := alert.RunModal()
	if response != appkit.AlertFirstButtonReturn {
		return
	}

	// Update UI to show downloading
	umc.updateMenuItem.SetTitle("Downloading...")
	umc.updateMenuItem.SetEnabled(false)

	// Download and install in background
	go func() {
		err := umc.downloader.DownloadAndInstall(umc.currentUpdateInfo)

		// Dispatch UI updates to main thread using GCD
		dispatch.MainQueue().DispatchAsync(func() {
			// Reset update state
			umc.hasUpdate = false

			// Update UI items
			umc.updateMenuItem.SetEnabled(true)
			umc.updateMenuItem.SetTitle("Check for Updates")

			if err != nil {
				logrus.WithError(err).Error("Failed to install update")
				showAlert("Update Failed", fmt.Sprintf("Failed to install update: %v", err))
			} else {
				showAlert("Update Successful", fmt.Sprintf("Successfully updated to version %s. Please restart batt to complete the update.", umc.currentUpdateInfo.LatestVersion))
				umc.currentUpdateInfo = nil
			}
		})
	}()
}

// showUpdateNotification shows a notification about the available update
func (umc *UpdateMenuController) showUpdateNotification(updateInfo *UpdateInfo) {
	// For now, just log it - notification center integration would go here
	logrus.WithFields(logrus.Fields{
		"version":     updateInfo.LatestVersion,
		"release_url": updateInfo.ReleaseURL,
	}).Info("Update notification would be shown")

	// In a real implementation, this would use NSUserNotificationCenter
	// or the newer UserNotifications framework
}
