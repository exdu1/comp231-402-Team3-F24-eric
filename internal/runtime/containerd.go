package runtime

import (
	"context"
	"fmt"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/pkg/unpack"
	"github.com/containerd/errdefs"
	"io"
	"log"
	"syscall"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-spec/specs-go"

	. "github.com/comp231-402-Team3-F24/pkg/types"
)

// ContainerdRuntime implements the types.Runtime interface using containerd
type ContainerdRuntime struct {
	client *containerd.Client
	ns     string
}

func NewContainerdRuntime(address, namespace string) (*ContainerdRuntime, error) {
	// Add retries for client creation
	var client *containerd.Client
	var err error
	for i := 0; i < 3; i++ {
		client, err = containerd.New(address)
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create containerd client after retries: %w", err)
	}

	// Ensure namespace exists
	ctx := context.Background()
	if err := client.NamespaceService().Create(ctx, namespace, nil); err != nil && !errdefs.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create namespace: %w", err)
	}

	return &ContainerdRuntime{
		client: client,
		ns:     namespace,
	}, nil
}

// CreateContainer creates a new container using the provided configuration
func (r *ContainerdRuntime) CreateContainer(ctx context.Context, config ContainerConfig) error {
	ctx = namespaces.WithNamespace(ctx, r.ns)

	fmt.Printf("Starting container creation for %s\n", config.ID)

	// Check if container already exists
	if err := r.cleanupContainer(ctx, config.ID); err != nil {
		fmt.Printf("Cleanup failed for %s: %v\n", config.ID, err)
		return fmt.Errorf("failed to cleanup existing container: %w", err)
	}

	fmt.Printf("Pulling image for %s\n", config.ID)
	// Pull image if needed
	image, err := r.pullImageIfNotExists(ctx, config.Image)
	if err != nil {
		fmt.Printf("Image pull failed for %s: %v\n", config.ID, err)
		return fmt.Errorf("failed to pull image: %w", err)
	}

	fmt.Printf("Creating container %s\n", config.ID)
	// Create container
	_, err = r.client.NewContainer(
		ctx,
		config.ID,
		containerd.WithImage(image),
		containerd.WithNewSnapshot(config.ID+"-snapshot", image),
		containerd.WithNewSpec(
			oci.WithImageConfig(image),
			r.withContainerConfig(config),
		),
	)
	if err != nil {
		fmt.Printf("Container creation failed for %s: %v\n", config.ID, err)
		return fmt.Errorf("failed to create container: %w", err)
	}

	fmt.Printf("Container %s created successfully\n", config.ID)
	return nil
}

// StartContainer starts an existing container
func (r *ContainerdRuntime) StartContainer(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, r.ns)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	// Create new task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		// If task exists, try to get it
		task, err = container.Task(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to create/get task: %w", err)
		}
	}

	// Start the task
	err = task.Start(ctx)
	if err != nil {
		return fmt.Errorf("failed to start task: %w", err)
	}

	// Wait for the task to be running
	deadline := time.Now().Add(10 * time.Second)
	for {
		status, err := task.Status(ctx)
		if err != nil {
			return fmt.Errorf("failed to get task status: %w", err)
		}

		if status.Status == containerd.Running {
			break
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for container to start")
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// Helper function to cleanup a container and its resources
func (r *ContainerdRuntime) cleanupContainer(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, r.ns)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		// Container doesn't exist, nothing to clean up
		return nil
	}

	// Try to get the task
	task, err := container.Task(ctx, nil)
	if err == nil {
		// Kill the task if it exists
		_ = task.Kill(ctx, syscall.SIGKILL, containerd.WithKillAll)

		// Set up exit status channel
		exitStatusC, err := task.Wait(ctx)
		if err == nil {
			select {
			case <-exitStatusC:
			case <-time.After(5 * time.Second):
				// Timeout waiting for exit
			}
		}

		// Delete the task
		_, _ = task.Delete(ctx, containerd.WithProcessKill)
	}

	// Delete the container
	return container.Delete(ctx, containerd.WithSnapshotCleanup)
}

// StopContainer stops a running container
func (r *ContainerdRuntime) StopContainer(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, r.ns)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		// If there's no task, the container is already stopped
		return nil
	}

	status, err := task.Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get task status: %w", err)
	}

	if status.Status == containerd.Running {
		// Set up exit status channel before killing
		exitStatusC, err := task.Wait(ctx)
		if err != nil {
			return fmt.Errorf("failed to setup task wait: %w", err)
		}

		// Send SIGTERM
		if err := task.Kill(ctx, syscall.SIGTERM); err != nil {
			return fmt.Errorf("failed to send SIGTERM: %w", err)
		}

		// Wait for task to stop with timeout
		select {
		case <-exitStatusC:
			break
		case <-time.After(10 * time.Second):
			// Force kill if timeout
			if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
				return fmt.Errorf("failed to force kill: %w", err)
			}
			select {
			case <-exitStatusC:
				break
			case <-time.After(5 * time.Second):
				return fmt.Errorf("timeout waiting for container to stop")
			}
		}

		// Delete the task
		if _, err := task.Delete(ctx, containerd.WithProcessKill); err != nil {
			return fmt.Errorf("failed to delete task: %w", err)
		}
	}

	// Allow a brief moment for state changes to propagate
	time.Sleep(500 * time.Millisecond)

	return nil
}

