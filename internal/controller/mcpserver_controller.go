/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	kagentv1alpha1 "github.com/kagent-dev/kagent/go/controller/api/v1alpha1"
	mcpv1 "github.com/v2dY/project/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// MCPServerReconciler reconciles a MCPServer object
type MCPServerReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	buildManager    *BuildManager
	resourceBuilder *ResourceBuilder
	statusManager   *StatusManager
}

// +kubebuilder:rbac:groups=mcp.v2dy.github.io,resources=mcpservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mcp.v2dy.github.io,resources=mcpservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mcp.v2dy.github.io,resources=mcpservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=kagent.dev,resources=toolservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kagent.dev,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MCPServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the MCPServer instance
	mcpServer := &mcpv1.MCPServer{}
	if err := r.Get(ctx, req.NamespacedName, mcpServer); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("MCPServer resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get MCPServer")
		return ctrl.Result{}, err
	}

	// Initialize status if needed
	if result, err := r.statusManager.InitializeStatus(ctx, mcpServer, req); err != nil {
		return result, err
	}

	// Phase 1: Handle build
	if result, err := r.reconcileBuildPhase(ctx, mcpServer); err != nil || result.RequeueAfter > 0 {
		return result, err
	}

	// Phase 2: Handle deployment
	if result, err := r.reconcileDeploymentPhase(ctx, mcpServer, req); err != nil || result.RequeueAfter > 0 {
		return result, err
	}

	// Phase 3: Handle service
	if result, err := r.reconcileServicePhase(ctx, mcpServer); err != nil || result.RequeueAfter > 0 {
		return result, err
	}

	// Phase 4: Handle Kagent integration (ToolServer and Agent)
	if result, err := r.reconcileKagentPhase(ctx, mcpServer); err != nil || result.RequeueAfter > 0 {
		return result, err
	}

	// Update final status
	return r.statusManager.UpdateFinalStatus(ctx, mcpServer)
}

// reconcileBuildPhase handles the build phase
func (r *MCPServerReconciler) reconcileBuildPhase(ctx context.Context, mcpServer *mcpv1.MCPServer) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Skip build phase if no registry is specified (using PrebuiltBuilder)
	if mcpServer.Spec.Registry == "" {
		log.Info("Skipping build phase - using prebuilt image")
		// Mark build as complete for prebuilt images
		meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{
			Type: typeBuildStatus, Status: metav1.ConditionTrue,
			Reason: "BuildSkipped", Message: "Build skipped - using prebuilt image"})
		return ctrl.Result{}, nil
	}

	buildCompleted, requeueAfter, err := r.buildManager.ReconcileBuild(ctx, mcpServer)
	if err != nil {
		recErr := &ReconcileError{
			Phase:   "Build",
			Reason:  "BuildFailed",
			Message: fmt.Sprintf("Build failed: %s", err),
			Err:     err,
		}
		return ctrl.Result{}, r.statusManager.HandleReconcileError(ctx, mcpServer, recErr)
	}

	if !buildCompleted {
		meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{
			Type: typeBuildStatus, Status: metav1.ConditionFalse,
			Reason: "Building", Message: "Build in progress"})

		if err := r.Status().Update(ctx, mcpServer); err != nil {
			log.Error(err, "Failed to update build status")
		}

		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// Mark build as complete
	meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{
		Type: typeBuildStatus, Status: metav1.ConditionTrue,
		Reason: "BuildComplete", Message: "Build completed successfully"})

	log.Info("Build phase completed successfully")
	return ctrl.Result{}, nil
}

// reconcileDeploymentPhase handles the deployment phase
func (r *MCPServerReconciler) reconcileDeploymentPhase(ctx context.Context, mcpServer *mcpv1.MCPServer, req ctrl.Request) (ctrl.Result, error) {
	// Check if deployment exists
	found := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, found)

	if err != nil && apierrors.IsNotFound(err) {
		return r.createDeployment(ctx, mcpServer)
	} else if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get deployment: %w", err)
	}

	// Deployment exists, check if it needs updates
	return r.reconcileExistingDeployment(ctx, mcpServer, req)
}

