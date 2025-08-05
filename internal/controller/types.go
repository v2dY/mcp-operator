package controller

import (
	"fmt"
	"strings"
	"time"
)

const (
	// Status conditions
	typeAvailableMCPServer = "Available"
	typeBuildStatus        = "BuildReady"

	// Labels
	labelAppName   = "project"
	labelInstance  = "project"
	labelManagedBy = "mcp-operator"
	labelComponent = "mcp-server"

	// Requeue intervals
	shortRequeue  = 10 * time.Second
	mediumRequeue = 30 * time.Second
	longRequeue   = time.Minute

	// Resource names
	buildJobPrefix = "build"
	operatorName   = "mcp-operator"
)

// ReconcileError represents structured reconciliation errors
type ReconcileError struct {
	Phase   string
	Reason  string
	Message string
	Err     error
}

func (e *ReconcileError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s phase failed (%s): %s - %v", e.Phase, e.Reason, e.Message, e.Err)
	}
	return fmt.Sprintf("%s phase failed (%s): %s", e.Phase, e.Reason, e.Message)
}

// DeploymentChanges represents what changed in the deployment spec
type DeploymentChanges struct {
	Replicas    bool
	PodTemplate bool
	Changes     []string
}

// GetChangesSummary returns a comma-separated summary of changes
func (dc *DeploymentChanges) GetChangesSummary() string {
	return strings.Join(dc.Changes, ", ")
}

// HasChanges returns true if any changes were detected
func (dc *DeploymentChanges) HasChanges() bool {
	return dc.Replicas || dc.PodTemplate
}

// TemplateData represents data for template rendering
type TemplateData struct {
	MCPServerName string // MCPServer name
	Namespace     string // MCPServer namespace
	OpenAPIUrl    string // OpenAPI URL
	BasePath      string // API base path
	Registry      string // Container registry
	ImageName     string // Image name for buildah
}
