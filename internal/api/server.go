package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/comp231-402-Team3-F24/internal/pod"
	"github.com/comp231-402-Team3-F24/pkg/types"
)

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

	return mux
}

// handlePods handles /api/pods endpoints
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

// handlePodRoutes handles all /api/pods/* routes
func (s *Server) handlePodRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/pods/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Handle /api/pods/{id}/logs separately
	if len(parts) >= 2 && parts[1] == "logs" {
		s.handlePodLogs(w, r, parts[0])
		return
	}

	// Regular pod operations
	s.handlePod(w, r)
}

// handlePod handles /api/pods/{id} endpoints
func (s *Server) handlePod(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/pods/")
	if id == "" {
		http.Error(w, "Pod ID required", http.StatusBadRequest)
		return
	}

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

// Pod management handlers
func (s *Server) listPods(w http.ResponseWriter, r *http.Request) {
	pods, err := s.podManager.ListPods(r.Context())
	if err != nil {
		log.Printf("Error listing pods: %v", err)
		http.Error(w, fmt.Sprintf("Failed to list pods: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, pods)
}

func (s *Server) createPod(w http.ResponseWriter, r *http.Request) {
	var spec types.PodSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		log.Printf("Error decoding pod spec: %v", err)
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate pod spec
	if err := validatePodSpec(&spec); err != nil {
		log.Printf("Invalid pod spec: %v", err)
		http.Error(w, fmt.Sprintf("Invalid pod specification: %v", err), http.StatusBadRequest)
		return
	}

	if err := s.podManager.CreatePod(r.Context(), spec); err != nil {
		log.Printf("Error creating pod: %v", err)
		if errors.Is(err, pod.ErrPodAlreadyExists) {
			http.Error(w, "Pod already exists", http.StatusConflict)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to create pod: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

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

	respondJSON(w, retrievedPod)
}

func (s *Server) updatePod(w http.ResponseWriter, r *http.Request, id string) {
	var spec types.PodSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		log.Printf("Error decoding pod spec for update: %v", err)
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate pod spec
	if err := validatePodSpec(&spec); err != nil {
		log.Printf("Invalid pod spec for update: %v", err)
		http.Error(w, fmt.Sprintf("Invalid pod specification: %v", err), http.StatusBadRequest)
		return
	}

	if err := s.podManager.UpdatePod(r.Context(), id, spec); err != nil {
		log.Printf("Error updating pod %s: %v", id, err)
		if errors.Is(err, pod.ErrPodNotFound) {
			http.Error(w, "Pod not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to update pod: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

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

// Container handlers
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

func (s *Server) listContainers(w http.ResponseWriter, r *http.Request) {
	pods, err := s.podManager.ListPods(r.Context())
	if err != nil {
		log.Printf("Error listing containers: %v", err)
		http.Error(w, fmt.Sprintf("Failed to list containers: %v", err), http.StatusInternalServerError)
		return
	}

	var containers []types.ContainerStatus
	for _, userPod := range pods {
		containers = append(containers, userPod.ContainerStatuses...)
	}

	respondJSON(w, containers)
}

func (s *Server) getContainer(w http.ResponseWriter, r *http.Request, id string) {
	pods, err := s.podManager.ListPods(r.Context())
	if err != nil {
		log.Printf("Error getting container %s: %v", id, err)
		http.Error(w, fmt.Sprintf("Failed to get container: %v", err), http.StatusInternalServerError)
		return
	}

	for _, userPod := range pods {
		for _, container := range userPod.ContainerStatuses {
			if container.ID == id {
				respondJSON(w, container)
				return
			}
		}
	}

	http.Error(w, "Container not found", http.StatusNotFound)
}

func (s *Server) deleteContainer(w http.ResponseWriter, r *http.Request, id string) {
	pods, err := s.podManager.ListPods(r.Context())
	if err != nil {
		log.Printf("Error deleting container %s: %v", id, err)
		http.Error(w, fmt.Sprintf("Failed to delete container: %v", err), http.StatusInternalServerError)
		return
	}

	for _, podStatus := range pods {
		for _, container := range podStatus.ContainerStatuses {
			if container.ID == id {
				if err := s.podManager.DeletePod(r.Context(), podStatus.Spec.ID); err != nil {
					log.Printf("Error deleting container %s: %v", id, err)
					http.Error(w, fmt.Sprintf("Failed to delete container: %v", err), http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
	}

	http.Error(w, "Container not found", http.StatusNotFound)
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

func validatePodSpec(spec *types.PodSpec) error {
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
		if container.ID == "" {
			return fmt.Errorf("container %d: ID is required", i)
		}
		if container.Name == "" {
			return fmt.Errorf("container %d: name is required", i)
		}
		if container.Image == "" {
			return fmt.Errorf("container %d: image is required", i)
		}
	}
	return nil
}
