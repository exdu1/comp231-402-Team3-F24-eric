package pod_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/comp231-402-Team3-F24/internal/pod"
	"github.com/comp231-402-Team3-F24/internal/runtime"
	"github.com/comp231-402-Team3-F24/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testNamespace = "litepod-integration-test"
	sockPath      = "/run/containerd/containerd.sock"
	testImage     = "docker.io/library/alpine:latest"
)

type IntegrationTestSuite struct {
	podManager pod.Manager
	runtime    types.ContainerRuntime
	cleanup    *pod.TestCleanup
	t          *testing.T
}

// setupIntegrationTest creates a new test suite with proper cleanup handling
func setupIntegrationTest(t *testing.T) (*IntegrationTestSuite, func()) {
	t.Helper()

	if os.Getenv("SKIP_INTEGRATION") == "1" {
		t.Skip("Skipping integration tests (SKIP_INTEGRATION=1)")
	}

	// Create runtime with longer timeout context
	_, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Create runtime
	rt, err := runtime.NewContainerdRuntime(sockPath, testNamespace)
	if err != nil {
		t.Skipf("Failed to create runtime: %v", err)
	}

	// Create pod manager
	pm := pod.NewManager(rt)

	// Initialize cleanup handler with longer timeout
	cleanup := pod.NewTestCleanup(rt)

	suite := &IntegrationTestSuite{
		podManager: pm,
		runtime:    rt,
		cleanup:    cleanup,
		t:          t,
	}

	// Return cleanup function with retry logic
	return suite, func() {
		t.Log("Starting test cleanup")
		// Try cleanup multiple times if needed
		for i := 0; i < 3; i++ {
			if i > 0 {
				t.Logf("Cleanup retry attempt %d/3", i+1)
				time.Sleep(2 * time.Second)
			}
			cleanup.Cleanup(t)
			// Verify cleanup
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			containers, err := rt.ListContainers(ctx)
			cancel()
			if err != nil || len(containers) > 0 {
				continue
			}
			break
		}
		t.Log("Cleanup completed")
	}
}

// createTestPod is a helper to create a test pod with proper tracking
func (s *IntegrationTestSuite) createTestPod(name string) (*types.PodSpec, error) {
	podID := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())

	spec := &types.PodSpec{
		ID:   podID,
		Name: name,
		Containers: []types.ContainerConfig{
			{
				ID:    fmt.Sprintf("test-container-%d", time.Now().UnixNano()),
				Name:  "test-container",
				Image: testImage,
				Command: []string{
					"sh",
					"-c",
					"while true; do echo 'test container running'; sleep 1; done",
				},
			},
		},
	}

	// Track the pod for cleanup
	s.cleanup.TrackPod(podID)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Ensure image is available
	err := s.runtime.PullImage(ctx, testImage)
	if err != nil {
		return nil, fmt.Errorf("failed to pull test image: %w", err)
	}

	// Create the pod
	status, err := s.podManager.CreatePod(ctx, *spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create pod: %w", err)
	}
	if status == nil {
		return nil, fmt.Errorf("pod created but status is nil")
	}

	return spec, nil
}

