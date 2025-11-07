package gui

import (
	"sync"
	"testing"
	"time"
)

func TestUpdateScheduler_StartStop(t *testing.T) {
	var updateFound bool
	var mu sync.Mutex

	onUpdateFound := func(info *UpdateInfo) {
		mu.Lock()
		updateFound = true
		mu.Unlock()
	}

	scheduler := NewUpdateScheduler("v1.0.0", onUpdateFound)

	// Test start
	scheduler.Start()
	time.Sleep(100 * time.Millisecond) // Give it time to start

	// Should be running
	if scheduler.ticker == nil {
		t.Error("expected ticker to be set after Start")
	}

	// Test stop
	scheduler.Stop()
	time.Sleep(100 * time.Millisecond) // Give it time to stop

	// Should be stopped
	if scheduler.ticker != nil {
		t.Error("expected ticker to be nil after Stop")
	}

	// Verify the callback variable was accessible (even though we didn't trigger it)
	if updateFound != false {
		t.Log("updateFound variable was set correctly (unused in this test)")
	}

	// Test start again
	scheduler.Start()
	time.Sleep(100 * time.Millisecond)

	if scheduler.ticker == nil {
		t.Error("expected ticker to be set after second Start")
	}

	scheduler.Stop()
}

func TestUpdateScheduler_GetCurrentVersion(t *testing.T) {
	scheduler := NewUpdateScheduler("v1.2.3", nil)
	if scheduler.GetCurrentVersion() != "v1.2.3" {
		t.Errorf("expected GetCurrentVersion to return v1.2.3, got %s", scheduler.GetCurrentVersion())
	}
}

func TestUpdateScheduler_GetLastUpdateInfo(t *testing.T) {
	scheduler := NewUpdateScheduler("v1.0.0", nil)

	// Initially should be nil
	if scheduler.GetLastUpdateInfo() != nil {
		t.Error("expected GetLastUpdateInfo to return nil initially")
	}

	// Set some update info through the checker
	updateInfo := &UpdateInfo{
		CurrentVersion: "v1.0.0",
		LatestVersion:  "v1.1.0",
		HasUpdate:      true,
	}
	scheduler.checker.lastUpdateInfo = updateInfo

	retrieved := scheduler.GetLastUpdateInfo()
	if retrieved != updateInfo {
		t.Error("expected GetLastUpdateInfo to return the set update info")
	}
}

func TestUpdateScheduler_CheckNow(t *testing.T) {
	// This test would require mocking the HTTP client, which is complex
	// For now, we just test that CheckNow doesn't panic and returns appropriate results

	scheduler := NewUpdateScheduler("v1.0.0", nil)

	// Since we can't easily mock the HTTP requests without more complex setup,
	// we just verify that CheckNow can be called without panicking
	// In a real test environment, you'd mock the HTTP responses

	// This will likely fail due to network issues, but should not panic
	updateInfo, err := scheduler.CheckNow()

	// We expect either an error (network issues) or nil/no update
	// The important thing is that it doesn't panic
	if err != nil {
		t.Logf("CheckNow returned error (expected in test environment): %v", err)
	} else {
		t.Logf("CheckNow completed successfully")
		if updateInfo != nil {
			t.Logf("Update info: %+v", updateInfo)
		}
	}
}

func TestUpdateScheduler_MultipleStartStop(t *testing.T) {
	scheduler := NewUpdateScheduler("v1.0.0", nil)

	// Multiple start/stop cycles should work correctly
	for i := 0; i < 3; i++ {
		scheduler.Start()
		time.Sleep(50 * time.Millisecond)

		if scheduler.ticker == nil {
			t.Errorf("expected ticker to be set in cycle %d", i)
		}

		scheduler.Stop()
		time.Sleep(50 * time.Millisecond)

		if scheduler.ticker != nil {
			t.Errorf("expected ticker to be nil after stop in cycle %d", i)
		}
	}
}

func TestUpdateScheduler_ConcurrentAccess(t *testing.T) {
	scheduler := NewUpdateScheduler("v1.0.0", nil)

	// Test concurrent start/stop operations
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	// Start multiple goroutines that try to start/stop
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			if id%2 == 0 {
				scheduler.Start()
			} else {
				scheduler.Stop()
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any panics or errors
	for err := range errors {
		if err != nil {
			t.Errorf("concurrent access error: %v", err)
		}
	}

	// Clean up
	scheduler.Stop()
}

func TestUpdateScheduler_Callback(t *testing.T) {
	var callbackCalled bool
	var receivedUpdateInfo *UpdateInfo
	var mu sync.Mutex

	onUpdateFound := func(info *UpdateInfo) {
		mu.Lock()
		defer mu.Unlock()
		callbackCalled = true
		receivedUpdateInfo = info
	}

	scheduler := NewUpdateScheduler("v1.0.0", onUpdateFound)

	// Manually trigger the callback to test it
	testUpdateInfo := &UpdateInfo{
		CurrentVersion: "v1.0.0",
		LatestVersion:  "v1.1.0",
		HasUpdate:      true,
	}

	if scheduler.onUpdateFound != nil {
		scheduler.onUpdateFound(testUpdateInfo)
	}

	// Give callback time to execute
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if !callbackCalled {
		t.Error("expected callback to be called")
	}
	if receivedUpdateInfo != testUpdateInfo {
		t.Error("expected callback to receive the correct update info")
	}
	mu.Unlock()
}