// createDeployment creates a new deployment
func (r *MCPServerReconciler) createDeployment(ctx context.Context, mcpServer *mcpv1.MCPServer) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	dep, err := r.resourceBuilder.BuildDeployment(mcpServer)
	if err != nil {
		recErr := &ReconcileError{
			Phase:   "Deployment",
			Reason:  "DeploymentCreationFailed",
			Message: fmt.Sprintf("Failed to build deployment: %s", err),
			Err:     err,
		}
		return ctrl.Result{}, r.statusManager.HandleReconcileError(ctx, mcpServer, recErr)
	}

	log.Info("Creating new deployment", "deployment", dep.Name)
	if err = r.Create(ctx, dep); err != nil {
		recErr := &ReconcileError{
			Phase:   "Deployment",
			Reason:  "DeploymentCreationFailed",
			Message: fmt.Sprintf("Failed to create deployment: %s", err),
			Err:     err,
		}
		return ctrl.Result{}, r.statusManager.HandleReconcileError(ctx, mcpServer, recErr)
	}

	return ctrl.Result{RequeueAfter: mediumRequeue}, nil
}

// reconcileServicePhase handles the service phase
func (r *MCPServerReconciler) reconcileServicePhase(ctx context.Context, mcpServer *mcpv1.MCPServer) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	service := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, service)

	if err != nil && apierrors.IsNotFound(err) {
		svc, err := r.resourceBuilder.BuildService(mcpServer)
		if err != nil {
			recErr := &ReconcileError{
				Phase:   "Service",
				Reason:  "ServiceCreationFailed",
				Message: fmt.Sprintf("Failed to define service: %s", err),
				Err:     err,
			}
			return ctrl.Result{}, r.statusManager.HandleReconcileError(ctx, mcpServer, recErr)
		}

		log.Info("Creating new service", "service", svc.Name)
		if err = r.Create(ctx, svc); err != nil {
			recErr := &ReconcileError{
				Phase:   "Service",
				Reason:  "ServiceCreationFailed",
				Message: fmt.Sprintf("Failed to create service: %s", err),
				Err:     err,
			}
			return ctrl.Result{}, r.statusManager.HandleReconcileError(ctx, mcpServer, recErr)
		}
		return ctrl.Result{RequeueAfter: mediumRequeue}, nil
	} else if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get service: %w", err)
	}

	return ctrl.Result{}, nil
}

// reconcileExistingDeployment reconciles an existing deployment
func (r *MCPServerReconciler) reconcileExistingDeployment(ctx context.Context, mcpServer *mcpv1.MCPServer, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Create desired deployment
	dep, err := r.resourceBuilder.BuildDeployment(mcpServer)
	if err != nil {
		recErr := &ReconcileError{
			Phase:   "Deployment",
			Reason:  "DeploymentBuildFailed",
			Message: fmt.Sprintf("Failed to build deployment: %s", err),
			Err:     err,
		}
		return ctrl.Result{}, r.statusManager.HandleReconcileError(ctx, mcpServer, recErr)
	}

	// Get current deployment
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, found)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get existing deployment: %w", err)
	}

	// Detect what changed
	changes := DetectDeploymentChanges(dep, found)

	// If no changes, we're done
	if !changes.HasChanges() {
		log.V(1).Info("No deployment changes detected", "deployment", found.Name)
		return ctrl.Result{}, nil
	}

	// Apply changes and update
	return r.applyDeploymentChanges(ctx, mcpServer, req, found, dep, changes)
}

