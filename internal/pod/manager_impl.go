package pod

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/comp231-402-Team3-F24/pkg/types"
)

type podManager struct {
	runtime  types.ContainerRuntime
	mu       sync.RWMutex
	pods     map[string]*types.PodStatus
	events   chan podEvent
	shutdown chan struct{}
	watchers sync.Map
}

type podEvent struct {
	podID string
	event string
	err   error
}

func NewManager(rt types.ContainerRuntime) Manager {
	pm := &podManager{
		runtime:  rt,
		pods:     make(map[string]*types.PodStatus),
		events:   make(chan podEvent, 100),
		shutdown: make(chan struct{}),
		watchers: sync.Map{},
	}

	go func() {
		pm.processEvents()
	}()
	return pm
}

func (pm *podManager) Close() error {
	close(pm.shutdown) // Signal the processEvents goroutine to stop
	// Give processEvents time to clean up
	time.Sleep(100 * time.Millisecond)
	return pm.runtime.Close()
}

func (pm *podManager) CreatePod(ctx context.Context, spec types.PodSpec) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		if err := ValidateSpec(&spec); err != nil {
			return fmt.Errorf("invalid pod specification: %w", err)
		}

		pm.mu.Lock()
		defer pm.mu.Unlock()

		if _, exists := pm.pods[spec.ID]; exists {
			return ErrPodAlreadyExists
		}

		status := &types.PodStatus{
			Spec:      spec,
			Phase:     types.PodPending,
			CreatedAt: time.Now(),
			Conditions: []types.PodCondition{
				{
					Type:               types.PodInitialized,
					Status:             false,
					LastTransitionTime: time.Now(),
				},
			},
		}

		pm.pods[spec.ID] = status

		// Create containers synchronously to ensure they exist before returning
		if err := pm.createPodContainers(ctx, spec); err != nil {
			delete(pm.pods, spec.ID)
			return fmt.Errorf("failed to create pod containers: %w", err)
		}

		// Directly update status instead of using events
		status.Phase = types.PodPending
		return nil
	}
}

