package pod

import (
	"context"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/comp231-402-Team3-F24/pkg/types"
)

// TestCleanup handles graceful cleanup of test resources
type TestCleanup struct {
	runtime types.ContainerRuntime
	mu      sync.Mutex
	pods    map[string]bool
}

// NewTestCleanup creates a new test cleanup handler
func NewTestCleanup(runtime types.ContainerRuntime) *TestCleanup {
	return &TestCleanup{
		runtime: runtime,
		pods:    make(map[string]bool),
	}
}

// TrackPod adds a pod to be cleaned up
func (tc *TestCleanup) TrackPod(podID string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.pods[podID] = true
}

// Cleanup performs thorough cleanup with retries
func (tc *TestCleanup) Cleanup(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// First, list all containers
	containers, err := tc.runtime.ListContainers(ctx)
	if err != nil {
		t.Logf("Failed to list containers during cleanup: %v", err)
		return
	}

	// Create wait group for parallel cleanup
	var wg sync.WaitGroup
	var cleanupErrors []string
	var errorsMu sync.Mutex

	// Clean up each container
	for _, container := range containers {
		wg.Add(1)
		go func(containerID string) {
			defer wg.Done()
			if err := tc.cleanupContainerWithRetry(ctx, containerID); err != nil {
				errorsMu.Lock()
				cleanupErrors = append(cleanupErrors, fmt.Sprintf("container %s: %v", containerID, err))
				errorsMu.Unlock()
			}
		}(container.ID)
	}

	// Wait for all cleanup operations with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("Initial cleanup completed")
	case <-time.After(30 * time.Second):
		t.Log("Cleanup timed out, proceeding with verification")
	}

	// Final verification with forced cleanup
	if err := tc.verifyAndForceCleanup(ctx); err != nil {
		t.Logf("Final cleanup verification failed: %v", err)
	}

	// Report any errors that occurred during cleanup
	if len(cleanupErrors) > 0 {
		t.Logf("Cleanup completed with %d errors:", len(cleanupErrors))
		for _, err := range cleanupErrors {
			t.Logf("  - %s", err)
		}
	} else {
		t.Log("Cleanup completed successfully")
	}
}

// cleanupContainerWithRetry attempts to clean up a container with retries
func (tc *TestCleanup) cleanupContainerWithRetry(ctx context.Context, containerID string) error {
	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// First try to stop the container
		err := tc.runtime.StopContainer(ctx, containerID)
		if err != nil {
			log.Printf("Attempt %d: Failed to stop container %s: %v", attempt+1, containerID, err)
			lastErr = err
			time.Sleep(time.Second * time.Duration(attempt+1))
			continue
		}

		// Wait for container to stop
		time.Sleep(time.Second)

		// Then try to remove it
		err = tc.runtime.RemoveContainer(ctx, containerID)
		if err != nil {
			log.Printf("Attempt %d: Failed to remove container %s: %v", attempt+1, containerID, err)
			lastErr = err
			time.Sleep(time.Second * time.Duration(attempt+1))
			continue
		}

		return nil
	}

	return fmt.Errorf("failed after %d attempts, last error: %v", maxRetries, lastErr)
}

// verifyAndForceCleanup ensures all containers are removed, using force if necessary
func (tc *TestCleanup) verifyAndForceCleanup(ctx context.Context) error {
	containers, err := tc.runtime.ListContainers(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify cleanup: %w", err)
	}

	if len(containers) == 0 {
		return nil
	}

	// Force cleanup of remaining containers
	var finalErrors []string
	for _, container := range containers {
		// Force stop
		_ = tc.runtime.StopContainer(ctx, container.ID)
		time.Sleep(time.Second)

		// Force remove
		if err := tc.runtime.RemoveContainer(ctx, container.ID); err != nil {
			finalErrors = append(finalErrors, fmt.Sprintf("failed to force remove container %s: %v", container.ID, err))
		}
	}

	if len(finalErrors) > 0 {
		return fmt.Errorf("force cleanup failed: %v", finalErrors)
	}

	return nil
}
