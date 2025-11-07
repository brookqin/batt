package gui

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// UpdateScheduler manages automatic update checking
type UpdateScheduler struct {
	checker       *UpdateChecker
	ticker        *time.Ticker
	stopChan      chan struct{}
	mu            sync.Mutex
	onUpdateFound func(*UpdateInfo) // Callback when update is found
}

// NewUpdateScheduler creates a new update scheduler
func NewUpdateScheduler(currentVersion string, onUpdateFound func(*UpdateInfo)) *UpdateScheduler {
	return &UpdateScheduler{
		checker:       NewUpdateChecker(currentVersion),
		onUpdateFound: onUpdateFound,
		stopChan:      make(chan struct{}),
	}
}

// Start begins automatic update checking
func (us *UpdateScheduler) Start() {
	us.mu.Lock()
	defer us.mu.Unlock()

	if us.ticker != nil {
		logrus.Warn("Update scheduler already started")
		return
	}

	logrus.Info("Starting automatic update scheduler")

	// Check immediately on startup
	go us.checkForUpdate()

	// Set up daily ticker
	us.ticker = time.NewTicker(24 * time.Hour)

	// Run update checks in a goroutine with proper synchronization
	go func() {
		// Get a local copy of the ticker and stop channel to avoid race conditions
		us.mu.Lock()
		ticker := us.ticker
		stopChan := us.stopChan
		us.mu.Unlock()

		// Ensure ticker is not nil before using it
		if ticker == nil {
			logrus.Error("Ticker is nil in update scheduler goroutine")
			return
		}

		for {
			select {
			case <-ticker.C:
				us.checkForUpdate()
			case <-stopChan:
				return
			}
		}
	}()
}

// Stop stops the update scheduler
func (us *UpdateScheduler) Stop() {
	us.mu.Lock()
	defer us.mu.Unlock()

	if us.ticker == nil {
		return
	}

	logrus.Info("Stopping automatic update scheduler")

	us.ticker.Stop()
	close(us.stopChan)
	us.ticker = nil
	us.stopChan = make(chan struct{})
}

// checkForUpdate performs the actual update check
func (us *UpdateScheduler) checkForUpdate() {
	us.mu.Lock()
	checker := us.checker
	onUpdateFound := us.onUpdateFound
	us.mu.Unlock()

	logrus.Debug("Running scheduled update check")

	updateInfo, err := checker.CheckForUpdate()
	if err != nil {
		logrus.WithError(err).Error("Failed to check for updates")
		return
	}

	if updateInfo != nil && updateInfo.HasUpdate {
		logrus.WithFields(logrus.Fields{
			"current_version": updateInfo.CurrentVersion,
			"latest_version":  updateInfo.LatestVersion,
		}).Info("Update available")

		// Notify the callback if provided
		if onUpdateFound != nil {
			onUpdateFound(updateInfo)
		}
	} else {
		logrus.Debug("No updates available")
	}
}

// GetCurrentVersion returns the current version being checked against
func (us *UpdateScheduler) GetCurrentVersion() string {
	return us.checker.currentVersion
}

// GetLastUpdateInfo returns the last update information
func (us *UpdateScheduler) GetLastUpdateInfo() *UpdateInfo {
	return us.checker.GetLastUpdateInfo()
}

// CheckNow performs an immediate update check
func (us *UpdateScheduler) CheckNow() (*UpdateInfo, error) {
	return us.checker.CheckForUpdate()
}
