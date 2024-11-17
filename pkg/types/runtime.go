package types

import (
	"context"
	"io"
)

// Runtime defines the interface for container operations
type Runtime interface {
	CreateContainer(ctx context.Context, config ContainerConfig) error
	StartContainer(ctx context.Context, id string) error
	StopContainer(ctx context.Context, id string) error
	RemoveContainer(ctx context.Context, id string) error
	ListContainers(ctx context.Context) ([]ContainerStatus, error)
	ContainerStats(ctx context.Context, id string) (*ContainerStats, error)
	PullImage(ctx context.Context, ref string) error
	ImportImage(ctx context.Context, imageName string, reader io.Reader) error
	ListImages(ctx context.Context) ([]ImageInfo, error)
}
