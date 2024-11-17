package types

import (
	"context"
	"io"
)

// Runtime defines the interface for container operations
type Runtime interface {
	// CreateContainer creates a new container using the provided configuration
	CreateContainer(ctx context.Context, config ContainerConfig) error

	// StartContainer starts an existing container
	StartContainer(ctx context.Context, id string) error

	// StopContainer stops a running container
	StopContainer(ctx context.Context, id string) error

	// RemoveContainer removes a container and its associated resources
	RemoveContainer(ctx context.Context, id string) error

	// ListContainers returns a list of all containers
	ListContainers(ctx context.Context) ([]ContainerStatus, error)

	// ContainerStats returns resource usage statistics for a container
	ContainerStats(ctx context.Context, id string) (*ContainerStats, error)

	// PullImage pulls a container image from a registry
	PullImage(ctx context.Context, ref string) error

	// ImportImage imports a container image from a tarball
	ImportImage(ctx context.Context, imageName string, reader io.Reader) error

	// ListImages returns a list of available container images
	ListImages(ctx context.Context) ([]ImageInfo, error)
}

// ContainerRuntime defines the interface for container operations
type ContainerRuntime interface {
	// CreateContainer creates a new container using the provided configuration
	CreateContainer(ctx context.Context, config ContainerConfig) error

	// StartContainer starts an existing container
	StartContainer(ctx context.Context, id string) error

	// StopContainer stops a running container
	StopContainer(ctx context.Context, id string) error

	// RemoveContainer removes a container and its associated resources
	RemoveContainer(ctx context.Context, id string) error

	// ListContainers returns a list of all containers
	ListContainers(ctx context.Context) ([]ContainerStatus, error)

	// ContainerStats returns resource usage statistics for a container
	ContainerStats(ctx context.Context, id string) (*ContainerStats, error)

	// PullImage pulls a container image from a registry
	PullImage(ctx context.Context, ref string) error

	// ImportImage imports a container image from a tarball
	ImportImage(ctx context.Context, imageName string, reader io.Reader) error

	// ListImages returns a list of available container images
	ListImages(ctx context.Context) ([]ImageInfo, error)

	// Close closes the runtime connection
	Close() error
}