// RemoveContainer removes a container and its associated resources
func (r *ContainerdRuntime) RemoveContainer(ctx context.Context, id string) error {
	return r.cleanupContainer(ctx, id)
}

// ListContainers returns a list of all containers in the namespace
func (r *ContainerdRuntime) ListContainers(ctx context.Context) ([]ContainerStatus, error) {
	ctx = namespaces.WithNamespace(ctx, r.ns)

	// Get all containers
	containers, err := r.client.Containers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	// Get container info for each container
	var statuses []ContainerStatus
	for _, container := range containers {
		info, err := container.Info(ctx)
		if err != nil {
			log.Printf("warning: failed to get container info: %v", err)
			continue
		}

		// Get task status if it exists
		status := ContainerStatus{
			ID:        container.ID(),
			CreatedAt: info.CreatedAt,
		}

		// Try to get the task
		task, err := container.Task(ctx, nil)
		if err == nil {
			taskStatus, err := task.Status(ctx)
			if err == nil {
				status.State = string(taskStatus.Status)
			} else {
				status.State = "unknown"
			}
		} else {
			status.State = "created"
		}

		statuses = append(statuses, status)
	}

	return statuses, nil
}

// ContainerStats returns resource usage statistics for a container
func (r *ContainerdRuntime) ContainerStats(ctx context.Context, id string) (*ContainerStats, error) {
	ctx = namespaces.WithNamespace(ctx, r.ns)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to load container: %w", err)
	}

	task, err := container.Task(ctx, cio.NewAttach(cio.WithStdio))
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	_, err = task.Metrics(ctx) // _ = metrics
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}

	// Create a basic stats object
	// TODO: Parse metrics into stats
	stats := &ContainerStats{
		Time:    time.Now(),
		CPU:     CPUStats{},
		Memory:  MemoryStats{},
		Network: NetworkStats{},
	}

	return stats, nil
}

// Helper functions

// Helper function to pull an image if it doesn't exist
func (r *ContainerdRuntime) pullImageIfNotExists(ctx context.Context, ref string) (containerd.Image, error) {
	image, err := r.client.GetImage(ctx, ref)
	if err == nil {
		return image, nil
	}

	image, err = r.client.Pull(ctx, ref, containerd.WithPullUnpack)
	if err != nil {
		return nil, err
	}
	return image, nil
}

// PullImage pulls a container image from a registry
func (r *ContainerdRuntime) PullImage(ctx context.Context, ref string) error {
	ctx = namespaceContext(ctx, r.ns)

	// Check if image already exists locally
	_, err := r.client.GetImage(ctx, ref)
	if err == nil {
		// Image exists locally
		return nil
	}

	// Pull the image with progress tracking
	ongoing := make(chan struct{})
	defer close(ongoing)

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fmt.Printf("Pulling image: %s\n", ref)
			case <-ongoing:
				return
			}
		}
	}()

	// Pull the image with unpack
	_, err = r.client.Pull(ctx, ref,
		containerd.WithPullUnpack,
		containerd.WithPullSnapshotter("overlayfs"),
		containerd.WithImageHandlerWrapper(func(h images.Handler) images.Handler {
			return images.HandlerFunc(func(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
				if desc.MediaType != images.MediaTypeDockerSchema1Manifest {
					return h.Handle(ctx, desc)
				}
				return nil, fmt.Errorf("schema 1 not supported")
			})
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", ref, err)
	}

	return nil
}

// ImportImage imports a container image from a tarball and tags it with the provided imageName.
func (r *ContainerdRuntime) ImportImage(ctx context.Context, imageName string, reader io.Reader) error {
	if reader == nil {
		return fmt.Errorf("nil reader provided for image import")
	}

	ctx = namespaceContext(ctx, r.ns)

	// Import the image
	imgs, err := r.client.Import(ctx, reader)
	if err != nil {
		return fmt.Errorf("failed to import image: %w", err)
	}

	// Ensure at least one image was imported
	if len(imgs) == 0 {
		return fmt.Errorf("no images were imported")
	}

	// Use the first imported image
	img := imgs[0]

	// Unpack the image using the unpack package
	unpacker, err := unpack.NewUnpacker(ctx, r.client.ContentStore())
	if err != nil {
		return fmt.Errorf("failed to create unpacker: %w", err)
	}
	defer func(unpacker *unpack.Unpacker) {
		_, err := unpacker.Wait()
		if err != nil {
			fmt.Printf("failed to wait for unpacker: %v", err)
		}
	}(unpacker) // Ensure resources are released

	handler := unpacker.Unpack(images.HandlerFunc(func(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		return nil, nil
	}))

	// Dispatch the handler
	if err := images.Dispatch(ctx, handler, nil, img.Target); err != nil {
		return fmt.Errorf("failed to unpack image: %w", err)
	}

	// Tag the image with the provided imageName
	// Create a new image object with the desired name that points to the same target
	newImage := images.Image{
		Name:   imageName,
		Target: img.Target,
	}

	// Add the new image to the image store
	if _, err := r.client.ImageService().Create(ctx, newImage); err != nil {
		return fmt.Errorf("failed to tag image: %w", err)
	}

	return nil
}

// ListImages returns a list of available container images
func (r *ContainerdRuntime) ListImages(ctx context.Context) ([]ImageInfo, error) {
	ctx = namespaceContext(ctx, r.ns)

	allImages, err := r.client.ListImages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}

	var imageList []ImageInfo
	for _, img := range allImages {
		size, err := img.Size(ctx)
		if err != nil {
			// Log error but continue
			log.Printf("failed to get size for image %s: %v", img.Name(), err)
		}

		info := ImageInfo{
			ID:        img.Name(),
			Tags:      img.Labels(),
			Size:      size,
			CreatedAt: time.Time{}, // Would need additional metadata parsing to get creation time
		}
		imageList = append(imageList, info)
	}

	return imageList, nil
}

