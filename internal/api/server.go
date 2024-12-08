// Package api provides the HTTP API server for managing pods and containers
// @title LitePod API
// @version 1.0
// @description A lightweight container management API for running and managing containers
// @host localhost:8080
// @BasePath /api
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	_ "github.com/comp231-402-Team3-F24/docs"
	"github.com/comp231-402-Team3-F24/internal/pod"
	"github.com/comp231-402-Team3-F24/pkg/types"
	httpSwagger "github.com/swaggo/http-swagger"
)

// swaggerPodSpec defines the desired state of a pod for Swagger documentation
type swaggerPodSpec struct {
	// Unique identifier for the pod
	ID string `json:"id"`
	// Human-readable name for the pod
	Name string `json:"name"`
	// List of containers in the pod
	Containers []swaggerContainerConfig `json:"containers"`
	// Resource requirements/limits for the entire pod
	Resources swaggerResources `json:"resources"`
	// Network configuration
	Network swaggerPodNetworkConfig `json:"network"`
	// Volume mounts shared across containers
	Volumes []swaggerPodVolume `json:"volumes"`
	// Pod-wide environment variables
	Environment map[string]string `json:"environment,omitempty"`
	// Restart policy for containers in the pod
	RestartPolicy string `json:"restartPolicy"`
	// Labels for pod metadata
	Labels map[string]string `json:"labels,omitempty"`
}

// swaggerContainerConfig represents container configuration for Swagger documentation
type swaggerContainerConfig struct {
	// Container name
	Name string `json:"name"`
	// Container image
	Image string `json:"image"`
	// Command to run
	Command []string `json:"command,omitempty"`
	// Environment variables
	Environment map[string]string `json:"environment,omitempty"`
}

// swaggerResources represents resource requirements for Swagger documentation
type swaggerResources struct {
	// CPU limit in cores
	CPU float64 `json:"cpu"`
	// Memory limit in bytes
	Memory int64 `json:"memory"`
}

// swaggerPodNetworkConfig represents network configuration for Swagger documentation
type swaggerPodNetworkConfig struct {
	// Use host network
	HostNetwork bool `json:"hostNetwork,omitempty"`
	// DNS configuration
	DNS swaggerPodDNSConfig `json:"dns,omitempty"`
	// Port mappings
	Ports []swaggerPortMapping `json:"ports,omitempty"`
}

// swaggerPodDNSConfig represents DNS configuration for Swagger documentation
type swaggerPodDNSConfig struct {
	// DNS nameservers
	Nameservers []string `json:"nameservers,omitempty"`
	// DNS search domains
	Searches []string `json:"searches,omitempty"`
	// DNS options
	Options []string `json:"options,omitempty"`
}

// swaggerPortMapping represents port mapping for Swagger documentation
type swaggerPortMapping struct {
	// Port mapping name
	Name string `json:"name,omitempty"`
	// Host port
	HostPort int32 `json:"hostPort"`
	// Container port
	ContainerPort int32 `json:"containerPort"`
	// Protocol (tcp/udp)
	Protocol string `json:"protocol,omitempty"`
}

// swaggerPodVolume represents volume configuration for Swagger documentation
type swaggerPodVolume struct {
	// Volume name
	Name string `json:"name"`
	// Host path
	HostPath string `json:"hostPath,omitempty"`
}

