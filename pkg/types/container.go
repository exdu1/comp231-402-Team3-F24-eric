package types

import (
	"time"
)

// ContainerConfig holds the configuration for creating a new container
type ContainerConfig struct {
	ID          string
	Name        string
	Image       string
	Command     []string
	Environment map[string]string
	Mounts      []Mount
	Resources   Resources
}

// Mount represents a volume mount
type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// Resources represents container resource limits
type Resources struct {
	CPUShares  uint64
	CPUQuota   int64
	MemoryMB   int64
	MemorySwap int64
}

// ContainerStats represents container resource usage statistics
type ContainerStats struct {
	CPU     CPUStats
	Memory  MemoryStats
	Network NetworkStats
	Time    time.Time
}

// CPUStats represents CPU usage statistics
type CPUStats struct {
	Usage  uint64
	System uint64
	User   uint64
}

// MemoryStats represents memory usage statistics
type MemoryStats struct {
	Usage    uint64
	MaxUsage uint64
	Limit    uint64
}

// NetworkStats represents network usage statistics
type NetworkStats struct {
	RxBytes   uint64
	TxBytes   uint64
	RxPackets uint64
	TxPackets uint64
}

// ContainerStatus represents the current state of a container
type ContainerStatus struct {
	ID         string
	Name       string
	State      string // running, stopped, paused, etc.
	CreatedAt  time.Time
	StartedAt  time.Time
	FinishedAt time.Time
	ExitCode   int
	Error      string
}