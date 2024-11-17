// internal/runtime/containerd_test.go
package runtime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/comp231-402-Team3-F24/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testNamespace = "litepod-test"
	testImage     = "docker.io/library/alpine:latest"
	sockPath      = "/run/containerd/containerd.sock"
)

type RuntimeTest struct {
	runtime *ContainerdRuntime
	ctx     context.Context
}

func checkContainerdAccessWSL() (bool, string) {
	// Check if we're in WSL
	if _, err := os.Stat("/proc/sys/fs/binfmt_misc/WSLInterop"); err == nil {
		// We're in WSL, do additional checks
		out, err := exec.Command("ps", "aux").Output()
		if err != nil {
			return false, "failed to check processes"
		}

		if !strings.Contains(string(out), "containerd") {
			return false, "containerd process not running in WSL"
		}
	}

	// Check socket existence and permissions
	_, err := os.Stat(sockPath) // info, err
	if err != nil {
		return false, fmt.Sprintf("containerd socket not found: %v", err)
	}

	// Get detailed socket info
	out, err := exec.Command("ls", "-l", sockPath).Output()
	if err != nil {
		return false, fmt.Sprintf("failed to get socket details: %v", err)
	}

	return true, string(out)
}

func setupTest(t *testing.T) (*RuntimeTest, func()) {
	t.Helper()

	// Try to check WSL-specific issues
	ok, msg := checkContainerdAccessWSL()
	if !ok {
		t.Skipf("Skipping containerd tests (WSL): %s", msg)
	}

	// Set up test environment
	if os.Getenv("SKIP_INTEGRATION") == "1" {
		t.Skip("Skipping integration tests (SKIP_INTEGRATION=1)")
	}

	ctx := context.Background()
	runtime, err := NewContainerdRuntime(sockPath, testNamespace)
	if err != nil {
		// Log detailed error information
		t.Logf("Failed to create runtime: %v", err)
		t.Logf("Socket permissions: %s", msg)
		t.Logf("Current user: %d", os.Getuid())
		t.Logf("Current group: %d", os.Getgid())
		t.Skip("Skipping test due to runtime creation failure")
	}

	cleanup := func() {
		if err := runtime.Close(); err != nil {
			t.Logf("Failed to close runtime: %v", err)
		}
	}

	return &RuntimeTest{
		runtime: runtime,
		ctx:     ctx,
	}, cleanup
}