// applyDeploymentChanges applies detected changes to the deployment
func (r *MCPServerReconciler) applyDeploymentChanges(ctx context.Context, mcpServer *mcpv1.MCPServer, req ctrl.Request, found, desired *appsv1.Deployment, changes *DeploymentChanges) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Log detected changes
	log.Info("Detected deployment changes",
		"changes", changes.GetChangesSummary(),
		"deployment", found.Name)

	// Apply changes to existing deployment
	found.Spec.Replicas = desired.Spec.Replicas
	found.Spec.Template = desired.Spec.Template

	// Update deployment
	if err := r.Update(ctx, found); err != nil {
		// Re-fetch the mcpServer Custom Resource before updating the status
		if err := r.Get(ctx, req.NamespacedName, mcpServer); err != nil {
			log.Error(err, "Failed to re-fetch mcpServer")
			return ctrl.Result{}, err
		}

		reason := "UpdateFailed"
		if changes.Replicas && !changes.PodTemplate {
			reason = "ResizeFailed"
		} else if changes.PodTemplate {
			reason = "ConfigUpdateFailed"
		}

		recErr := &ReconcileError{
			Phase:   "Deployment",
			Reason:  reason,
			Message: fmt.Sprintf("Failed to update deployment: %s. Changes attempted: %s", err.Error(), changes.GetChangesSummary()),
			Err:     err,
		}
		return ctrl.Result{}, r.statusManager.HandleReconcileError(ctx, mcpServer, recErr)
	}

	// Success - log what was updated
	log.Info("Successfully updated deployment",
		"deployment", found.Name,
		"changes", changes.GetChangesSummary())

	return ctrl.Result{RequeueAfter: shortRequeue}, nil
}

// reconcileKagentPhase handles the Kagent integration phase
func (r *MCPServerReconciler) reconcileKagentPhase(ctx context.Context, mcpServer *mcpv1.MCPServer) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Skip if Kagent integration is not requested
	if mcpServer.Spec.Kagent == nil {
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling Kagent integration", "mcpserver", mcpServer.Name)

	// Phase 4a: Create/Update ToolServer
	if result, err := r.reconcileToolServer(ctx, mcpServer); err != nil || result.RequeueAfter > 0 {
		return result, err
	}

	// Phase 4b: Create/Update Agent
	if result, err := r.reconcileAgent(ctx, mcpServer); err != nil || result.RequeueAfter > 0 {
		return result, err
	}

	return ctrl.Result{}, nil
}

// reconcileToolServer creates or updates the ToolServer for Kagent integration
func (r *MCPServerReconciler) reconcileToolServer(ctx context.Context, mcpServer *mcpv1.MCPServer) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Generate ToolServer name
	toolServerName := mcpServer.Name + "-toolserver"
	
	// Create the desired ToolServer
	desired := &kagentv1alpha1.ToolServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      toolServerName,
			Namespace: mcpServer.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "toolserver",
				"app.kubernetes.io/instance":   toolServerName,
				"app.kubernetes.io/component":  "kagent-integration",
				"app.kubernetes.io/part-of":    "mcp-operator",
				"app.kubernetes.io/managed-by": "mcp-operator",
			"mcp.v2dy.github.io/mcpserver": mcpServer.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: mcpServer.APIVersion,
					Kind:       mcpServer.Kind,
					Name:       mcpServer.Name,
					UID:        mcpServer.UID,
					Controller: &[]bool{true}[0],
				},
			},
		},
		Spec: kagentv1alpha1.ToolServerSpec{
			Description: fmt.Sprintf("ToolServer for MCP Server %s", mcpServer.Name),
			Config: kagentv1alpha1.ToolServerConfig{
				StreamableHttp: &kagentv1alpha1.StreamableHttpServerConfig{
					HttpToolServerConfig: kagentv1alpha1.HttpToolServerConfig{
						URL: fmt.Sprintf("http://%s.%s.svc.cluster.local:3001/mcp", 
							mcpServer.Name, mcpServer.Namespace),
					},
				},
			},
		},
	}

	// Check if ToolServer already exists
	existing := &kagentv1alpha1.ToolServer{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      toolServerName,
		Namespace: mcpServer.Namespace,
	}, existing)

	if err != nil && apierrors.IsNotFound(err) {
		// Create new ToolServer
		log.Info("Creating ToolServer", "toolserver", toolServerName)
		if err := r.Create(ctx, desired); err != nil {
			log.Error(err, "Failed to create ToolServer", "toolserver", toolServerName)
			return ctrl.Result{}, err
		}
		log.Info("Successfully created ToolServer", "toolserver", toolServerName)
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Failed to get ToolServer", "toolserver", toolServerName)
		return ctrl.Result{}, err
	}

	// Update existing ToolServer if needed
	if existing.Spec.Description != desired.Spec.Description ||
		existing.Spec.Config.StreamableHttp == nil ||
		existing.Spec.Config.StreamableHttp.URL != desired.Spec.Config.StreamableHttp.URL {
		
		existing.Spec = desired.Spec
		existing.Labels = desired.Labels
		
		log.Info("Updating ToolServer", "toolserver", toolServerName)
		if err := r.Update(ctx, existing); err != nil {
			log.Error(err, "Failed to update ToolServer", "toolserver", toolServerName)
			return ctrl.Result{}, err
		}
		log.Info("Successfully updated ToolServer", "toolserver", toolServerName)
	}

	return ctrl.Result{}, nil
}

