package runtime

import (
	"context"
	"fmt"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/pkg/unpack"
	"io"
	"log"
	"syscall"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
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
	client, err := containerd.New(address)
	if err != nil {
		return nil, fmt.Errorf("failed to create containerd client: %w", err)
	}

	return &ContainerdRuntime{
		client: client,
		ns:     namespace,
	}, nil
}

// CreateContainer creates a new container using the provided configuration
func (r *ContainerdRuntime) CreateContainer(ctx context.Context, config ContainerConfig) error {
	ctx = namespaceContext(ctx, r.ns)

	// Check if container already exists and remove it if it does
	if existing, err := r.client.LoadContainer(ctx, config.ID); err == nil {
		if err := r.RemoveContainer(ctx, existing.ID()); err != nil {
			return fmt.Errorf("failed to remove existing container: %w", err)
		}
	}

	// Pull the image if it doesn't exist
	image, err := r.pullImageIfNotExists(ctx, config.Image)
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	// Create container with cleanup on failure
	container, err := r.client.NewContainer(
		ctx,
		config.ID,
		containerd.WithImage(image),
		containerd.WithNewSnapshot(config.ID+"-snapshot", image),
		containerd.WithNewSpec(
			oci.WithImageConfig(image),
			oci.WithHostNamespace(specs.NetworkNamespace),
			r.withContainerConfig(config),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	// Create task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		// Cleanup container on task creation failure
		if cleanupErr := container.Delete(ctx, containerd.WithSnapshotCleanup); cleanupErr != nil {
			log.Printf("failed to cleanup container after task creation failure: %v", cleanupErr)
		}
		return fmt.Errorf("failed to create task: %w", err)
	}
	println(task.ID())

	return nil
}

// StartContainer starts an existing container with proper state checking
func (r *ContainerdRuntime) StartContainer(ctx context.Context, id string) error {
	ctx = namespaceContext(ctx, r.ns)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	// Check current status before starting
	status, err := task.Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get task status: %w", err)
	}

	// Only start if not already running
	if status.Status != containerd.Running {
		if err := task.Start(ctx); err != nil {
			return fmt.Errorf("failed to start task: %w", err)
		}
	}

	return nil
}

// Helper function to cleanup a container and its resources
func (r *ContainerdRuntime) cleanupContainer(ctx context.Context, id string) error {
	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return nil // Container doesn't exist, nothing to clean up
	}

	task, err := container.Task(ctx, nil)
	if err == nil {
		// If task exists, force kill it
		_ = task.Kill(ctx, syscall.SIGKILL, containerd.WithKillAll)
		// Wait with timeout for task to exit
		exitCh, _ := task.Wait(ctx)
		select {
		case <-exitCh:
		case <-time.After(5 * time.Second):
		}
		_, _ = task.Delete(ctx, containerd.WithProcessKill)
	}

	return container.Delete(ctx, containerd.WithSnapshotCleanup)
}

// StopContainer stops a running container
func (r *ContainerdRuntime) StopContainer(ctx context.Context, id string) error {
	ctx = namespaceContext(ctx, r.ns)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	status, err := task.Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get task status: %w", err)
	}

	if status.Status == containerd.Running {
		// Give the container 30 seconds to stop gracefully
		if err := task.Kill(ctx, syscall.SIGKILL, containerd.WithKillAll); err != nil {
			return fmt.Errorf("failed to kill task: %w", err)
		}

		// Wait for the task to exit
		_, err = task.Wait(ctx)
		if err != nil {
			return fmt.Errorf("failed to wait for task: %w", err)
		}
	}

	return nil
}