func TestContainerdRuntime_CreateContainer(t *testing.T) {
	rt, cleanup := setupTest(t)
	defer cleanup()

	tests := []struct {
		name    string
		config  types.ContainerConfig
		wantErr bool
	}{
		{
			name: "basic container",
			config: types.ContainerConfig{
				ID:    fmt.Sprintf("test-create-container-%d", time.Now().UnixNano()),
				Name:  "test-container",
				Image: testImage,
				Command: []string{
					"sh",
					"-c",
					"echo 'hello world'",
				},
			},
			wantErr: false,
		},
		{
			name: "container with environment",
			config: types.ContainerConfig{
				ID:    fmt.Sprintf("test-create-container-env-%d", time.Now().UnixNano()),
				Name:  "test-container-env",
				Image: testImage,
				Command: []string{
					"sh",
					"-c",
					"echo $TEST_VAR",
				},
				Environment: map[string]string{
					"TEST_VAR": "test_value",
				},
			},
			wantErr: false,
		},
		{
			name: "container with invalid image",
			config: types.ContainerConfig{
				ID:    fmt.Sprintf("test-create-container-invalid-%d", time.Now().UnixNano()),
				Name:  "test-container-invalid",
				Image: "invalid/image:latest",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up any existing container before the test
			_ = rt.runtime.cleanupContainer(rt.ctx, tt.config.ID)

			// Ensure cleanup after the test
			defer func() {
				if err := rt.runtime.cleanupContainer(rt.ctx, tt.config.ID); err != nil {
					t.Logf("Failed to cleanup container %s: %v", tt.config.ID, err)
				}
			}()

			err := rt.runtime.CreateContainer(rt.ctx, tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Verify container exists
			containers, err := rt.runtime.ListContainers(rt.ctx)
			assert.NoError(t, err)
			found := false
			for _, c := range containers {
				if c.ID == tt.config.ID {
					found = true
					break
				}
			}
			assert.True(t, found, "Created container should be in list")
		})
	}
}

func TestContainerdRuntime_StartStopContainer(t *testing.T) {
	rt, cleanup := setupTest(t)
	defer cleanup()

	containerID := fmt.Sprintf("test-start-stop-%d", time.Now().UnixNano())

	// Clean up any existing container before the test
	_ = rt.runtime.cleanupContainer(rt.ctx, containerID)

	// Ensure cleanup after the test
	defer func() {
		if err := rt.runtime.cleanupContainer(rt.ctx, containerID); err != nil {
			t.Logf("Failed to cleanup container %s: %v", containerID, err)
		}
	}()

	// Create a long-running container
	config := types.ContainerConfig{
		ID:    containerID,
		Name:  "test-start-stop",
		Image: testImage,
		Command: []string{
			"sh",
			"-c",
			"while true; do sleep 1; done",
		},
	}

	// Create container
	err := rt.runtime.CreateContainer(rt.ctx, config)
	require.NoError(t, err, "Failed to create container")

	// Test start
	t.Run("start container", func(t *testing.T) {
		err := rt.runtime.StartContainer(rt.ctx, containerID)
		assert.NoError(t, err, "Failed to start container")

		// Give the container time to start
		time.Sleep(time.Second)

		// Verify container is running
		containers, err := rt.runtime.ListContainers(rt.ctx)
		assert.NoError(t, err)

		found := false
		for _, c := range containers {
			if c.ID == containerID {
				found = true
				assert.Equal(t, "running", c.State, "Container should be running")
				break
			}
		}
		assert.True(t, found, "Container should be in list")
	})

	// Test stop
	t.Run("stop container", func(t *testing.T) {
		err := rt.runtime.StopContainer(rt.ctx, containerID)
		assert.NoError(t, err, "Failed to stop container")

		// Give the container time to stop
		time.Sleep(time.Second)

		// Verify container is stopped (in "created" state)
		containers, err := rt.runtime.ListContainers(rt.ctx)
		assert.NoError(t, err)

		found := false
		for _, c := range containers {
			if c.ID == containerID {
				found = true
				assert.Equal(t, "created", c.State, "Container should be in created state after stopping")
				break
			}
		}
		assert.True(t, found, "Container should be in list")
	})
}

func TestContainerdRuntime_ContainerStats(t *testing.T) {
	rt, cleanup := setupTest(t)
	defer cleanup()

	containerID := fmt.Sprintf("test-stats-%d", time.Now().UnixNano())

	// Clean up any existing container before the test
	_ = rt.runtime.cleanupContainer(rt.ctx, containerID)

	// Ensure cleanup after the test
	defer func() {
		if err := rt.runtime.cleanupContainer(rt.ctx, containerID); err != nil {
			t.Logf("Failed to cleanup container %s: %v", containerID, err)
		}
	}()

	// Create a CPU-intensive container
	config := types.ContainerConfig{
		ID:    containerID,
		Name:  "test-stats",
		Image: testImage,
		Command: []string{
			"sh",
			"-c",
			"while true; do echo 'consuming cpu' > /dev/null; done",
		},
		Resources: types.Resources{
			CPUShares: 1024,
			MemoryMB:  256,
		},
	}

	// Create and start container
	err := rt.runtime.CreateContainer(rt.ctx, config)
	require.NoError(t, err, "Failed to create container")

	err = rt.runtime.StartContainer(rt.ctx, containerID)
	require.NoError(t, err, "Failed to start container")

	// Give the container time to generate some stats
	time.Sleep(2 * time.Second)

	// Test stats collection
	t.Run("collect stats", func(t *testing.T) {
		stats, err := rt.runtime.ContainerStats(rt.ctx, containerID)
		assert.NoError(t, err)
		assert.NotNil(t, stats)
		assert.NotZero(t, stats.Time)
	})
}

func TestContainerdRuntime_RemoveContainer(t *testing.T) {
	rt, cleanup := setupTest(t)
	defer cleanup()

	containerID := fmt.Sprintf("test-remove-%d", time.Now().UnixNano())

	// Clean up any existing container before the test
	_ = rt.runtime.cleanupContainer(rt.ctx, containerID)

	// Create container
	config := types.ContainerConfig{
		ID:      containerID,
		Name:    "test-remove",
		Image:   testImage,
		Command: []string{"sh", "-c", "echo 'test'"},
	}

	err := rt.runtime.CreateContainer(rt.ctx, config)
	require.NoError(t, err, "Failed to create container")

	// Test remove
	t.Run("remove container", func(t *testing.T) {
		err := rt.runtime.RemoveContainer(rt.ctx, containerID)
		assert.NoError(t, err, "Failed to remove container")

		// Verify container is removed
		containers, err := rt.runtime.ListContainers(rt.ctx)
		assert.NoError(t, err)

		for _, c := range containers {
			assert.NotEqual(t, c.ID, containerID, "Container should not be in list")
		}
	})
}

func TestContainerdRuntime_PullImage(t *testing.T) {
	rt, cleanup := setupTest(t)
	defer cleanup()

	tests := []struct {
		name    string
		imgRef  string
		wantErr bool
	}{
		{
			name:    "pull valid alpine image",
			imgRef:  "docker.io/library/alpine:latest",
			wantErr: false,
		},
		{
			name:    "pull invalid image",
			imgRef:  "docker.io/invalid/nonexistent:latest",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rt.runtime.PullImage(rt.ctx, tt.imgRef)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Verify image exists in list
			images, err := rt.runtime.ListImages(rt.ctx)
			assert.NoError(t, err)
			found := false
			for _, img := range images {
				if img.ID == tt.imgRef {
					found = true
					break
				}
			}
			assert.True(t, found, "Pulled image should be in list")
		})
	}
}