func (pm *podManager) createPodContainers(ctx context.Context, spec types.PodSpec) error {
	var errs []error
	for _, container := range spec.Containers {
		// Merge pod-level environment variables
		mergedEnv := make(map[string]string)
		for k, v := range spec.Environment {
			mergedEnv[k] = v
		}
		for k, v := range container.Environment {
			mergedEnv[k] = v
		}
		container.Environment = mergedEnv

		if err := pm.runtime.CreateContainer(ctx, container); err != nil {
			errs = append(errs, fmt.Errorf("failed to create container %s: %w", container.ID, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%v", errs)
	}
	return nil
}

func (pm *podManager) StartPod(ctx context.Context, id string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		pm.mu.RLock()
		pod, exists := pm.pods[id]
		if !exists {
			pm.mu.RUnlock()
			return ErrPodNotFound
		}
		pm.mu.RUnlock()

		var errs []error
		for _, container := range pod.Spec.Containers {
			if err := pm.runtime.StartContainer(ctx, container.ID); err != nil {
				errs = append(errs, fmt.Errorf("failed to start container %s: %w", container.ID, err))
				// Continue trying to start other containers
			}
		}

		if len(errs) > 0 {
			// Even if some containers failed to start, we'll try to stop all to maintain consistency
			go pm.StopPod(context.Background(), id)
			return fmt.Errorf("%v", errs)
		}

		pm.updatePodStatus(id, types.PodRunning, "")
		return nil
	}
}

func (pm *podManager) StopPod(ctx context.Context, id string) error {
	pm.mu.RLock()
	pod, exists := pm.pods[id]
	if !exists {
		pm.mu.RUnlock()
		return ErrPodNotFound
	}
	pm.mu.RUnlock()

	var errs []error
	for _, container := range pod.Spec.Containers {
		if err := pm.runtime.StopContainer(ctx, container.ID); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop container %s: %w", container.ID, err))
			// Continue trying to stop other containers
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%v", errs)
	}

	pm.updatePodStatus(id, types.PodPending, "")
	return nil
}

func (pm *podManager) DeletePod(ctx context.Context, id string) error {
	pm.mu.Lock()
	pod, exists := pm.pods[id]
	if !exists {
		pm.mu.Unlock()
		return ErrPodNotFound
	}
	pm.mu.Unlock()

	// Try to stop first, but continue with deletion even if stop fails
	_ = pm.StopPod(ctx, id)

	var errs []error
	for _, container := range pod.Spec.Containers {
		if err := pm.runtime.RemoveContainer(ctx, container.ID); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove container %s: %w", container.ID, err))
		}
	}

	pm.mu.Lock()
	delete(pm.pods, id)
	pm.mu.Unlock()

	if len(errs) > 0 {
		return fmt.Errorf("%v", errs)
	}
	return nil
}

func (pm *podManager) GetPod(ctx context.Context, id string) (*types.PodStatus, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	status, exists := pm.pods[id]
	if !exists {
		return nil, ErrPodNotFound
	}

	// Update container statuses before returning
	containerStatuses := make([]types.ContainerStatus, 0, len(status.Spec.Containers))
	containers, err := pm.runtime.ListContainers(ctx)
	if err == nil {
		containerMap := make(map[string]types.ContainerStatus)
		for _, c := range containers {
			containerMap[c.ID] = c
		}

		for _, container := range status.Spec.Containers {
			if cs, ok := containerMap[container.ID]; ok {
				containerStatuses = append(containerStatuses, cs)
			}
		}
	}
	status.ContainerStatuses = containerStatuses

	statusCopy := *status
	return &statusCopy, nil
}

func (pm *podManager) GetPodLogs(ctx context.Context, id string, opts LogOptions) (map[string]string, error) {
	pm.mu.RLock()
	pod, exists := pm.pods[id]
	if !exists {
		pm.mu.RUnlock()
		return nil, ErrPodNotFound
	}
	pm.mu.RUnlock()

	logs := make(map[string]string)
	for _, container := range pod.Spec.Containers {
		// In a real implementation, this would get actual container logs
		logs[container.Name] = fmt.Sprintf("Logs for container %s in pod %s", container.Name, id)
	}

	return logs, nil
}

func (pm *podManager) ListPods(ctx context.Context) ([]types.PodStatus, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	pods := make([]types.PodStatus, 0, len(pm.pods))
	for _, status := range pm.pods {
		pods = append(pods, *status)
	}
	return pods, nil
}

func (pm *podManager) UpdatePod(ctx context.Context, id string, spec types.PodSpec) error {
	if err := ValidateSpec(&spec); err != nil {
		return fmt.Errorf("invalid pod specification: %w", err)
	}

	pm.mu.Lock()
	pod, exists := pm.pods[id]
	if !exists {
		pm.mu.Unlock()
		return ErrPodNotFound
	}

	oldSpec := pod.Spec
	pod.Spec = spec // Update the spec immediately
	pm.mu.Unlock()

	if err := pm.applyPodUpdate(ctx, id, oldSpec, spec); err != nil {
		// Restore old spec on failure
		pm.mu.Lock()
		pod.Spec = oldSpec
		pm.mu.Unlock()
		return err
	}

	pm.updatePodStatus(id, types.PodRunning, "")
	return nil
}

func (pm *podManager) WatchPod(ctx context.Context, id string) (<-chan types.PodStatus, error) {
	pm.mu.RLock()
	_, exists := pm.pods[id]
	pm.mu.RUnlock()
	if !exists {
		return nil, ErrPodNotFound
	}

	statusChan := make(chan types.PodStatus, 10)
	pm.watchers.Store(id, statusChan)

	go func() {
		<-ctx.Done()
		pm.watchers.Delete(id)
		close(statusChan)
	}()

	return statusChan, nil
}

func (pm *podManager) updatePodStatus(id string, phase types.PodPhase, message string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pod, exists := pm.pods[id]; exists {
		pod.Phase = phase
		pod.Message = message
		now := time.Now()
		for i := range pod.Conditions {
			if pod.Conditions[i].Status != (phase == types.PodRunning) {
				pod.Conditions[i].Status = phase == types.PodRunning
				pod.Conditions[i].LastTransitionTime = now
				pod.Conditions[i].Message = message
			}
		}

		if ch, ok := pm.watchers.Load(id); ok {
			select {
			case ch.(chan types.PodStatus) <- *pod:
			default:
			}
		}
	}
}

func (pm *podManager) processEvents() {
	for {
		select {
		case event := <-pm.events:
			pm.mu.Lock()
			if pod, exists := pm.pods[event.podID]; exists {
				switch event.event {
				case "created":
					pod.Phase = types.PodRunning
				case "creation_failed":
					pod.Phase = types.PodFailed
					pod.Message = event.err.Error()
				case "deleted":
					delete(pm.pods, event.podID)
				case "container_stop_failed":
					pod.Phase = types.PodFailed
					pod.Message = event.err.Error()
				}

				// Notify watchers
				if watchChan, ok := pm.watchers.Load(event.podID); ok {
					select {
					case watchChan.(chan types.PodStatus) <- *pod:
					default:
						// Channel is full or closed, skip notification
					}
				}
			}
			pm.mu.Unlock()
		case <-pm.shutdown:
			return
		}
	}
}

func (pm *podManager) applyPodUpdate(ctx context.Context, id string, oldSpec, newSpec types.PodSpec) error {
	// Stop all existing containers first
	for _, container := range oldSpec.Containers {
		if err := pm.runtime.StopContainer(ctx, container.ID); err != nil {
			log.Printf("warning: failed to stop container %s: %v", container.ID, err)
		}
	}

	// Wait for containers to actually stop
	time.Sleep(2 * time.Second)

	// Remove all old containers
	for _, container := range oldSpec.Containers {
		if err := pm.runtime.RemoveContainer(ctx, container.ID); err != nil {
			log.Printf("warning: failed to remove container %s: %v", container.ID, err)
		}
	}

	// Verify all old containers are gone
	containers, err := pm.runtime.ListContainers(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify container cleanup: %w", err)
	}

	// Create a map of old container IDs for quick lookup
	oldContainerIDs := make(map[string]bool)
	for _, container := range oldSpec.Containers {
		oldContainerIDs[container.ID] = true
	}

	// Check if any old containers still exist
	for _, container := range containers {
		if oldContainerIDs[container.ID] {
			// Try one more time to remove it
			_ = pm.runtime.StopContainer(ctx, container.ID)
			_ = pm.runtime.RemoveContainer(ctx, container.ID)
		}
	}

	// Create new containers
	for _, container := range newSpec.Containers {
		if err := pm.runtime.CreateContainer(ctx, container); err != nil {
			return fmt.Errorf("failed to create container %s: %w", container.ID, err)
		}
	}

	// Start new containers
	for _, container := range newSpec.Containers {
		if err := pm.runtime.StartContainer(ctx, container.ID); err != nil {
			return fmt.Errorf("failed to start container %s: %w", container.ID, err)
		}
	}

	return nil
}
