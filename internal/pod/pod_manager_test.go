package pod

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/comp231-402-Team3-F24/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
				ID:   "test-pod-1",
				Name: "test-pod",
				Containers: []types.ContainerConfig{
					{
						ID:    "test-container-1",
						Name:  "test-container",
						Image: "docker.io/library/alpine:latest",
					},
				},
				Resources: types.Resources{
					CPUShares: 1024,
					MemoryMB:  256,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid pod spec - no containers",
			spec: types.PodSpec{
				ID:   "test-pod-2",
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
						ID:    "test-container-1",
						Name:  "test-container",
						Image: "docker.io/library/alpine:latest",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create new runtime mock
			mockRuntime := &MockRuntime{}
			pm := NewManager(mockRuntime)

			// Test pod creation
			err := pm.CreatePod(context.Background(), tt.spec)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Verify pod was created
			pod, err := pm.GetPod(context.Background(), tt.spec.ID)
			assert.NoError(t, err)
			assert.Equal(t, tt.spec.ID, pod.Spec.ID)
		})
	}
}

// TestDeletePod verifies pod deletion functionality
func TestDeletePod(t *testing.T) {
	// Create new runtime mock
	mockRuntime := &MockRuntime{}
	pm := NewManager(mockRuntime)

	// Create a pod for testing
	spec := types.PodSpec{
		ID:   "test-pod-delete",
		Name: "test-pod",
		Containers: []types.ContainerConfig{
			{
				ID:    "test-container-1",
				Name:  "test-container",
				Image: "docker.io/library/alpine:latest",
			},
		},
	}

	// Create pod
	err := pm.CreatePod(context.Background(), spec)
	require.NoError(t, err)

	// Test deletion
	err = pm.DeletePod(context.Background(), spec.ID)
	assert.NoError(t, err)

	// Verify pod was deleted
	_, err = pm.GetPod(context.Background(), spec.ID)
	assert.Equal(t, ErrPodNotFound, err)

	// Test deleting non-existent pod
	err = pm.DeletePod(context.Background(), "nonexistent-pod")
	assert.Equal(t, ErrPodNotFound, err)
}

// TestStartStopPod verifies pod lifecycle management
func TestStartStopPod(t *testing.T) {
	// Create new runtime mock
	mockRuntime := &MockRuntime{}
	pm := NewManager(mockRuntime)

	// Create a pod for testing
	spec := types.PodSpec{
		ID:   "test-pod-lifecycle",
		Name: "test-pod",
		Containers: []types.ContainerConfig{
			{
				ID:    "test-container-1",
				Name:  "test-container",
				Image: "docker.io/library/alpine:latest",
			},
		},
	}

	// Create pod
	err := pm.CreatePod(context.Background(), spec)
	require.NoError(t, err)

	// Test starting pod
	err = pm.StartPod(context.Background(), spec.ID)
	assert.NoError(t, err)

	// Verify pod is running
	pod, err := pm.GetPod(context.Background(), spec.ID)
	assert.NoError(t, err)
	assert.Equal(t, types.PodRunning, pod.Phase)

	// Test stopping pod
	err = pm.StopPod(context.Background(), spec.ID)
	assert.NoError(t, err)

	// Verify pod is stopped
	pod, err = pm.GetPod(context.Background(), spec.ID)
	assert.NoError(t, err)
	assert.Equal(t, types.PodPending, pod.Phase)
}

// TestUpdatePod verifies pod update functionality
func TestUpdatePod(t *testing.T) {
	// Create new runtime mock
	mockRuntime := &MockRuntime{}
	pm := NewManager(mockRuntime)

	// Create initial pod spec
	originalSpec := types.PodSpec{
		ID:   "test-pod-update",
		Name: "test-pod",
		Containers: []types.ContainerConfig{
			{
				ID:    "test-container-1",
				Name:  "test-container",
				Image: "docker.io/library/alpine:latest",
			},
		},
	}

	// Create pod
	err := pm.CreatePod(context.Background(), originalSpec)
	require.NoError(t, err)

	// Create updated spec
	updatedSpec := originalSpec
	updatedSpec.Containers = append(updatedSpec.Containers, types.ContainerConfig{
		ID:    "test-container-2",
		Name:  "test-container-2",
		Image: "docker.io/library/nginx:latest",
	})

	// Test update
	err = pm.UpdatePod(context.Background(), originalSpec.ID, updatedSpec)
	assert.NoError(t, err)

	// Verify update
	pod, err := pm.GetPod(context.Background(), originalSpec.ID)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(pod.Spec.Containers))
}

// TestPodWatch verifies pod watching functionality
func TestPodWatch(t *testing.T) {
	// Create new runtime mock
	mockRuntime := &MockRuntime{}
	pm := NewManager(mockRuntime)

	// Create a pod for testing
	spec := types.PodSpec{
		ID:   "test-pod-watch",
		Name: "test-pod",
		Containers: []types.ContainerConfig{
			{
				ID:    "test-container-1",
				Name:  "test-container",
				Image: "docker.io/library/alpine:latest",
			},
		},
	}

	// Create pod
	err := pm.CreatePod(context.Background(), spec)
	require.NoError(t, err)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start watching pod
	watchChan, err := pm.WatchPod(ctx, spec.ID)
	require.NoError(t, err)

	// Start pod to trigger status change
	go func() {
		err := pm.StartPod(context.Background(), spec.ID)
		require.NoError(t, err)
	}()

	// Wait for status update
	select {
	case status := <-watchChan:
		assert.Equal(t, types.PodRunning, status.Phase)
	case <-ctx.Done():
		t.Fatal("timeout waiting for pod status update")
	}
}

// MockRuntime implements the ContainerRuntime interface for testing
type MockRuntime struct {
	createCalls int
	startCalls  int
	stopCalls   int
	removeCalls int
}

func (m *MockRuntime) CreateContainer(ctx context.Context, config types.ContainerConfig) error {
	m.createCalls++
	return nil
}

func (m *MockRuntime) StartContainer(ctx context.Context, id string) error {
	m.startCalls++
	return nil
}

func (m *MockRuntime) StopContainer(ctx context.Context, id string) error {
	m.stopCalls++
	return nil
}

func (m *MockRuntime) RemoveContainer(ctx context.Context, id string) error {
	m.removeCalls++
	return nil
}

func (m *MockRuntime) ListContainers(ctx context.Context) ([]types.ContainerStatus, error) {
	return []types.ContainerStatus{}, nil
}

func (m *MockRuntime) ContainerStats(ctx context.Context, id string) (*types.ContainerStats, error) {
	return &types.ContainerStats{}, nil
}

func (m *MockRuntime) PullImage(ctx context.Context, ref string) error {
	return nil
}

func (m *MockRuntime) ImportImage(ctx context.Context, imageName string, reader io.Reader) error {
	return nil
}

func (m *MockRuntime) ListImages(ctx context.Context) ([]types.ImageInfo, error) {
	return []types.ImageInfo{}, nil
}

func (m *MockRuntime) Close() error {
	return nil
}