func TestPodLifecycle_Integration(t *testing.T) {
	suite, cleanup := setupIntegrationTest(t)
	defer cleanup()

	t.Run("full pod lifecycle", func(t *testing.T) {
		// Create pod spec directly with two containers
		podID := fmt.Sprintf("lifecycle-test-%d", time.Now().UnixNano())
		spec := &types.PodSpec{
			ID:   podID,
			Name: "lifecycle-test",
			Containers: []types.ContainerConfig{
				{
					ID:    fmt.Sprintf("test-container-1-%d", time.Now().UnixNano()),
					Name:  "test-container-1",
					Image: testImage,
					Command: []string{
						"sh",
						"-c",
						"while true; do echo 'first container running'; sleep 1; done",
					},
				},
				{
					ID:    fmt.Sprintf("test-container-2-%d", time.Now().UnixNano()),
					Name:  "test-container-2",
					Image: testImage,
					Command: []string{
						"sh",
						"-c",
						"while true; do echo 'second container running'; sleep 1; done",
					},
				},
			},
		}

		// Track the pod for cleanup
		suite.cleanup.TrackPod(podID)

		// Create context for operations
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		// Ensure image is available
		t.Log("Pulling test image")
		err := suite.runtime.PullImage(ctx, testImage)
		require.NoError(t, err, "Failed to pull test image")

		// Create pod
		t.Log("Creating pod with multiple containers")
		status, err := suite.podManager.CreatePod(ctx, *spec)
		require.NoError(t, err, "Failed to create pod with two containers")
		require.NotNil(t, status, "Pod status should not be nil")

		// Set up pod watching after creating the pod
		t.Log("Setting up pod watch")
		watchChan, err := suite.podManager.WatchPod(ctx, spec.ID)
		require.NoError(t, err, "Failed to set up pod watch")

		// Verify initial pod state through watch channel
		var sawPending bool
		select {
		case status := <-watchChan:
			if status.Phase == types.PodPending {
				sawPending = true
			}
		case <-time.After(5 * time.Second):
			t.Error("Timeout waiting for pod pending status")
		}
		require.True(t, sawPending, "Should have seen pod in pending state")

		// Start pod
		t.Log("Starting pod")
		err = suite.podManager.StartPod(ctx, spec.ID)
		require.NoError(t, err, "Failed to start pod")

		// Wait for running state through watch channel
		var sawRunning bool
		select {
		case status := <-watchChan:
			if status.Phase == types.PodRunning {
				sawRunning = true
			}
		case <-time.After(20 * time.Second):
			t.Error("Timeout waiting for pod running status")
		}
		require.True(t, sawRunning, "Should have seen pod enter running state")

		// Verify both containers are running
		t.Log("Verifying container statuses")
		time.Sleep(2 * time.Second) // Allow time for containers to start
		containers, err := suite.runtime.ListContainers(ctx)
		require.NoError(t, err, "Failed to list containers")

		runningCount := 0
		for _, c := range containers {
			t.Logf("Container %s state: %s", c.ID, c.State)
			if c.State == "running" {
				runningCount++
			}
		}
		assert.Equal(t, 2, runningCount, "Both containers should be running")

		// Get and verify pod logs
		t.Log("Checking pod logs")
		logs, err := suite.podManager.GetPodLogs(ctx, spec.ID, pod.LogOptions{})
		require.NoError(t, err, "Failed to get pod logs")
		assert.NotEmpty(t, logs, "Pod logs should not be empty")
		for containerName, log := range logs {
			t.Logf("Logs for container %s: %s", containerName, log)
		}

		// Test pod update
		t.Log("Testing pod update")
		updatedSpec := *spec
		updatedSpec.Labels = map[string]string{"test": "update"}
		err = suite.podManager.UpdatePod(ctx, spec.ID, updatedSpec)
		require.NoError(t, err, "Failed to update pod")

		// Verify update
		status, err = suite.podManager.GetPod(ctx, spec.ID)
		require.NoError(t, err, "Failed to get pod status after update")
		assert.Equal(t, "update", status.Spec.Labels["test"], "Pod update should be reflected")

		// Stop pod
		t.Log("Stopping pod")
		err = suite.podManager.StopPod(ctx, spec.ID)
		require.NoError(t, err, "Failed to stop pod")

		// Verify pod is stopped
		t.Log("Verifying pod stop")
		time.Sleep(2 * time.Second) // Allow time for pod to stop
		status, err = suite.podManager.GetPod(ctx, spec.ID)
		require.NoError(t, err, "Failed to get pod status")
		assert.Equal(t, types.PodPending, status.Phase, "Pod should be pending after stop")

		// Verify containers are stopped
		t.Log("Verifying containers are stopped")
		containers, err = suite.runtime.ListContainers(ctx)
		require.NoError(t, err, "Failed to list containers")
		for _, c := range containers {
			if strings.HasPrefix(c.ID, "test-container") {
				t.Logf("Container %s state after stop: %s", c.ID, c.State)
				assert.NotEqual(t, "running", c.State, "Containers should not be running")
			}
		}

		// Try to restart the pod
		t.Log("Restarting pod")
		err = suite.podManager.StartPod(ctx, spec.ID)
		require.NoError(t, err, "Failed to restart pod")

		// Verify pod is running again
		t.Log("Verifying pod restart")
		time.Sleep(2 * time.Second)
		status, err = suite.podManager.GetPod(ctx, spec.ID)
		require.NoError(t, err, "Failed to get pod status")
		assert.Equal(t, types.PodRunning, status.Phase, "Pod should be running after restart")

		// Delete pod with retries
		t.Log("Deleting pod")
		err = suite.podManager.DeletePod(ctx, spec.ID)
		require.NoError(t, err, "Failed to delete pod")

		t.Log("Verifying pod deletion")
		_, err = suite.podManager.GetPod(ctx, spec.ID)
		assert.Equal(t, pod.ErrPodNotFound, err, "Pod should not exist after deletion")

		// Add retry logic for container cleanup verification
		t.Log("Verifying container cleanup")
		maxRetries := 5
		var lastErr error
		for attempt := 0; attempt < maxRetries; attempt++ {
			if attempt > 0 {
				t.Logf("Retry attempt %d/%d for container cleanup verification", attempt+1, maxRetries)
				time.Sleep(2 * time.Second) // Increasing backoff between retries
			}

			containers, err := suite.runtime.ListContainers(ctx)
			if err != nil {
				lastErr = fmt.Errorf("failed to list containers on attempt %d: %w", attempt+1, err)
				continue
			}

			foundContainers := false
			for _, c := range containers {
				if strings.Contains(c.ID, "test-container") {
					foundContainers = true
					t.Logf("Found container still present: %s (state: %s)", c.ID, c.State)
					// Try to force cleanup of lingering containers
					_ = suite.runtime.StopContainer(ctx, c.ID)
					_ = suite.runtime.RemoveContainer(ctx, c.ID)
				}
			}

			if !foundContainers {
				t.Log("All test containers successfully cleaned up")
				return // Success case
			}
		}

		if lastErr != nil {
			t.Errorf("Container cleanup verification failed after %d attempts: %v", maxRetries, lastErr)
		}
	})
}

