package pod

import (
	"context"
	"fmt"
	"strings"
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

func (pm *podManager) CreatePod(ctx context.Context, spec types.PodSpec) (*types.PodStatus, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		pm.mu.Lock()
		defer pm.mu.Unlock()

		// Check if pod with this ID already exists
		if _, exists := pm.pods[spec.ID]; exists {
			return nil, ErrPodAlreadyExists
		}

		// Validate pod specification
		if err := ValidateSpec(&spec); err != nil {
			return nil, fmt.Errorf("invalid pod specification: %w", err)
		}

		// Create pod status
		podStatus := &types.PodStatus{
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

		// Create containers for the pod
		if err := pm.createPodContainers(ctx, spec); err != nil {
			return nil, fmt.Errorf("failed to create pod containers: %w", err)
		}

		// Store pod status
		pm.pods[spec.ID] = podStatus

		// Trigger pod creation event
		pm.events <- podEvent{
			podID: spec.ID,
			event: "created",
		}

		return podStatus, nil
	}
}

func (pm *podManager) createPodContainers(ctx context.Context, spec types.PodSpec) error {
	var errs []error
	for _, container := range spec.Containers {
		// Set container ID based on pod ID and container name
		container.ID = fmt.Sprintf("%s-%s", spec.ID, container.Name)

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
	// Check pod existence with read lock
	pm.mu.RLock()
	pod, exists := pm.pods[id]
	if !exists {
		pm.mu.RUnlock()
		return ErrPodNotFound
	}
	// Make a copy of the spec while under read lock
	spec := pod.Spec
	currentPhase := pod.Phase
	pm.mu.RUnlock()

	// Only proceed if pod is not already running
	if currentPhase == types.PodRunning {
		return nil
	}

	// Update status to pending if not already
	if currentPhase != types.PodPending {
		pm.updatePodStatus(id, types.PodPending, "Starting pod containers")
	}

	// Start each container
	for _, container := range spec.Containers {
		fmt.Printf("Starting container %s for pod %s\n", container.ID, id)
		if err := pm.runtime.StartContainer(ctx, container.ID); err != nil {
			message := fmt.Sprintf("Failed to start container %s: %v", container.ID, err)
			pm.updatePodStatus(id, types.PodFailed, message)
			return fmt.Errorf("failed to start container %s: %w", container.ID, err)
		}
		fmt.Printf("Container %s start initiated for pod %s\n", container.ID, id)
	}

	// Create a separate context with timeout for container startup
	startupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Create a channel to receive container startup result
	resultChan := make(chan error, 1)

	// Start monitoring container states in a separate goroutine
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-startupCtx.Done():
				if startupCtx.Err() == context.DeadlineExceeded {
					message := "Timeout waiting for containers to start"
					pm.updatePodStatus(id, types.PodFailed, message)
					resultChan <- fmt.Errorf(message)
				}
				return
			case <-ticker.C:
				containers, err := pm.runtime.ListContainers(startupCtx)
				if err != nil {
					message := fmt.Sprintf("Failed to list containers: %v", err)
					pm.updatePodStatus(id, types.PodFailed, message)
					resultChan <- fmt.Errorf("failed to list containers: %w", err)
					return
				}

				// Verify pod still exists
				pm.mu.RLock()
				_, exists := pm.pods[id]
				pm.mu.RUnlock()
				if !exists {
					resultChan <- fmt.Errorf("pod no longer exists")
					return
				}

				// Check if all containers are running
				allRunning := true
				for _, container := range spec.Containers {
					found := false
					for _, c := range containers {
						if c.ID == container.ID {
							found = true
							if c.State != "running" {
								allRunning = false
							}
							break
						}
					}
					if !found {
						allRunning = false
						break
					}
				}

				if allRunning {
					fmt.Printf("All containers running for pod %s\n", id)
					message := "All containers running"
					pm.updatePodStatus(id, types.PodRunning, message)
					resultChan <- nil
					return
				}
			}
		}
	}()

	// Wait for result or context cancellation
	select {
	case err := <-resultChan:
		return err
	case <-ctx.Done():
		message := "Context cancelled while waiting for containers to start"
		pm.updatePodStatus(id, types.PodFailed, message)
		return fmt.Errorf(message)
	}
}