// Helper functions for container configuration
func (r *ContainerdRuntime) withContainerConfig(config ContainerConfig) oci.SpecOpts {
	return func(ctx context.Context, client oci.Client, container *containers.Container, s *specs.Spec) error {
		// Set command if specified
		if len(config.Command) > 0 {
			s.Process.Args = config.Command
		}

		// Set environment variables
		if len(config.Environment) > 0 {
			s.Process.Env = []string{}
			for k, v := range config.Environment {
				s.Process.Env = append(s.Process.Env, fmt.Sprintf("%s=%s", k, v))
			}
		}

		// Set resource limits
		if s.Linux == nil {
			s.Linux = &specs.Linux{}
		}
		if s.Linux.Resources == nil {
			s.Linux.Resources = &specs.LinuxResources{}
		}

		if config.Resources.CPUShares > 0 {
			if s.Linux.Resources.CPU == nil {
				s.Linux.Resources.CPU = &specs.LinuxCPU{}
			}
			shares := uint64(config.Resources.CPUShares)
			s.Linux.Resources.CPU.Shares = &shares
		}

		if config.Resources.MemoryMB > 0 {
			if s.Linux.Resources.Memory == nil {
				s.Linux.Resources.Memory = &specs.LinuxMemory{}
			}
			memory := int64(config.Resources.MemoryMB * 1024 * 1024)
			s.Linux.Resources.Memory.Limit = &memory
		}

		return nil
	}
}

func (r *ContainerdRuntime) getMountOptions(readonly bool) []string {
	options := []string{"rbind"}
	if readonly {
		options = append(options, "ro")
	}
	return options
}

func namespaceContext(ctx context.Context, namespace string) context.Context {
	return namespaces.WithNamespace(ctx, namespace)
}

// Close closes the containerd client connection
func (r *ContainerdRuntime) Close() error {
	// Create a new context for closing operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try one final cleanup
	containers, err := r.ListContainers(ctx)
	if err == nil {
		for _, container := range containers {
			// Use forceCleanup for final attempt
			if err := r.forceCleanup(ctx, container.ID); err != nil {
				// Just log errors at this point
				log.Printf("warning: failed final cleanup of container %s: %v", container.ID, err)
			}
		}
	}

	// Wait a moment for cleanup to complete
	time.Sleep(2 * time.Second)

	// Close the client connection
	if err := r.client.Close(); err != nil {
		return fmt.Errorf("failed to close containerd client: %w", err)
	}

	return nil
}

func (r *ContainerdRuntime) forceCleanup(ctx context.Context, id string) error {
	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return nil // Container doesn't exist
	}

	// Try to get the task
	task, err := container.Task(ctx, nil)
	if err == nil {
		// Kill with SIGKILL
		_ = task.Kill(ctx, syscall.SIGKILL, containerd.WithKillAll)

		// Wait briefly for the kill to take effect
		time.Sleep(1 * time.Second)

		// Delete the task
		_, _ = task.Delete(ctx, containerd.WithProcessKill)
	}

	// Delete container and snapshot
	return container.Delete(ctx, containerd.WithSnapshotCleanup)
}
