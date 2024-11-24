package types

import (
	"time"
)

// PodSpec defines the desired state of a pod
type PodSpec struct {
	// Unique identifier for the pod
	ID string `json:"id"`

	// Human-readable name for the pod
	Name string `json:"name"`

	// List of containers in the pod
	Containers []ContainerConfig `json:"containers"`

	// Resource requirements/limits for the entire pod
	Resources Resources `json:"resources"`

	// Network configuration
	Network PodNetworkConfig `json:"network"`

	// Volume mounts shared across containers
	Volumes []PodVolume `json:"volumes"`

	// Pod-wide environment variables
	Environment map[string]string `json:"environment,omitempty"`

	// Restart policy for containers in the pod
	RestartPolicy RestartPolicy `json:"restartPolicy"`

	// Labels for pod metadata
	Labels map[string]string `json:"labels,omitempty"`
}

// PodStatus represents the current state of a pod
type PodStatus struct {
	// Pod specification
	Spec PodSpec `json:"spec"`

	// Current phase of the pod
	Phase PodPhase `json:"phase"`

	// Detailed status message
	Message string `json:"message,omitempty"`

	// Container statuses
	ContainerStatuses []ContainerStatus `json:"containerStatuses"`

	// Pod IP address
	IP string `json:"ip,omitempty"`

	// Important timestamps
	CreatedAt  time.Time `json:"createdAt"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`

	// Conditions represent the latest available observations of a pod's state
	Conditions []PodCondition `json:"conditions,omitempty"`

	// Pod error messages
	ErrPodNotFound error
}

// PodPhase represents the current lifecycle phase of a pod
type PodPhase string

const (
	// PodPending means the pod has been accepted but not all containers are ready
	PodPending PodPhase = "Pending"
	// PodRunning means the pod has been bound to a node and all containers are created
	PodRunning PodPhase = "Running"
	// PodSucceeded means all containers in the pod have terminated successfully
	PodSucceeded PodPhase = "Succeeded"
	// PodFailed means all containers in the pod have terminated, and at least one container has terminated in failure
	PodFailed PodPhase = "Failed"
	// PodUnknown means the state of the pod could not be obtained
	PodUnknown PodPhase = "Unknown"
)

// PodConditionType represents the type of condition a pod is in
type PodConditionType string

const (
	// PodReady means the pod is able to service requests
	PodReady PodConditionType = "Ready"
	// PodInitialized means all init containers have completed successfully
	PodInitialized PodConditionType = "Initialized"
	// PodContainersReady means all containers in the pod are ready
	PodContainersReady PodConditionType = "ContainersReady"
)

// PodCondition contains details for the current condition of a pod
type PodCondition struct {
	Type    PodConditionType `json:"type"`
	Status  bool             `json:"status"`
	Message string           `json:"message,omitempty"`
	// Time when the condition was last updated
	LastTransitionTime time.Time `json:"lastTransitionTime"`
}

// PodNetworkConfig contains network configuration for a pod
type PodNetworkConfig struct {
	// Whether to use host networking
	HostNetwork bool `json:"hostNetwork,omitempty"`
	// DNS configuration
	DNS PodDNSConfig `json:"dns,omitempty"`
	// Port mappings
	Ports []PortMapping `json:"ports,omitempty"`
}

// PodDNSConfig contains DNS configuration for a pod
type PodDNSConfig struct {
	Nameservers []string `json:"nameservers,omitempty"`
	Searches    []string `json:"searches,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// PortMapping defines a port mapping between the host and pod
type PortMapping struct {
	Name          string `json:"name,omitempty"`
	HostPort      int32  `json:"hostPort"`
	ContainerPort int32  `json:"containerPort"`
	Protocol      string `json:"protocol,omitempty"`
}

// PodVolume represents a volume that can be mounted into containers
type PodVolume struct {
	Name     string `json:"name"`
	HostPath string `json:"hostPath,omitempty"`
	// Add support for other volume types as needed
}

// RestartPolicy describes how containers should be restarted
type RestartPolicy string

const (
	// RestartAlways means the container will always be restarted
	RestartAlways RestartPolicy = "Always"
	// RestartOnFailure means the container will be restarted only if it exits with a non-zero exit code
	RestartOnFailure RestartPolicy = "OnFailure"
	// RestartNever means the container will never be restarted
	RestartNever RestartPolicy = "Never"
)