func TestContainerdRuntime_ListImages(t *testing.T) {
	rt, cleanup := setupTest(t)
	defer cleanup()

	// Pull only alpine as test image to minimize network issues
	testImages := []string{
		"docker.io/library/alpine:latest",
	}

	for _, img := range testImages {
		err := rt.runtime.PullImage(rt.ctx, img)
		require.NoError(t, err, "Failed to pull test image")
	}

	t.Run("list images", func(t *testing.T) {
		images, err := rt.runtime.ListImages(rt.ctx)
		assert.NoError(t, err)
		assert.NotEmpty(t, images, "Image list should not be empty")

		// Verify test image is in the list
		foundImages := make(map[string]bool)
		for _, img := range images {
			foundImages[img.ID] = true
			// Verify image properties
			assert.NotEmpty(t, img.ID, "Image ID should not be empty")
			// Size might be zero for some images, so we don't check that
		}

		for _, testImg := range testImages {
			assert.True(t, foundImages[testImg], "Test image should be in list")
		}
	})
}

func TestContainerdRuntime_ImportImage(t *testing.T) {
	rt, cleanup := setupTest(t)
	defer cleanup()

	// Create a minimal but valid OCI image tar content
	// TODO switch to a real tar file for more complex tests
	tarContent := []byte{
		0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0xff, 0x4b, 0xcf, 0xcf, 0x4f, 0x49, 0xaa,
		0x4c, 0x85, 0x00, 0x00, 0x00, 0xff, 0xff,
	}

	tests := []struct {
		name      string
		imageName string
		reader    io.Reader
		wantErr   bool
	}{
		{
			name:      "import with empty reader",
			imageName: "test/empty:latest",
			reader:    bytes.NewReader([]byte{}),
			wantErr:   true,
		},
		{
			name:      "import with nil reader",
			imageName: "test/nil:latest",
			reader:    nil,
			wantErr:   true,
		},
		{
			name:      "import with invalid tar content",
			imageName: "test/invalid:latest",
			reader:    bytes.NewReader(tarContent),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.reader == nil {
				// Special handling for nil reader case
				err := rt.runtime.ImportImage(rt.ctx, tt.imageName, nil)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "nil reader")
				return
			}

			err := rt.runtime.ImportImage(rt.ctx, tt.imageName, tt.reader)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Only verify image list if we don't expect an error
			if !tt.wantErr {
				images, err := rt.runtime.ListImages(rt.ctx)
				assert.NoError(t, err)
				found := false
				for _, img := range images {
					if img.ID == tt.imageName {
						found = true
						break
					}
				}
				assert.True(t, found, "Imported image should be in list")
			}
		})
	}
}
