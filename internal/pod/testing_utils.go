package pod

import (
	"context"
	"fmt"
	"io"
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

	log.Printf("Pods to cleanup: %v\n", tc.pods)
	for podID := range tc.pods {
		log.Printf("Attempting to cleanup pod: %s\n", podID)
	}

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
	log.Printf("Cleanup completed\n")
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

// MockRuntime implements the ContainerRuntime interface for testing
type MockRuntime struct {
	containers         map[string]types.ContainerStatus
	skipContainerID    string
	stayInStartingState bool
	failStartContainer  bool
	failListContainers  bool
	mu                 sync.RWMutex
}

func (m *MockRuntime) ListContainers(ctx context.Context) ([]types.ContainerStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if m.failListContainers {
		return nil, fmt.Errorf("simulated list containers failure")
	}

	containers := make([]types.ContainerStatus, 0, len(m.containers))
	for _, container := range m.containers {
		if container.ID != m.skipContainerID {
			containers = append(containers, container)
		}
	}

	return containers, nil
}

func (m *MockRuntime) StartContainer(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	container, exists := m.containers[id]
	if !exists {
		return fmt.Errorf("container %s not found", id)
	}

	if m.failStartContainer {
		return fmt.Errorf("simulated start container failure")
	}

	// Simulate a very brief startup delay
	time.Sleep(time.Millisecond)

	if m.stayInStartingState {
		container.State = "starting"
	} else {
		container.State = "running"
	}
	m.containers[id] = container
	return nil
}

func (m *MockRuntime) CreateContainer(ctx context.Context, config types.ContainerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check if container already exists
	if _, exists := m.containers[config.ID]; exists {
		return fmt.Errorf("container %s already exists", config.ID)
	}

	m.containers[config.ID] = types.ContainerStatus{
		ID:    config.ID,
		Name:  config.Name,
		State: "created",
	}
	return nil
}

func (m *MockRuntime) StopContainer(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	container, exists := m.containers[id]
	if !exists {
		return fmt.Errorf("container %s not found", id)
	}

	// Only stop if container is running
	if container.State != "running" && container.State != "starting" {
		return fmt.Errorf("container %s is not running (current state: %s)", id, container.State)
	}

	// Simulate a very brief stop delay
	time.Sleep(time.Millisecond)

	container.State = "stopped"
	m.containers[id] = container
	return nil
}

func (m *MockRuntime) RemoveContainer(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	container, exists := m.containers[id]
	if !exists {
		return fmt.Errorf("container %s not found", id)
	}

	// Only remove if container is stopped
	if container.State != "stopped" {
		// Force stop the container first
		container.State = "stopped"
		m.containers[id] = container
	}

	delete(m.containers, id)
	return nil
}

func (m *MockRuntime) DeleteContainer(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if _, exists := m.containers[id]; !exists {
		return fmt.Errorf("container %s not found", id)
	}

	delete(m.containers, id)
	return nil
}

func (m *MockRuntime) ContainerStats(ctx context.Context, id string) (*types.ContainerStats, error) {
	return nil, nil
}

func (m *MockRuntime) PullImage(ctx context.Context, ref string) error {
	return nil
}

func (m *MockRuntime) ImportImage(ctx context.Context, imageName string, reader io.Reader) error {
	return nil
}

func (m *MockRuntime) ListImages(ctx context.Context) ([]types.ImageInfo, error) {
	return nil, nil
}

func (m *MockRuntime) Close() error {
	return nil
}