func (pm *podManager) StopPod(ctx context.Context, id string) error {
	// Check pod existence with read lock
	pm.mu.RLock()
	pod, exists := pm.pods[id]
	if !exists {
		pm.mu.RUnlock()
		return ErrPodNotFound
	}
	// Make a copy of the spec while under read lock
	spec := pod.Spec
	currentPhase := pod.Phase
	pm.mu.RUnlock()

	// Only proceed if pod is not already stopped
	if currentPhase == types.PodFailed {
		return nil
	}

	var errs []error
	for _, container := range spec.Containers {
		if err := pm.runtime.StopContainer(ctx, container.ID); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop container %s: %w", container.ID, err))
		}
	}

	// Verify all containers are stopped
	containers, err := pm.runtime.ListContainers(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to list containers: %w", err))
	} else {
		for _, container := range spec.Containers {
			for _, c := range containers {
				if c.ID == container.ID && c.State == "running" {
					errs = append(errs, fmt.Errorf("container %s still running after stop", container.ID))
				}
			}
		}
	}

	if len(errs) > 0 {
		errorMsg := fmt.Sprintf("Failed to stop pod: %v", errs)
		pm.updatePodStatus(id, types.PodFailed, errorMsg)
		return fmt.Errorf("%v", errs)
	}

	pm.updatePodStatus(id, types.PodPending, "Pod stopped")
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

	// Send the updated event
	pm.events <- podEvent{
		podID: id,
		event: "updated",
	}
	return nil
}

