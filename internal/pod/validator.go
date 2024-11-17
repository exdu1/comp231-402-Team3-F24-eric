package pod

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/comp231-402-Team3-F24/pkg/types"
)

// ValidationError represents a pod specification validation error
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface
func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidateSpec performs complete validation of a pod specification
func ValidateSpec(spec *types.PodSpec) error {
	var errors []string

	// Validate basic pod metadata
	if err := validateMetadata(spec); err != nil {
		errors = append(errors, err.Error())
	}

	// Validate containers
	if err := validateContainers(spec); err != nil {
		errors = append(errors, err.Error())
	}

	// Validate resources
	if err := validateResources(spec); err != nil {
		errors = append(errors, err.Error())
	}

	// Validate network configuration
	if err := validateNetwork(spec); err != nil {
		errors = append(errors, err.Error())
	}

	// Validate volumes
	if err := validateVolumes(spec); err != nil {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		return ValidationError{
			Field:   "PodSpec",
			Message: strings.Join(errors, "; "),
		}
	}

	return nil
}

func validateMetadata(spec *types.PodSpec) error {
	if spec.ID == "" {
		return ValidationError{
			Field:   "ID",
			Message: "must not be empty",
		}
	}

	if !isValidIdentifier(spec.ID) {
		return ValidationError{
			Field:   "ID",
			Message: "must contain only lowercase letters, numbers, and hyphens",
		}
	}

	if spec.Name == "" {
		return ValidationError{
			Field:   "Name",
			Message: "must not be empty",
		}
	}

	// Validate labels if present
	for k, v := range spec.Labels {
		if !isValidLabelKey(k) {
			return ValidationError{
				Field:   fmt.Sprintf("Labels[%s]", k),
				Message: "invalid label key format",
			}
		}
		if !isValidLabelValue(v) {
			return ValidationError{
				Field:   fmt.Sprintf("Labels[%s]", k),
				Message: "invalid label value format",
			}
		}
	}

	return nil
}

func validateContainers(spec *types.PodSpec) error {
	if len(spec.Containers) == 0 {
		return ValidationError{
			Field:   "Containers",
			Message: "at least one container is required",
		}
	}

	seenNames := make(map[string]bool)
	for i, container := range spec.Containers {
		if container.Name == "" {
			return ValidationError{
				Field:   fmt.Sprintf("Containers[%d].Name", i),
				Message: "container name must not be empty",
			}
		}

		if seenNames[container.Name] {
			return ValidationError{
				Field:   fmt.Sprintf("Containers[%d].Name", i),
				Message: fmt.Sprintf("duplicate container name: %s", container.Name),
			}
		}
		seenNames[container.Name] = true

		if container.Image == "" {
			return ValidationError{
				Field:   fmt.Sprintf("Containers[%d].Image", i),
				Message: "container image must not be empty",
			}
		}
	}

	return nil
}

func validateResources(spec *types.PodSpec) error {
	if spec.Resources.CPUShares > 0 && spec.Resources.CPUShares < 2 {
		return ValidationError{
			Field:   "Resources.CPUShares",
			Message: "must be at least 2 if specified",
		}
	}

	if spec.Resources.MemoryMB < 0 {
		return ValidationError{
			Field:   "Resources.MemoryMB",
			Message: "cannot be negative",
		}
	}

	return nil
}

func validateNetwork(spec *types.PodSpec) error {
	// Validate DNS configuration
	for _, ns := range spec.Network.DNS.Nameservers {
		if ip := net.ParseIP(ns); ip == nil {
			return ValidationError{
				Field:   "Network.DNS.Nameservers",
				Message: fmt.Sprintf("invalid IP address: %s", ns),
			}
		}
	}

	// Validate port mappings
	seenPorts := make(map[int32]bool)
	for i, port := range spec.Network.Ports {
		if port.HostPort < 0 || port.HostPort > 65535 {
			return ValidationError{
				Field:   fmt.Sprintf("Network.Ports[%d].HostPort", i),
				Message: "must be between 0 and 65535",
			}
		}

		if port.ContainerPort < 0 || port.ContainerPort > 65535 {
			return ValidationError{
				Field:   fmt.Sprintf("Network.Ports[%d].ContainerPort", i),
				Message: "must be between 0 and 65535",
			}
		}

		if seenPorts[port.HostPort] {
			return ValidationError{
				Field:   fmt.Sprintf("Network.Ports[%d].HostPort", i),
				Message: fmt.Sprintf("duplicate host port: %d", port.HostPort),
			}
		}
		seenPorts[port.HostPort] = true
	}

	return nil
}

func validateVolumes(spec *types.PodSpec) error {
	seenNames := make(map[string]bool)
	for i, volume := range spec.Volumes {
		if volume.Name == "" {
			return ValidationError{
				Field:   fmt.Sprintf("Volumes[%d].Name", i),
				Message: "volume name must not be empty",
			}
		}

		if seenNames[volume.Name] {
			return ValidationError{
				Field:   fmt.Sprintf("Volumes[%d].Name", i),
				Message: fmt.Sprintf("duplicate volume name: %s", volume.Name),
			}
		}
		seenNames[volume.Name] = true

		if volume.HostPath != "" {
			if !isValidPath(volume.HostPath) {
				return ValidationError{
					Field:   fmt.Sprintf("Volumes[%d].HostPath", i),
					Message: "invalid host path",
				}
			}
		}
	}

	return nil
}

// Helper functions for validation
var (
	identifierRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)
	labelKeyRegex   = regexp.MustCompile(`^[a-z0-9A-Z]([a-z0-9A-Z-_./]*[a-z0-9A-Z])?$`)
	labelValueRegex = regexp.MustCompile(`^[a-z0-9A-Z]([a-z0-9A-Z-_.]*[a-z0-9A-Z])?$`)
	pathRegex       = regexp.MustCompile(`^(/[a-zA-Z0-9._-]+)+$`)
)

func isValidIdentifier(s string) bool {
	return identifierRegex.MatchString(s)
}

func isValidLabelKey(s string) bool {
	return labelKeyRegex.MatchString(s)
}

func isValidLabelValue(s string) bool {
	return labelValueRegex.MatchString(s)
}

func isValidPath(s string) bool {
	return pathRegex.MatchString(s)
}
