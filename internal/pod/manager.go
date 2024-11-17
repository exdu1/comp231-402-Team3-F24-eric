package pod

import (
	"context"
	"errors"
	"time"

	"github.com/comp231-402-Team3-F24/pkg/types"
)

var (
	// ErrPodNotFound indicates the requested pod doesn't exist
	ErrPodNotFound = errors.New("pod not found")
	// ErrPodAlreadyExists indicates a pod with the same ID already exists
	ErrPodAlreadyExists = errors.New("pod already exists")
	// ErrInvalidPodSpec indicates the pod specification is invalid
	ErrInvalidPodSpec = errors.New("invalid pod specification")
	// ErrPodNotRunning indicates an operation requires a running pod
	ErrPodNotRunning = errors.New("pod is not running")
)

// Manager defines the interface for pod operations
type Manager interface {
	// CreatePod creates a new pod from the given specification
	CreatePod(ctx context.Context, spec types.PodSpec) error

	// DeletePod removes a pod and all its resources
	DeletePod(ctx context.Context, id string) error

	// GetPod retrieves information about a specific pod
	GetPod(ctx context.Context, id string) (*types.PodStatus, error)

	// ListPods returns a list of all pods
	ListPods(ctx context.Context) ([]types.PodStatus, error)

	// StartPod starts all containers in a pod
	StartPod(ctx context.Context, id string) error

	// StopPod stops all containers in a pod
	StopPod(ctx context.Context, id string) error

	// UpdatePod updates a pod's specification
	UpdatePod(ctx context.Context, id string, spec types.PodSpec) error

	// GetPodLogs retrieves logs from all containers in a pod
	GetPodLogs(ctx context.Context, id string, opts LogOptions) (map[string]string, error)

	// WatchPod watches for changes in pod status
	WatchPod(ctx context.Context, id string) (<-chan types.PodStatus, error)
}

// LogOptions specifies options for retrieving container logs
type LogOptions struct {
	// Follow the log stream
	Follow bool
	// Number of lines to retrieve
	TailLines *int64
	// Include timestamps in logs
	Timestamps bool
	// Only get logs since this time
	SinceTime *time.Time
}