// RemoveContainer removes a container and its associated resources
func (r *ContainerdRuntime) RemoveContainer(ctx context.Context, id string) error {
	ctx = namespaceContext(ctx, r.ns)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	// First, try to get the task
	task, err := container.Task(ctx, nil)
	if err == nil {
		// If task exists, check if it's running
		status, err := task.Status(ctx)
		if err == nil && status.Status == containerd.Running {
			// Stop the task first
			if err := task.Kill(ctx, syscall.SIGTERM, containerd.WithKillAll); err != nil {
				// Log the error but continue with removal
				log.Printf("warning: failed to kill task: %v", err)
			}
			// Wait for the task to exit
			exitStatus, err := task.Wait(ctx)
			if err == nil {
				select {
				case <-exitStatus:
					// Task exited
				case <-time.After(10 * time.Second):
					// Force kill if it doesn't exit gracefully
					if err := task.Kill(ctx, syscall.SIGKILL, containerd.WithKillAll); err != nil {
						log.Printf("warning: failed to force kill task: %v", err)
					}
				}
			}
		}

		// Delete the task
		if _, err := task.Delete(ctx, containerd.WithProcessKill); err != nil {
			log.Printf("warning: failed to delete task: %v", err)
		}
	}

	// Delete the container
	if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
		return fmt.Errorf("failed to delete container: %w", err)
	}

	return nil
}

// ListContainers returns a list of all containers in the namespace
func (r *ContainerdRuntime) ListContainers(ctx context.Context) ([]ContainerStatus, error) {
	ctx = namespaceContext(ctx, r.ns)

	allContainers, err := r.client.Containers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var statuses []ContainerStatus

	for _, container := range allContainers {
		info, err := container.Info(ctx)
		if err != nil {
			// Log the error but continue listing other containers
			log.Printf("warning: failed to get info for container %s: %v", container.ID(), err)
			continue
		}

		status := ContainerStatus{
			ID:        container.ID(),
			CreatedAt: info.CreatedAt,
		}

		// Try to get labels including the name
		if name, ok := info.Labels["name"]; ok {
			status.Name = name
		}

		// Try to get the task to check running state
		task, err := container.Task(ctx, nil)
		if err == nil {
			taskStatus, err := task.Status(ctx)
			if err == nil {
				status.State = string(taskStatus.Status)
				status.FinishedAt = taskStatus.ExitTime
				status.ExitCode = int(taskStatus.ExitStatus)
			} else {
				status.State = "unknown"
				status.Error = err.Error()
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
	ctx = namespaceContext(ctx, r.ns)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	_, err = task.Metrics(ctx) // _ = metrics
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}

	// TODO: Fix this
	// Convert containerd metrics to our ContainerStats type
	stats := &ContainerStats{
		Time: time.Now(),
	}
	//	CPU:    metrics.CPU,
	//	Memory: metrics.Memory,
	//	Network: NetworkStats{
	//		RxBytes: metrics.Network.RxBytes,
	//		TxBytes: metrics.Network.TxBytes,
	//	},
	//}

	return stats, nil
}

// Helper functions

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

func (r *ContainerdRuntime) withContainerConfig(config ContainerConfig) oci.SpecOpts {
	return func(ctx context.Context, _ oci.Client, c *containers.Container, s *specs.Spec) error {
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
		if s.Linux.Resources.CPU == nil {
			s.Linux.Resources.CPU = &specs.LinuxCPU{}
		}
		if s.Linux.Resources.Memory == nil {
			s.Linux.Resources.Memory = &specs.LinuxMemory{}
		}

		if config.Resources.CPUShares > 0 {
			shares := uint64(config.Resources.CPUShares)
			s.Linux.Resources.CPU.Shares = &shares
		}
		if config.Resources.CPUQuota > 0 {
			quota := config.Resources.CPUQuota
			s.Linux.Resources.CPU.Quota = &quota
		}
		if config.Resources.MemoryMB > 0 {
			memory := int64(config.Resources.MemoryMB * 1024 * 1024)
			s.Linux.Resources.Memory.Limit = &memory
		}

		// Set mounts
		for _, mount := range config.Mounts {
			s.Mounts = append(s.Mounts, specs.Mount{
				Source:      mount.Source,
				Destination: mount.Target,
				Options:     r.getMountOptions(mount.ReadOnly),
			})
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
	return r.client.Close()
}
