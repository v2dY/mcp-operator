package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mcpv1 "github.com/v2dY/project/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuildManager_ReconcileBuild_NilMCPServer(t *testing.T) {
	// Create fake client and build manager
	scheme := runtime.NewScheme()
	err := mcpv1.AddToScheme(scheme)
	require.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	buildManager := NewBuildManager(fakeClient, scheme)

	// Test with nil MCPServer
	buildCompleted, requeue, err := buildManager.ReconcileBuild(context.TODO(), nil)

	// Should return error for nil MCPServer
	assert.False(t, buildCompleted)
	assert.Equal(t, int64(0), requeue.Nanoseconds())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mcpServer cannot be nil")
}

func TestBuildManager_buildBuildahJob(t *testing.T) {
	// Create fake client and build manager
	scheme := runtime.NewScheme()
	err := mcpv1.AddToScheme(scheme)
	require.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	buildManager := NewBuildManager(fakeClient, scheme)

	// Create test MCPServer
	mcpServer := &mcpv1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server",
			Namespace: "test-namespace",
		},
		Spec: mcpv1.MCPServerSpec{
			Url:      "https://example.com/api.yml",
			BasePath: "https://api.example.com",
			Registry: "my-registry.com",
		},
	}

	// Test building job
	job, err := buildManager.buildBuildahJob(mcpServer)

	// Should create job successfully
	assert.NoError(t, err)
	assert.NotNil(t, job)
	assert.Equal(t, "build-test-server", job.Name)
	assert.Equal(t, "test-namespace", job.Namespace)
	assert.Contains(t, job.Labels, "job-type")
	assert.Equal(t, "buildah", job.Labels["job-type"])
}
