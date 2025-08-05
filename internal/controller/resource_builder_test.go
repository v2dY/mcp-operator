package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	mcpv1 "github.com/v2dY/project/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPrebuiltBuilder_GetImage(t *testing.T) {
	builder := &PrebuiltBuilder{}

	tests := []struct {
		name     string
		server   *mcpv1.MCPServer
		expected string
	}{
		{
			name: "default prebuilt image",
			server: &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-server",
				},
				Spec: mcpv1.MCPServerSpec{
					Registry: "",
				},
			},
			expected: "sarco3t/openapi-mcp-generator:1.0.6",
		},
		{
			name: "custom registry image",
			server: &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-server",
				},
				Spec: mcpv1.MCPServerSpec{
					Registry: "my-registry.com",
				},
			},
			expected: "my-registry.com/mcp-server:test-server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			image, err := builder.GetImage(tt.server)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, image)
		})
	}
}
