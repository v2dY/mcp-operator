package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mcpv1 "github.com/v2dY/project/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMCPServerReconciler_Reconcile_NotFound(t *testing.T) {
	// Create fake client and reconciler
	scheme := runtime.NewScheme()
	err := mcpv1.AddToScheme(scheme)
	require.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := &MCPServerReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Test with non-existent MCPServer
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.TODO(), req)

	// Should return without error (resource not found is handled gracefully)
	assert.NoError(t, err)
	assert.False(t, result.Requeue)
}