// reconcileAgent creates or updates the Agent for Kagent integration
func (r *MCPServerReconciler) reconcileAgent(ctx context.Context, mcpServer *mcpv1.MCPServer) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Generate Agent name (default to mcpserver name + "-agent" if not specified)
	agentName := mcpServer.Name + "-agent"
	agentNamespace := mcpServer.Namespace
	
	// Use custom name/namespace if specified in the AgentSpec
	// Note: We can't directly access name/namespace from AgentSpec since it's embedded in the MCPServer
	// The controller automatically manages the name and namespace

	toolServerName := mcpServer.Name + "-toolserver"
	
	// Create the desired Agent spec
	desired := &kagentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentName,
			Namespace: agentNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "agent",
				"app.kubernetes.io/instance":   agentName,
				"app.kubernetes.io/component":  "kagent-integration",
				"app.kubernetes.io/part-of":    "mcp-operator",
				"app.kubernetes.io/managed-by": "mcp-operator",
			"mcp.v2dy.github.io/mcpserver": mcpServer.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: mcpServer.APIVersion,
					Kind:       mcpServer.Kind,
					Name:       mcpServer.Name,
					UID:        mcpServer.UID,
					Controller: &[]bool{true}[0],
				},
			},
		},
		Spec: *mcpServer.Spec.Kagent.DeepCopy(), // Copy the AgentSpec from MCPServer
	}

	// Automatically set the tools to reference our auto-generated ToolServer
	// Override any existing tools configuration to ensure it points to our ToolServer
	desired.Spec.Tools = []*kagentv1alpha1.Tool{
		{
			Type: kagentv1alpha1.ToolProviderType_McpServer,
			McpServer: &kagentv1alpha1.McpServerTool{
				ToolServer: fmt.Sprintf("%s/%s", mcpServer.Namespace, toolServerName),
				ToolNames:  []string{}, // Empty means all tools
			},
		},
	}

	// Check if Agent already exists
	existing := &kagentv1alpha1.Agent{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      agentName,
		Namespace: agentNamespace,
	}, existing)

	if err != nil && apierrors.IsNotFound(err) {
		// Create new Agent
		log.Info("Creating Agent", "agent", agentName, "namespace", agentNamespace)
		if err := r.Create(ctx, desired); err != nil {
			log.Error(err, "Failed to create Agent", "agent", agentName)
			return ctrl.Result{}, err
		}
		log.Info("Successfully created Agent", "agent", agentName)
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Agent", "agent", agentName)
		return ctrl.Result{}, err
	}

	// Update existing Agent if needed
	// We always update the tools to ensure they point to our ToolServer
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	
	log.Info("Updating Agent", "agent", agentName)
	if err := r.Update(ctx, existing); err != nil {
		log.Error(err, "Failed to update Agent", "agent", agentName)
		return ctrl.Result{}, err
	}
	log.Info("Successfully updated Agent", "agent", agentName)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MCPServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize managers
	r.buildManager = NewBuildManager(r.Client, r.Scheme)
	r.resourceBuilder = NewResourceBuilder(r.Client, r.Scheme)
	r.statusManager = NewStatusManager(r.Client)

	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1.MCPServer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&batchv1.Job{}).
		Owns(&kagentv1alpha1.ToolServer{}).
		Owns(&kagentv1alpha1.Agent{}).
		Complete(r)
}