func (pm *podManager) applyPodUpdate(ctx context.Context, id string, oldSpec, newSpec types.PodSpec) error {
	fmt.Printf("[DEBUG] Starting pod update for pod %s\n", id)
	
	// Initial validation with lock
	pm.mu.Lock()
	pod, exists := pm.pods[id]
	if !exists {
		pm.mu.Unlock()
		return fmt.Errorf("pod %s not found", id)
	}
	pm.mu.Unlock() // Unlock early to avoid holding lock during container operations

	// Set pod to pending state while we update
	fmt.Printf("[DEBUG] Setting pod %s to pending state\n", id)
	pm.updatePodStatus(id, types.PodPending, "Updating pod specification")

	// Stop and remove old containers
	fmt.Printf("[DEBUG] Starting container operations for pod %s. Old containers count: %d, New containers count: %d\n", 
		id, len(oldSpec.Containers), len(newSpec.Containers))
	
	for _, container := range oldSpec.Containers {
		fmt.Printf("[DEBUG] Processing old container %s\n", container.ID)
		// Stop container
		if err := pm.runtime.StopContainer(ctx, container.ID); err != nil {
			fmt.Printf("[DEBUG] Stop container result for %s: %v\n", container.ID, err)
			if !strings.Contains(err.Error(), "not running") {
				pm.updatePodStatus(id, types.PodFailed, fmt.Sprintf("Failed to stop container %s: %v", container.ID, err))
				return fmt.Errorf("failed to stop container %s: %w", container.ID, err)
			}
		}

		// Remove container
		if err := pm.runtime.RemoveContainer(ctx, container.ID); err != nil {
			fmt.Printf("[DEBUG] Remove container result for %s: %v\n", container.ID, err)
			if !strings.Contains(err.Error(), "not found") {
				pm.updatePodStatus(id, types.PodFailed, fmt.Sprintf("Failed to remove container %s: %v", container.ID, err))
				return fmt.Errorf("failed to remove container %s: %w", container.ID, err)
			}
		}
		fmt.Printf("[DEBUG] Successfully processed old container %s\n", container.ID)
	}

	// Create and start new containers
	for _, container := range newSpec.Containers {
		fmt.Printf("[DEBUG] Processing new container %s\n", container.ID)
		// Create container
		if err := pm.runtime.CreateContainer(ctx, container); err != nil {
			fmt.Printf("[DEBUG] Create container result for %s: %v\n", container.ID, err)
			if !strings.Contains(err.Error(), "already exists") {
				pm.updatePodStatus(id, types.PodFailed, fmt.Sprintf("Failed to create container %s: %v", container.ID, err))
				return fmt.Errorf("failed to create container %s: %w", container.ID, err)
			}
		}

		// Start container
		if err := pm.runtime.StartContainer(ctx, container.ID); err != nil {
			fmt.Printf("[DEBUG] Start container result for %s: %v\n", container.ID, err)
			pm.updatePodStatus(id, types.PodFailed, fmt.Sprintf("Failed to start container %s: %v", container.ID, err))
			return fmt.Errorf("failed to start container %s: %w", container.ID, err)
		}
		fmt.Printf("[DEBUG] Successfully processed new container %s\n", container.ID)
	}

	// Update pod spec
	fmt.Printf("[DEBUG] Updating pod spec in memory for pod %s\n", id)
	pm.mu.Lock()
	pod.Spec = newSpec
	pm.pods[id] = pod
	pm.mu.Unlock()

	fmt.Printf("[DEBUG] Beginning watcher notifications for pod %s\n", id)
	notifiedCount := 0
	pm.watchers.Range(func(key, value interface{}) bool {
		if key.(string) == id {
			ch := value.(chan types.PodStatus)
			select {
			case ch <- *pod:
				notifiedCount++
				fmt.Printf("[DEBUG] Successfully notified watcher %d for pod %s\n", notifiedCount, id)
			default:
				// Try a few more times with a small delay
				go func(ch chan types.PodStatus, status *types.PodStatus) {
					for i := 0; i < 3; i++ {
						time.Sleep(100 * time.Millisecond)
						// Check if channel is closed
						select {
						case _, ok := <-ch:
							if !ok {
								// Channel is closed, stop retrying
								return
							}
							// If we received a value, put it back
							select {
							case ch <- *status:
							default:
							}
						default:
							// Channel is still open, try to send
							select {
							case ch <- *status:
								fmt.Printf("Sent delayed status update to watcher: %s (Message: %s)\n", status.Phase, status.Message)
								return
							default:
								continue
							}
						}
					}
					fmt.Printf("Failed to send status update to watcher after retries for pod %s\n", id)
				}(ch, pod)
			}
		}
		return true
	})
	fmt.Printf("[DEBUG] Completed watcher notifications. Notified %d watchers\n", notifiedCount)

	// Send update event
	fmt.Printf("[DEBUG] Sending update event for pod %s\n", id)
	select {
	case pm.events <- podEvent{podID: id, event: "updated"}:
		fmt.Printf("[DEBUG] Successfully sent update event for pod %s\n", id)
	case <-ctx.Done():
		fmt.Printf("[DEBUG] Context cancelled while sending update event for pod %s\n", id)
		return ctx.Err()
	}

	// Verify all containers are running and update status
	fmt.Printf("[DEBUG] Starting container status check for updated pod %s\n", id)
	containers, err := pm.runtime.ListContainers(ctx)
	if err != nil {
		pm.updatePodStatus(id, types.PodFailed, fmt.Sprintf("Failed to list containers: %v", err))
		return fmt.Errorf("failed to list containers: %w", err)
	}

	allRunning := true
	for _, container := range newSpec.Containers {
		containerRunning := false
		for _, c := range containers {
			if c.ID == container.ID && c.State == "running" {
				containerRunning = true
				break
			}
		}
		if !containerRunning {
			allRunning = false
			fmt.Printf("[DEBUG] Container %s not running\n", container.ID)
			break
		}
	}

	if allRunning {
		pm.updatePodStatus(id, types.PodRunning, "Pod updated successfully")
	} else {
		pm.updatePodStatus(id, types.PodFailed, "Not all containers are running after update")
		return fmt.Errorf("not all containers are running after update")
	}

	return nil
}

func (pm *podManager) WatchPod(ctx context.Context, id string) (<-chan types.PodStatus, error) {
	pm.mu.Lock()
	pod, exists := pm.pods[id]
	if !exists {
		pm.mu.Unlock()
		return nil, fmt.Errorf("pod %s not found", id)
	}
	pm.mu.Unlock()

	// Create buffered channel to avoid blocking
	ch := make(chan types.PodStatus, 1)

	// Send initial status
	select {
	case ch <- *pod:
		fmt.Printf("Sent initial pod status: %s\n", pod.Phase)
	default:
		// Channel is full, which is fine for initial status
	}

	// Store channel in watchers map
	pm.watchers.Store(id, ch)

	// Start cleanup goroutine
	go func() {
		<-ctx.Done()
		pm.mu.Lock()
		if ch, ok := pm.watchers.Load(id); ok {
			pm.watchers.Delete(id)
			if statusCh, ok := ch.(chan types.PodStatus); ok {
				// Only close if not already closed
				select {
				case _, ok := <-statusCh:
					if ok {
						close(statusCh)
					}
				default:
					close(statusCh)
				}
			}
		}
		pm.mu.Unlock()
	}()

	return ch, nil
}

