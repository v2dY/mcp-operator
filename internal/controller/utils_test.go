package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	mcpv1 "github.com/v2dY/project/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetTemplateData(t *testing.T) {
	tests := []struct {
		name      string
		mcpServer *mcpv1.MCPServer
		expected  TemplateData
	}{
		{
			name: "basic MCPServer without registry",
			mcpServer: &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "default",
				},
				Spec: mcpv1.MCPServerSpec{
					Url:      "https://example.com/openapi.yml",
					BasePath: "https://api.example.com",
				},
			},
			expected: TemplateData{
				MCPServerName: "test-server",
				Namespace:     "default",
				OpenAPIUrl:    "https://example.com/openapi.yml",
				BasePath:      "https://api.example.com",
				Registry:      "",
				ImageName:     "test-server",
			},
		},
		{
			name: "MCPServer with registry",
			mcpServer: &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-server",
					Namespace: "production",
				},
				Spec: mcpv1.MCPServerSpec{
					Url:      "https://production.example.com/api.yml",
					BasePath: "https://api.production.example.com",
					Registry: "my-registry.com",
				},
			},
			expected: TemplateData{
				MCPServerName: "custom-server",
				Namespace:     "production",
				OpenAPIUrl:    "https://production.example.com/api.yml",
				BasePath:      "https://api.production.example.com",
				Registry:      "my-registry.com",
				ImageName:     "my-registry.com/custom-server",
			},
		},
		{
			name:      "nil MCPServer",
			mcpServer: nil,
			expected: TemplateData{
				MCPServerName: "",
				Namespace:     "",
				OpenAPIUrl:    "",
				BasePath:      "",
				Registry:      "",
				ImageName:     "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetTemplateData(tt.mcpServer)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetLabels(t *testing.T) {
	mcpServer := &mcpv1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server",
			Namespace: "test-namespace",
		},
	}

	labels := GetLabels(mcpServer)

	expected := map[string]string{
		"app.kubernetes.io/name":       labelAppName,
		"app.kubernetes.io/instance":   "test-server",
		"app.kubernetes.io/managed-by": operatorName,
		"app.kubernetes.io/component":  labelComponent,
	}

	assert.Equal(t, expected, labels)
}
