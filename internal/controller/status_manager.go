package controller

import (
	"context"
	"fmt"

	mcpv1 "github.com/v2dY/project/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// StatusManager handles status updates
type StatusManager struct {
	client.Client
}

// NewStatusManager creates a new status manager
func NewStatusManager(client client.Client) *StatusManager {
	return &StatusManager{
		Client: client,
	}
}

// HandleReconcileError handles structured reconciliation errors
func (sm *StatusManager) HandleReconcileError(ctx context.Context, mcpServer *mcpv1.MCPServer, recErr *ReconcileError) error {
	log := logf.FromContext(ctx)
	log.Error(recErr.Err, recErr.Message, "phase", recErr.Phase, "reason", recErr.Reason)

	meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{
		Type:    typeAvailableMCPServer,
		Status:  metav1.ConditionFalse,
		Reason:  recErr.Reason,
		Message: recErr.Message,
	})

	if err := sm.Status().Update(ctx, mcpServer); err != nil {
		return fmt.Errorf("failed to update status after error: %w", err)
	}

	return recErr
}

// InitializeStatus initializes MCPServer status
func (sm *StatusManager) InitializeStatus(ctx context.Context, mcpServer *mcpv1.MCPServer, req ctrl.Request) (ctrl.Result, error) {
	if len(mcpServer.Status.Conditions) == 0 {
		meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{
			Type: typeAvailableMCPServer, Status: metav1.ConditionUnknown,
			Reason: "Reconciling", Message: "Starting reconciliation"})

		if err := sm.Status().Update(ctx, mcpServer); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update initial status: %w", err)
		}

		// Re-fetch to get updated resource version
		if err := sm.Get(ctx, req.NamespacedName, mcpServer); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to re-fetch MCPServer: %w", err)
		}
	}
	return ctrl.Result{}, nil
}

// UpdateFinalStatus updates the final reconciliation status
func (sm *StatusManager) UpdateFinalStatus(ctx context.Context, mcpServer *mcpv1.MCPServer) (ctrl.Result, error) {
	// Check deployment readiness
	deployment := &appsv1.Deployment{}
	err := sm.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, deployment)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get deployment for status check: %w", err)
	}

	// Update status based on deployment readiness
	if deployment.Status.ReadyReplicas == deployment.Status.Replicas && deployment.Status.Replicas > 0 {
		meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{
			Type:    typeAvailableMCPServer,
			Status:  metav1.ConditionTrue,
			Reason:  "Ready",
			Message: fmt.Sprintf("Deployment ready with %d/%d replicas", deployment.Status.ReadyReplicas, deployment.Status.Replicas),
		})
	} else {
		meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{
			Type:    typeAvailableMCPServer,
			Status:  metav1.ConditionFalse,
			Reason:  "NotReady",
			Message: fmt.Sprintf("Deployment not ready: %d/%d replicas ready", deployment.Status.ReadyReplicas, deployment.Status.Replicas),
		})
	}

	// Update status
	if err := sm.Status().Update(ctx, mcpServer); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update final status: %w", err)
	}

	// Requeue to monitor health if not ready
	if deployment.Status.ReadyReplicas != deployment.Status.Replicas {
		return ctrl.Result{RequeueAfter: mediumRequeue}, nil
	}

	return ctrl.Result{}, nil
}