func (pm *podManager) processEvents() {
	for {
		select {
		case event := <-pm.events:
			fmt.Printf("[DEBUG] Processing event %s for pod %s\n", event.event, event.podID)
			
			// Check if pod exists before processing events (except for creation)
			if event.event != "created" {
				pm.mu.RLock()
				pod, exists := pm.pods[event.podID]
				pm.mu.RUnlock()
				if !exists {
					fmt.Printf("[DEBUG] Pod %s not found, skipping event %s\n", event.podID, event.event)
					continue
				}

				// Don't update status if pod is in a terminal state
				if pod.Phase == types.PodFailed {
					fmt.Printf("[DEBUG] Pod %s in failed state, skipping event %s\n", event.podID, event.event)
					continue
				}
			}

			switch event.event {
			case "created":
				pm.updatePodStatus(event.podID, types.PodPending, "")
			case "started":
				pm.updatePodStatus(event.podID, types.PodRunning, "All containers running")
			case "updated":
				fmt.Printf("[DEBUG] Starting container status check for updated pod %s\n", event.podID)
				// For updates, we need to verify all containers are running
				go func(id string) {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					ticker := time.NewTicker(100 * time.Millisecond)
					defer ticker.Stop()

					allRunning := false
					for {
						select {
						case <-ctx.Done():
							fmt.Printf("[DEBUG] Container status check timed out for pod %s\n", id)
							return
						case <-ticker.C:
							containers, err := pm.runtime.ListContainers(ctx)
							if err != nil {
								fmt.Printf("[DEBUG] Failed to list containers for pod %s: %v\n", id, err)
								return
							}

							pm.mu.RLock()
							pod, exists := pm.pods[id]
							if !exists {
								pm.mu.RUnlock()
								fmt.Printf("[DEBUG] Pod %s no longer exists during status check\n", id)
								return
							}
							spec := pod.Spec
							pm.mu.RUnlock()

							allRunning = true
							for _, container := range spec.Containers {
								containerRunning := false
								for _, c := range containers {
									if c.ID == container.ID && c.State == "running" {
										containerRunning = true
										break
									}
								}
								if !containerRunning {
									allRunning = false
									fmt.Printf("[DEBUG] Container %s not running\n", container.ID)
									break
								}
							}

							if allRunning {
								fmt.Printf("[DEBUG] All containers running for pod %s, updating status\n", id)
								pm.updatePodStatus(id, types.PodRunning, "Pod updated successfully")
								return
							}
						}
					}
				}(event.podID)
			case "creation_failed", "container_start_failed":
				pm.updatePodStatus(event.podID, types.PodFailed, event.err.Error())
			case "stopped":
				pm.updatePodStatus(event.podID, types.PodPending, "Pod stopped")
			}
		case <-pm.shutdown:
			fmt.Printf("[DEBUG] Received shutdown signal, stopping event processor\n")
			return
		}
	}
}

func (pm *podManager) updatePodStatus(id string, phase types.PodPhase, message string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pod, exists := pm.pods[id]
	if !exists {
		return
	}

	// Only update if the phase has changed or there's a new message
	if pod.Phase != phase || pod.Message != message {
		pod.Phase = phase
		pod.Message = message
		pm.pods[id] = pod

		// Notify watchers
		if ch, ok := pm.watchers.Load(id); ok {
			select {
			case ch.(chan types.PodStatus) <- *pod:
				fmt.Printf("Sent status update to watcher: %s (Message: %s)\n", phase, message)
			default:
				// Try a few more times with a small delay
				go func(ch chan types.PodStatus, status *types.PodStatus) {
					for i := 0; i < 3; i++ {
						time.Sleep(100 * time.Millisecond)
						// Check if channel is closed
						select {
						case _, ok := <-ch:
							if !ok {
								// Channel is closed, stop retrying
								return
							}
							// If we received a value, put it back
							select {
							case ch <- *status:
							default:
							}
						default:
							// Channel is still open, try to send
							select {
							case ch <- *status:
								fmt.Printf("Sent delayed status update to watcher: %s (Message: %s)\n", phase, message)
								return
							default:
								continue
							}
						}
					}
					fmt.Printf("Failed to send status update to watcher after retries for pod %s\n", id)
				}(ch.(chan types.PodStatus), pod)
			}
		}

		fmt.Printf("Pod %s is %s%s\n", id, strings.ToLower(string(phase)), 
			func() string { 
				if message != "" {
					return fmt.Sprintf(": %s", message)
				}
				return ""
			}())
	}
}