// swaggerPodStatus represents the current state of a pod for Swagger documentation
type swaggerPodStatus struct {
	// Pod specification
	Spec swaggerPodSpec `json:"spec"`
	// Current phase of the pod
	Phase string `json:"phase"`
	// Detailed status message
	Message string `json:"message,omitempty"`
	// Container statuses
	ContainerStatuses []swaggerContainerStatus `json:"containerStatuses"`
	// Pod IP address
	IP string `json:"ip,omitempty"`
	// Important timestamps
	CreatedAt  time.Time `json:"createdAt"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
}

// swaggerContainerStatus represents container status for Swagger documentation
type swaggerContainerStatus struct {
	// Container ID
	ID string `json:"id"`
	// Container name
	Name string `json:"name"`
	// Container state
	State string `json:"state"`
	// Exit code if terminated
	ExitCode int `json:"exitCode,omitempty"`
	// Error message if any
	Error string `json:"error,omitempty"`
	// Container image
	Image string `json:"image"`
	// Container created timestamp
	CreatedAt time.Time `json:"createdAt"`
	// Container started timestamp
	StartedAt time.Time `json:"startedAt,omitempty"`
	// Container finished timestamp
	FinishedAt time.Time `json:"finishedAt,omitempty"`
}

// Server represents the API server
type Server struct {
	podManager pod.Manager
}

// NewServer creates a new API server instance
func NewServer(pm pod.Manager) *Server {
	return &Server{
		podManager: pm,
	}
}

// SetupRoutes configures all API routes
func (s *Server) SetupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// Pod management endpoints
	mux.HandleFunc("/api/pods", s.handlePods)
	mux.HandleFunc("/api/pods/", s.handlePodRoutes)

	// Container endpoints
	mux.HandleFunc("/api/containers", s.handleContainers)
	mux.HandleFunc("/api/containers/", s.handleContainer)

	// Swagger documentation
	mux.HandleFunc("/swagger/", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"), // The URL pointing to API definition
	))

    // Serve static files
    mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

    // Serve HTML templates
    mux.HandleFunc("/dashboard", s.renderTemplate("dashboard.html"))
    mux.HandleFunc("/createPod", s.renderTemplate("podForm.html"))
    mux.HandleFunc("/createContainer", s.renderTemplate("containerForm.html"))
    mux.HandleFunc("/pod/", s.renderTemplate("pod.html"))
    mux.HandleFunc("/container/", s.renderTemplate("container.html"))

    return mux
}

// renderTemplate returns a handler function that renders the specified template
func (s *Server) renderTemplate(templateName string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        tmpl, err := template.ParseFiles("web/templates/" + templateName)
        if (err != nil) {
            http.Error(w, fmt.Sprintf("Error parsing template: %v", err), http.StatusInternalServerError)
            return
        }

        var data interface{}
        switch templateName {
        case "dashboard.html":
            data = s.getDashboardData(r)
        case "pod.html":
            data = s.getPodData(r)
        case "container.html":
            data = s.getContainerData(r)
        }

        if err := tmpl.Execute(w, data); err != nil {
            http.Error(w, fmt.Sprintf("Error executing template: %v", err), http.StatusInternalServerError)
        }
    }
}

func (s *Server) getDashboardData(r *http.Request) interface{} {
    pods, err := s.podManager.ListPods(r.Context())
    if err != nil {
        log.Printf("Error listing pods: %v", err)
        return nil
    }
    return struct {
        Pods []types.PodStatus
    }{Pods: pods}
}

func (s *Server) getPodData(r *http.Request) interface{} {
    podID := strings.TrimPrefix(r.URL.Path, "/pod/")
    pod, err := s.podManager.GetPod(r.Context(), podID)
    if err != nil {
        log.Printf("Error getting pod: %v", err)
        return nil
    }
    return pod
}

func (s *Server) getContainerData(r *http.Request) interface{} {
    containerID := strings.TrimPrefix(r.URL.Path, "/container/")
    pods, err := s.podManager.ListPods(r.Context())
    if err != nil {
        log.Printf("Error listing pods: %v", err)
        return nil
    }
    for _, pod := range pods {
        for _, container := range pod.Spec.Containers {
            if container.ID == containerID {
				return struct {
					PodID         string
					PodName       string
					ContainerID   string
					ContainerName string
					Image 	      string
					Environment   map[string]string
				}{
					PodID:         pod.Spec.ID,
					PodName:       pod.Spec.Name,
					ContainerID:   container.ID,
					ContainerName: container.Name,
					Image:        container.Image,
					Environment:   container.Environment,
				}
            }
        }
    }
    return nil
}

// @Summary      List all pods
// @Description  Get a list of all pods
// @Tags         pods
// @Accept       json
// @Produce      json
// @Success      200  {array}   swaggerPodStatus
// @Failure      500  {object}  string "Internal Server Error"
// @Router       /pods [get]
// @Summary      Create pod
// @Description  Create a new pod
// @Tags         pods
// @Accept       json
// @Produce      json
// @Param        pod  body      swaggerPodSpec  true  "Pod specification"
// @Success      201  {object}  swaggerPodStatus
// @Failure      400  {object}  string "Invalid request"
// @Failure      500  {object}  string "Internal Server Error"
// @Router       /pods [post]
func (s *Server) handlePods(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listPods(w, r)
	case http.MethodPost:
		s.createPod(w, r)
	case http.MethodOptions:
		s.handleOptions(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// @Summary      Get pod details
// @Description  Get detailed information about a specific pod
// @Tags         pods
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Pod ID"
// @Success      200  {object}  swaggerPodStatus
// @Failure      404  {object}  string "Pod not found"
// @Failure      500  {object}  string "Internal Server Error"
// @Router       /pods/{id} [get]
// @Summary      Update pod
// @Description  Update an existing pod
// @Tags         pods
// @Accept       json
// @Produce      json
// @Param        id   path      string        true  "Pod ID"
// @Param        pod  body      swaggerPodSpec  true  "Updated pod specification"
// @Success      200  {object}  swaggerPodStatus
// @Failure      400  {object}  string "Invalid request"
// @Failure      404  {object}  string "Pod not found"
// @Failure      500  {object}  string "Internal Server Error"
// @Router       /pods/{id} [put]
// @Summary      Delete pod
// @Description  Delete a specific pod
// @Tags         pods
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Pod ID"
// @Success      204  {object}  nil     "No Content"
// @Failure      404  {object}  string "Pod not found"
// @Failure      500  {object}  string "Internal Server Error"
// @Router       /pods/{id} [delete]
func (s *Server) handlePodRoutes(w http.ResponseWriter, r *http.Request) {
	// Extract pod ID from URL path
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "Invalid URL path", http.StatusBadRequest)
		return
	}

	id := parts[3]
	if id == "" {
		http.Error(w, "Pod ID is required", http.StatusBadRequest)
		return
	}

	// Handle /pods/{id}/logs separately
	if len(parts) == 5 && parts[4] == "logs" {
		if r.Method == http.MethodGet {
			s.handlePodLogs(w, r, id)
			return
		}
		http.Error(w, "Method not allowed for logs endpoint", http.StatusMethodNotAllowed)
		return
	}

	// Handle /pods/{id} endpoints
	switch r.Method {
	case http.MethodGet:
		s.getPod(w, r, id)
	case http.MethodPut:
		s.updatePod(w, r, id)
	case http.MethodDelete:
		s.deletePod(w, r, id)
	case http.MethodOptions:
		s.handleOptions(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// @Summary      List all containers
// @Description  Get a list of all containers across all pods
// @Tags         containers
// @Accept       json
// @Produce      json
// @Success      200  {array}   swaggerContainerStatus
// @Failure      500  {object}  string "Internal Server Error"
// @Router       /containers [get]
func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listContainers(w, r)
	case http.MethodOptions:
		s.handleOptions(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// @Summary      Get container details
// @Description  Get detailed information about a specific container
// @Tags         containers
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Container ID"
// @Success      200  {object}  swaggerContainerStatus
// @Failure      404  {object}  string "Container not found"
// @Failure      500  {object}  string "Internal Server Error"
// @Router       /containers/{id} [get]
func (s *Server) handleContainer(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/containers/")
	if id == "" {
		http.Error(w, "Container ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getContainer(w, r, id)
	case http.MethodDelete:
		s.deleteContainer(w, r, id)
	case http.MethodOptions:
		s.handleOptions(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// @Summary      List all pods
// @Description  Get a list of all pods
// @Tags         pods
// @Accept       json
// @Produce      json
// @Success      200  {array}   swaggerPodStatus
// @Failure      500  {object}  string "Internal Server Error"
// @Router       /pods [get]
func (s *Server) listPods(w http.ResponseWriter, r *http.Request) {
	pods, err := s.podManager.ListPods(r.Context())
	if err != nil {
		log.Printf("Error listing pods: %v", err)
		http.Error(w, fmt.Sprintf("Failed to list pods: %v", err), http.StatusInternalServerError)
		return
	}

	var podStatuses []swaggerPodStatus
	for _, pod := range pods {
		podStatuses = append(podStatuses, convertPodStatusToSwagger(pod))
	}

	respondJSON(w, podStatuses)
}

// @Summary      Create pod
// @Description  Create a new pod
// @Tags         pods
// @Accept       json
// @Produce      json
// @Param        pod  body      swaggerPodSpec  true  "Pod specification"
// @Success      201  {object}  swaggerPodStatus
// @Failure      400  {object}  string "Invalid request"
// @Failure      500  {object}  string "Internal Server Error"
// @Router       /pods [post]
func (s *Server) createPod(w http.ResponseWriter, r *http.Request) {
	var swaggerSpec swaggerPodSpec
	if err := json.NewDecoder(r.Body).Decode(&swaggerSpec); err != nil {
		log.Printf("Error decoding pod spec: %v", err)
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if err := validatePodSpec(&swaggerSpec); err != nil {
		log.Printf("Invalid pod spec: %v", err)
		http.Error(w, fmt.Sprintf("Invalid pod specification: %v", err), http.StatusBadRequest)
		return
	}

	internalSpec := convertSwaggerPodSpecToInternal(swaggerSpec)
	status, err := s.podManager.CreatePod(r.Context(), internalSpec)
	if err != nil {
		log.Printf("Error creating pod: %v", err)
		if errors.Is(err, pod.ErrPodAlreadyExists) {
			http.Error(w, "Pod already exists", http.StatusConflict)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to create pod: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	respondJSON(w, convertPodStatusToSwagger(*status))
}

// @Summary      Get pod details
// @Description  Get detailed information about a specific pod
// @Tags         pods
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Pod ID"
// @Success      200  {object}  swaggerPodStatus
// @Failure      404  {object}  string "Pod not found"
// @Failure      500  {object}  string "Internal Server Error"
// @Router       /pods/{id} [get]
func (s *Server) getPod(w http.ResponseWriter, r *http.Request, id string) {
	retrievedPod, err := s.podManager.GetPod(r.Context(), id)
	if err != nil {
		log.Printf("Error getting retrievedPod %s: %v", id, err)
		if errors.Is(err, pod.ErrPodNotFound) {
			http.Error(w, "Pod not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to get retrievedPod: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, convertPodStatusToSwagger(*retrievedPod))
}

// @Summary      Update pod
// @Description  Update an existing pod
// @Tags         pods
// @Accept       json
// @Produce      json
// @Param        id   path      string        true  "Pod ID"
// @Param        pod  body      swaggerPodSpec  true  "Updated pod specification"
// @Success      200  {object}  swaggerPodStatus
// @Failure      400  {object}  string "Invalid request"
// @Failure      404  {object}  string "Pod not found"
// @Failure      500  {object}  string "Internal Server Error"
// @Router       /pods/{id} [put]
func (s *Server) updatePod(w http.ResponseWriter, r *http.Request, id string) {
	var swaggerSpec swaggerPodSpec
	if err := json.NewDecoder(r.Body).Decode(&swaggerSpec); err != nil {
		log.Printf("Error decoding pod spec for update: %v", err)
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	internalSpec := convertSwaggerPodSpecToInternal(swaggerSpec)

	if err := s.podManager.UpdatePod(r.Context(), id, internalSpec); err != nil {
		log.Printf("Error updating pod %s: %v", id, err)
		if errors.Is(err, pod.ErrPodNotFound) {
			http.Error(w, "Pod not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to update pod: %v", err), http.StatusInternalServerError)
		return
	}

	// If you want to return the updated pod status, you'll need to fetch it separately
	status, err := s.podManager.GetPod(r.Context(), id)
	if err != nil {
		log.Printf("Error retrieving updated pod %s: %v", id, err)
		http.Error(w, fmt.Sprintf("Failed to retrieve updated pod: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, convertPodStatusToSwagger(*status))
}

// @Summary      Delete pod
// @Description  Delete a specific pod
// @Tags         pods
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Pod ID"
// @Success      204  {object}  nil     "No Content"
// @Failure      404  {object}  string "Pod not found"
// @Failure      500  {object}  string "Internal Server Error"
// @Router       /pods/{id} [delete]
func (s *Server) deletePod(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.podManager.DeletePod(r.Context(), id); err != nil {
		log.Printf("Error deleting pod %s: %v", id, err)
		if errors.Is(err, pod.ErrPodNotFound) {
			http.Error(w, "Pod not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to delete pod: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// @Summary      Get pod logs
// @Description  Get logs from all containers in a pod
// @Tags         pods
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Pod ID"
// @Success      200  {object}  map[string]string
// @Failure      404  {object}  string "Pod not found"
// @Failure      500  {object}  string "Internal Server Error"
// @Router       /pods/{id}/logs [get]
func (s *Server) handlePodLogs(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse log options from query parameters
	opts := pod.LogOptions{
		Follow:     r.URL.Query().Get("follow") == "true",
		Timestamps: r.URL.Query().Get("timestamps") == "true",
	}

	if tailStr := r.URL.Query().Get("tail"); tailStr != "" {
		var tailLines int64
		if _, err := fmt.Sscanf(tailStr, "%d", &tailLines); err == nil {
			opts.TailLines = &tailLines
		}
	}

	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			opts.SinceTime = &t
		}
	}

	logs, err := s.podManager.GetPodLogs(r.Context(), id, opts)
	if err != nil {
		log.Printf("Error getting logs for pod %s: %v", id, err)
		if errors.Is(err, pod.ErrPodNotFound) {
			http.Error(w, "Pod not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to get pod logs: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, logs)
}

// @Summary      List all containers
// @Description  Get a list of all containers across all pods
// @Tags         containers
// @Accept       json
// @Produce      json
// @Success      200  {array}   swaggerContainerStatus
// @Failure      500  {object}  string "Internal Server Error"
// @Router       /containers [get]
func (s *Server) listContainers(w http.ResponseWriter, r *http.Request) {
	pods, err := s.podManager.ListPods(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list pods: %v", err), http.StatusInternalServerError)
		return
	}

	var containerStatuses []swaggerContainerStatus
	for _, pod := range pods {
		for _, status := range pod.ContainerStatuses {
			containerStatuses = append(containerStatuses, convertContainerStatusToSwagger(status))
		}
	}

	respondJSON(w, containerStatuses)
}

// @Summary      Get container details
// @Description  Get detailed information about a specific container
// @Tags         containers
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Container ID"
// @Success      200  {object}  swaggerContainerStatus
// @Failure      404  {object}  string "Container not found"
// @Failure      500  {object}  string "Internal Server Error"
// @Router       /containers/{id} [get]
func (s *Server) getContainer(w http.ResponseWriter, r *http.Request, id string) {
	pods, err := s.podManager.ListPods(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list pods: %v", err), http.StatusInternalServerError)
		return
	}

	for _, pod := range pods {
		for _, status := range pod.ContainerStatuses {
			if status.ID == id {
				respondJSON(w, convertContainerStatusToSwagger(status))
				return
			}
		}
	}

	http.Error(w, fmt.Sprintf("Container not found: %s", id), http.StatusNotFound)
}

// @Summary      Delete container
// @Description  Delete a specific container
// @Tags         containers
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Container ID"
// @Success      204  {object}  nil     "No Content"
// @Failure      404  {object}  string "Container not found"
// @Failure      500  {object}  string "Internal Server Error"
// @Router       /containers/{id} [delete]
func (s *Server) deleteContainer(w http.ResponseWriter, r *http.Request, id string) {
	pods, err := s.podManager.ListPods(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list pods: %v", err), http.StatusInternalServerError)
		return
	}

	for _, pod := range pods {
		for _, container := range pod.ContainerStatuses {
			if container.ID == id {
				// Since containers are managed as part of pods, we need to update the pod
				// to remove the container
				updatedSpec := pod.Spec
				var updatedContainers []types.ContainerConfig
				for _, c := range updatedSpec.Containers {
					if c.Name != container.Name {
						updatedContainers = append(updatedContainers, types.ContainerConfig{
							ID:          c.ID,
							Name:        c.Name,
							Image:       c.Image,
							Command:     c.Command,
							Environment: c.Environment,
							Mounts:      c.Mounts,
							Resources:   c.Resources,
						})
					}
				}
				updatedSpec.Containers = updatedContainers

				err = s.podManager.UpdatePod(r.Context(), pod.Spec.ID, updatedSpec)
				if err != nil {
					http.Error(w, fmt.Sprintf("Failed to delete container: %v", err), http.StatusInternalServerError)
					return
				}

				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
	}

	http.Error(w, fmt.Sprintf("Container not found: %s", id), http.StatusNotFound)
}

// Utility functions
func (s *Server) handleOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.WriteHeader(http.StatusOK)
}

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
	}
}

func validatePodSpec(spec *swaggerPodSpec) error {
	if spec.ID == "" {
		return fmt.Errorf("pod ID is required")
	}
	if spec.Name == "" {
		return fmt.Errorf("pod name is required")
	}
	if len(spec.Containers) == 0 {
		return fmt.Errorf("at least one container is required")
	}
	for i, container := range spec.Containers {
		if container.Name == "" {
			return fmt.Errorf("container %d: name is required", i)
		}
		if container.Image == "" {
			return fmt.Errorf("container %d: image is required", i)
		}
	}
	return nil
}

// Convert swaggerPodSpec to types.PodSpec
func convertSwaggerPodSpecToInternal(spec swaggerPodSpec) types.PodSpec {
	containers := make([]types.ContainerConfig, len(spec.Containers))
	for i, c := range spec.Containers {
		containers[i] = types.ContainerConfig{
			Name:        c.Name,
			Image:       c.Image,
			Command:     c.Command,
			Environment: c.Environment,
		}
	}

	return types.PodSpec{
		ID:            spec.ID,
		Name:          spec.Name,
		Containers:    containers,
		RestartPolicy: types.RestartPolicy(spec.RestartPolicy),
		Labels:        spec.Labels,
		Environment:   spec.Environment,
	}
}

// Convert types.ContainerStatus to swaggerContainerStatus
func convertContainerStatusToSwagger(status types.ContainerStatus) swaggerContainerStatus {
	return swaggerContainerStatus{
		ID:         status.ID,
		Name:       status.Name,
		State:      status.State,
		ExitCode:   status.ExitCode,
		Error:      status.Error,
		CreatedAt:  status.CreatedAt,
		StartedAt:  status.StartedAt,
		FinishedAt: status.FinishedAt,
	}
}

// Convert types.PodStatus to swaggerPodStatus
func convertPodStatusToSwagger(status types.PodStatus) swaggerPodStatus {
	containerStatuses := make([]swaggerContainerStatus, len(status.ContainerStatuses))
	for i, cs := range status.ContainerStatuses {
		containerStatuses[i] = convertContainerStatusToSwagger(cs)
	}

	return swaggerPodStatus{
		Spec:              convertPodSpecToSwagger(status.Spec),
		Phase:             string(status.Phase),
		Message:           status.Message,
		ContainerStatuses: containerStatuses,
		IP:                status.IP,
		CreatedAt:         status.CreatedAt,
		StartedAt:         status.StartedAt,
		FinishedAt:        status.FinishedAt,
	}
}

// Convert types.PodSpec to swaggerPodSpec
func convertPodSpecToSwagger(spec types.PodSpec) swaggerPodSpec {
	containers := make([]swaggerContainerConfig, len(spec.Containers))
	for i, c := range spec.Containers {
		containers[i] = swaggerContainerConfig{
			Name:        c.Name,
			Image:       c.Image,
			Command:     c.Command,
			Environment: c.Environment,
		}
	}

	return swaggerPodSpec{
		ID:            spec.ID,
		Name:          spec.Name,
		Containers:    containers,
		RestartPolicy: string(spec.RestartPolicy),
		Labels:        spec.Labels,
		Environment:   spec.Environment,
	}
}
