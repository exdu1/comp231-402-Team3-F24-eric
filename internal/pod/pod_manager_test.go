package pod

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/comp231-402-Team3-F24/pkg/types"
	"github.com/stretchr/testify/require"
)

const (
	testPodID       = "test-pod-1"
	testContainerID = "test-pod-1-test-container"
)

// Helper function to create a test pod spec
func createTestPod(id string) types.PodSpec {
	return types.PodSpec{
		ID:   id,
		Name: "test-pod",
		Containers: []types.ContainerConfig{
			{
				ID:    fmt.Sprintf("%s-%s", id, "test-container"),
				Name:  "test-container",
				Image: "test-image",
			},
		},
	}
}

// Helper function to create a test pod manager with mock runtime
func createTestPodManager(t *testing.T) (Manager, *MockRuntime) {
	runtime := &MockRuntime{
		containers: make(map[string]types.ContainerStatus),
	}
	manager := NewManager(runtime)
	return manager, runtime
}

// TestCreatePod verifies pod creation functionality
func TestCreatePod(t *testing.T) {
	tests := []struct {
		name    string
		spec    types.PodSpec
		wantErr bool
	}{
		{
			name: "valid pod spec",
			spec: types.PodSpec{
				ID:   testPodID,
				Name: "test-pod",
				Containers: []types.ContainerConfig{
					{
						ID:    testContainerID,
						Name:  "test-container",
						Image: "test-image",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid pod spec - no containers",
			spec: types.PodSpec{
				ID:   testPodID,
				Name: "test-pod",
			},
			wantErr: true,
		},
		{
			name: "invalid pod spec - empty ID",
			spec: types.PodSpec{
				Name: "test-pod",
				Containers: []types.ContainerConfig{
					{
						ID:    testContainerID,
						Name:  "test-container",
						Image: "test-image",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, _ := createTestPodManager(t)
			_, err := manager.CreatePod(context.Background(), tt.spec)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestDeletePod verifies pod deletion functionality
func TestDeletePod(t *testing.T) {
	manager, _ := createTestPodManager(t)
	pod := createTestPod("test-pod-1")

	// Create pod
	_, err := manager.CreatePod(context.Background(), pod)
	require.NoError(t, err)

	// Delete pod
	err = manager.DeletePod(context.Background(), pod.ID)
	require.NoError(t, err)

	// Verify pod is deleted
	_, err = manager.GetPod(context.Background(), pod.ID)
	require.Error(t, err)
}

// TestStartStopPod verifies pod lifecycle management
func TestStartStopPod(t *testing.T) {
	t.Run("successful_start_and_stop", func(t *testing.T) {
		manager, _ := createTestPodManager(t)
		pod := createTestPod("test-pod-1")
		_, err := manager.CreatePod(context.Background(), pod)
		require.NoError(t, err)

		err = manager.StartPod(context.Background(), pod.ID)
		require.NoError(t, err)

		err = manager.StopPod(context.Background(), pod.ID)
		require.NoError(t, err)
	})

	t.Run("container_start_failure", func(t *testing.T) {
		manager, runtime := createTestPodManager(t)
		runtime.failStartContainer = true

		pod := createTestPod("test-pod-1")
		_, err := manager.CreatePod(context.Background(), pod)
		require.NoError(t, err)

		err = manager.StartPod(context.Background(), pod.ID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "simulated start container failure")
	})

	t.Run("container_never_reaches_running_state", func(t *testing.T) {
		manager, runtime := createTestPodManager(t)
		runtime.stayInStartingState = true

		pod := createTestPod("test-pod-1")
		_, err := manager.CreatePod(context.Background(), pod)
		require.NoError(t, err)

		err = manager.StartPod(context.Background(), pod.ID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Timeout waiting for containers to start")
	})

	t.Run("container_list_failure", func(t *testing.T) {
		manager, runtime := createTestPodManager(t)
		runtime.failListContainers = true

		pod := createTestPod("test-pod-1")
		_, err := manager.CreatePod(context.Background(), pod)
		require.NoError(t, err)

		err = manager.StartPod(context.Background(), pod.ID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to list containers: simulated list containers failure")
	})

	t.Run("container_missing_in_list", func(t *testing.T) {
		manager, runtime := createTestPodManager(t)
		pod := createTestPod("test-pod-1")
		runtime.skipContainerID = fmt.Sprintf("%s-%s", pod.ID, "test-container")

		_, err := manager.CreatePod(context.Background(), pod)
		require.NoError(t, err)

		err = manager.StartPod(context.Background(), pod.ID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Timeout waiting for containers to start")
	})
}

// TestUpdatePod verifies pod update functionality
func TestUpdatePod(t *testing.T) {
	// Create a context with longer timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, _ := createTestPodManager(t)
	pod := createTestPod("test-pod-1")

	// Create pod
	status, err := manager.CreatePod(ctx, pod)
	require.NoError(t, err)
	require.Equal(t, pod.ID, status.Spec.ID)

	// Start watching pod to ensure we get updates
	watchChan, err := manager.WatchPod(ctx, pod.ID)
	require.NoError(t, err)

	// Start the pod first
	err = manager.StartPod(ctx, pod.ID)
	require.NoError(t, err)

	// Wait for pod to be running
	waitForPodPhase(t, watchChan, types.PodRunning)

	// Update pod spec
	updatedPod := pod
	updatedPod.Name = "updated-test-pod"
	err = manager.UpdatePod(ctx, pod.ID, updatedPod)
	require.NoError(t, err)

	// Wait for update to be processed and verify name was updated
	waitForPodSpec(t, watchChan, "updated-test-pod")

	// Verify final state
	finalStatus, err := manager.GetPod(ctx, pod.ID)
	require.NoError(t, err)
	require.Equal(t, "updated-test-pod", finalStatus.Spec.Name)
	require.Equal(t, types.PodRunning, finalStatus.Phase)

	// Cleanup
	err = manager.DeletePod(ctx, pod.ID)
	require.NoError(t, err)
}

func waitForPodPhase(t *testing.T, ch <-chan types.PodStatus, phase types.PodPhase) {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case status, ok := <-ch:
			if !ok {
				t.Fatal("Watch channel closed unexpectedly")
				return
			}
			if status.Phase == phase {
				return
			}
		case <-timeout:
			t.Fatalf("Timeout waiting for pod phase %s", phase)
		}
	}
}

func waitForPodSpec(t *testing.T, ch <-chan types.PodStatus, name string) {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case status, ok := <-ch:
			if !ok {
				t.Fatal("Watch channel closed unexpectedly")
				return
			}
			if status.Spec.Name == name {
				return
			}
		case <-timeout:
			t.Fatalf("Timeout waiting for pod spec name %s", name)
		}
	}
}

// TestPodWatch verifies pod watching functionality
func TestPodWatch(t *testing.T) {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	manager, _ := createTestPodManager(t)
	pod := createTestPod("test-pod-1")

	// Create pod
	_, err := manager.CreatePod(ctx, pod)
	require.NoError(t, err)

	// Start watching pod
	watchChan, err := manager.WatchPod(ctx, pod.ID)
	require.NoError(t, err)

	// Create error channel to capture errors from goroutine
	errChan := make(chan error, 1)

	// Start pod in background
	go func() {
		if err := manager.StartPod(ctx, pod.ID); err != nil {
			errChan <- err
			return
		}
		errChan <- nil
	}()

	// Wait for pod to be running or error
	var lastStatus *types.PodStatus
	done := make(chan struct{})

	go func() {
		defer close(done)
		for status := range watchChan {
			lastStatus = &status
			if status.Phase == types.PodRunning {
				return
			}
		}
	}()

	// Wait for either completion or timeout
	select {
	case err := <-errChan:
		require.NoError(t, err, "StartPod failed")
	case <-ctx.Done():
		t.Fatal("timeout waiting for pod to start")
	}

	select {
	case <-done:
		// Success case
	case <-ctx.Done():
		t.Fatal("timeout waiting for pod status updates")
	}

	require.NotNil(t, lastStatus)
	require.Equal(t, types.PodRunning, lastStatus.Phase)

	// Clean up
	err = manager.StopPod(ctx, pod.ID)
	require.NoError(t, err)
}