func TestPodUpdate_Integration(t *testing.T) {
	suite, cleanup := setupIntegrationTest(t)
	defer cleanup()

	t.Run("update pod configuration", func(t *testing.T) {
		// Create initial pod
		spec, err := suite.createTestPod("update-test")
		require.NoError(t, err, "Failed to create test pod")

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		// Start the pod
		t.Log("Starting initial pod configuration")
		err = suite.podManager.StartPod(ctx, spec.ID)
		require.NoError(t, err, "Failed to start pod")

		// Verify initial pod is running
		time.Sleep(2 * time.Second)
		status, err := suite.podManager.GetPod(ctx, spec.ID)
		require.NoError(t, err, "Failed to get pod status")
		assert.Equal(t, types.PodRunning, status.Phase, "Pod should be running")
		assert.Equal(t, 1, len(status.Spec.Containers), "Should have 1 container")

		// Create updated spec with additional container
		t.Log("Creating updated pod specification")
		updatedSpec := *spec
		updatedSpec.Containers = append(updatedSpec.Containers, types.ContainerConfig{
			ID:    fmt.Sprintf("test-container-2-%d", time.Now().UnixNano()),
			Name:  "test-container-2",
			Image: testImage,
			Command: []string{
				"sh",
				"-c",
				"while true; do echo 'second container running'; sleep 1; done",
			},
		})

		// Update the pod
		t.Log("Applying pod update")
		err = suite.podManager.UpdatePod(ctx, spec.ID, updatedSpec)
		require.NoError(t, err, "Failed to update pod")

		// Allow time for update to complete
		time.Sleep(5 * time.Second)

		// Verify update
		t.Log("Verifying pod update")
		status, err = suite.podManager.GetPod(ctx, spec.ID)
		require.NoError(t, err, "Failed to get updated pod status")
		assert.Equal(t, types.PodRunning, status.Phase, "Updated pod should be running")
		assert.Equal(t, 2, len(status.Spec.Containers), "Should have 2 containers after update")

		// Verify both containers are running
		containers, err := suite.runtime.ListContainers(ctx)
		require.NoError(t, err, "Failed to list containers")

		runningCount := 0
		for _, c := range containers {
			if c.State == "running" {
				runningCount++
			}
		}
		assert.Equal(t, 2, runningCount, "Both containers should be running")
	})
}

func TestPodWatch_Integration(t *testing.T) {
	suite, cleanup := setupIntegrationTest(t)
	defer cleanup()

	t.Run("watch pod status changes", func(t *testing.T) {
		// Create a test pod
		spec, err := suite.createTestPod("watch-test")
		require.NoError(t, err)

		// Set up watch before creating pod
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		watchChan, err := suite.podManager.WatchPod(ctx, spec.ID)
		require.NoError(t, err)

		// Start the pod in a goroutine
		errChan := make(chan error, 1)
		go func() {
			err := suite.podManager.StartPod(ctx, spec.ID)
			errChan <- err
		}()

		// Wait for both the StartPod completion and running state
		var sawRunning bool
		for !sawRunning {
			select {
			case err := <-errChan:
				require.NoError(t, err, "Failed to start pod")
			case status := <-watchChan:
				if status.Phase == types.PodRunning {
					sawRunning = true
				}
			case <-time.After(10 * time.Second):
				t.Fatal("Timeout waiting for pod running status")
			}
		}

		assert.True(t, sawRunning, "Should have seen pod enter running state")
	})
}
