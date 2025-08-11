package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	mcpv1 "github.com/v2dY/project/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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
			expected: PrebuiltImage,
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

func TestnewGenBuilder_GetImage(t *testing.T) {
	builder := &newGenBuilder{}

	server := &mcpv1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-server",
		},
		Spec: mcpv1.MCPServerSpec{
			Legacy: ptr.To(false),
		},
	}

	image, err := builder.GetImage(server)
	assert.NoError(t, err)
	assert.Equal(t, newGenImage, image)
}

func TestResourceBuilder_buildAuthArgs(t *testing.T) {
	rb := &ResourceBuilder{}

	tests := []struct {
		name     string
		auth     *mcpv1.AuthConfig
		expected []string
	}{
		{
			name: "basic auth with direct value",
			auth: &mcpv1.AuthConfig{
				Basic: &mcpv1.BasicAuth{
					Username: "user",
					Password: &mcpv1.ValueOrSecret{
						Value: ptr.To("pass"),
					},
				},
			},
			expected: []string{"--auth-type", "basic", "--basic-username", "user", "--basic-password", "pass"},
		},
		{
			name: "bearer auth with direct value",
			auth: &mcpv1.AuthConfig{
				Bearer: &mcpv1.BearerAuth{
					Token: &mcpv1.ValueOrSecret{
						Value: ptr.To("token123"),
					},
				},
			},
			expected: []string{"--auth-type", "bearer", "--bearer-token", "token123"},
		},
		{
			name: "api key auth with direct value",
			auth: &mcpv1.AuthConfig{
				APIKey: &mcpv1.APIKeyAuth{
					Location: "header",
					Name:     "X-API-Key",
					Value: &mcpv1.ValueOrSecret{
						Value: ptr.To("key123"),
					},
				},
			},
			expected: []string{"--auth-type", "api_key", "--api-key-location", "header", "--api-key-name", "X-API-Key", "--api-key-value", "key123"},
		},
		{
			name: "oauth2 auth with direct value",
			auth: &mcpv1.AuthConfig{
				OAuth2: &mcpv1.OAuth2Auth{
					TokenURL: "https://auth.example.com/token",
					ClientID: "client123",
					ClientSecret: &mcpv1.ValueOrSecret{
						Value: ptr.To("secret123"),
					},
					Scope: "read write",
				},
			},
			expected: []string{"--auth-type", "oauth2", "--oauth-token-url", "https://auth.example.com/token", "--oauth-client-id", "client123", "--oauth-client-secret", "secret123", "--oauth-scope", "read write"},
		},
		{
			name: "oauth2 auth without scope",
			auth: &mcpv1.AuthConfig{
				OAuth2: &mcpv1.OAuth2Auth{
					TokenURL: "https://auth.example.com/token",
					ClientID: "client123",
					ClientSecret: &mcpv1.ValueOrSecret{
						Value: ptr.To("secret123"),
					},
				},
			},
			expected: []string{"--auth-type", "oauth2", "--oauth-token-url", "https://auth.example.com/token", "--oauth-client-id", "client123", "--oauth-client-secret", "secret123"},
		},
		{
			name: "basic auth with secret reference",
			auth: &mcpv1.AuthConfig{
				Basic: &mcpv1.BasicAuth{
					Username: "user",
					Password: &mcpv1.ValueOrSecret{
						SecretRef: &mcpv1.SecretRef{
							Name: "auth-secret",
							Key:  "password",
						},
					},
				},
			},
			expected: []string{"--auth-type", "basic", "--basic-username", "user", "--basic-password", "${BASIC_PASSWORD}"},
		},
		{
			name:     "no auth",
			auth:     &mcpv1.AuthConfig{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := rb.buildAuthArgs(tt.auth)
			assert.Equal(t, tt.expected, args)
		})
	}
}

func TestResourceBuilder_buildContainer(t *testing.T) {
	rb := &ResourceBuilder{}

	tests := []struct {
		name               string
		server             *mcpv1.MCPServer
		image              string
		expectedCommand    []string
		expectedArgsPrefix []string
	}{
		{
			name: "legacy mode with prebuilt image",
			server: &mcpv1.MCPServer{
				Spec: mcpv1.MCPServerSpec{
					Legacy:   ptr.To(true),
					Url:      "https://api.example.com/openapi.json",
					BasePath: "/api/v1",
					Registry: "",
				},
			},
			image:              PrebuiltImage,
			expectedCommand:    nil,
			expectedArgsPrefix: []string{"https://api.example.com/openapi.json", "/api/v1"},
		},
		{
			name: "non-legacy mode with new image image",
			server: &mcpv1.MCPServer{
				Spec: mcpv1.MCPServerSpec{
					Legacy:   ptr.To(false),
					Url:      "https://api.example.com/openapi.json",
					BasePath: "/api/v1",
				},
			},
			image:              newGenImage,
			expectedCommand:    nil,
			expectedArgsPrefix: []string{"generate", "--path", "https://api.example.com/openapi.json", "--host", "0.0.0.0", "--port", "3001", "--base-url", "/api/v1"},
		},
		{
			name: "non-legacy mode with auth",
			server: &mcpv1.MCPServer{
				Spec: mcpv1.MCPServerSpec{
					Legacy: ptr.To(false),
					Url:    "https://api.example.com/openapi.json",
					Auth: &mcpv1.AuthConfig{
						Basic: &mcpv1.BasicAuth{
							Username: "user",
							Password: &mcpv1.ValueOrSecret{
								Value: ptr.To("pass"),
							},
						},
					},
				},
			},
			image:              newGenImage,
			expectedCommand:    nil,
			expectedArgsPrefix: []string{"generate", "--path", "https://api.example.com/openapi.json", "--host", "0.0.0.0", "--port", "3001", "--auth-type", "basic", "--basic-username", "user", "--basic-password", "pass"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := rb.buildContainer(tt.server, tt.image)

			assert.Equal(t, "mcp", container.Name)
			assert.Equal(t, tt.image, container.Image)
			assert.Equal(t, tt.expectedCommand, container.Command)

			if len(tt.expectedArgsPrefix) > 0 {
				assert.True(t, len(container.Args) >= len(tt.expectedArgsPrefix),
					"Expected at least %d args, got %d", len(tt.expectedArgsPrefix), len(container.Args))
				for i, expectedArg := range tt.expectedArgsPrefix {
					assert.Equal(t, expectedArg, container.Args[i],
						"Expected arg[%d] to be %s, got %s", i, expectedArg, container.Args[i])
				}
			}
		})
	}
}

func TestResourceBuilder_buildAuthArgsAndEnv(t *testing.T) {
	rb := &ResourceBuilder{}

	tests := []struct {
		name         string
		auth         *mcpv1.AuthConfig
		expectedArgs []string
		expectedEnv  []corev1.EnvVar
	}{
		{
			name: "basic auth with secret reference",
			auth: &mcpv1.AuthConfig{
				Basic: &mcpv1.BasicAuth{
					Username: "user",
					Password: &mcpv1.ValueOrSecret{
						SecretRef: &mcpv1.SecretRef{
							Name: "auth-secret",
							Key:  "password",
						},
					},
				},
			},
			expectedArgs: []string{"--auth-type", "basic", "--basic-username", "user", "--basic-password", "${BASIC_PASSWORD}"},
			expectedEnv: []corev1.EnvVar{
				{
					Name: "BASIC_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "auth-secret",
							},
							Key: "password",
						},
					},
				},
			},
		},
		{
			name: "bearer auth with secret reference",
			auth: &mcpv1.AuthConfig{
				Bearer: &mcpv1.BearerAuth{
					Token: &mcpv1.ValueOrSecret{
						SecretRef: &mcpv1.SecretRef{
							Name: "token-secret",
							Key:  "token",
						},
					},
				},
			},
			expectedArgs: []string{"--auth-type", "bearer", "--bearer-token", "${BEARER_TOKEN}"},
			expectedEnv: []corev1.EnvVar{
				{
					Name: "BEARER_TOKEN",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "token-secret",
							},
							Key: "token",
						},
					},
				},
			},
		},
		{
			name: "basic auth with direct value",
			auth: &mcpv1.AuthConfig{
				Basic: &mcpv1.BasicAuth{
					Username: "user",
					Password: &mcpv1.ValueOrSecret{
						Value: ptr.To("direct-password"),
					},
				},
			},
			expectedArgs: []string{"--auth-type", "basic", "--basic-username", "user", "--basic-password", "direct-password"},
			expectedEnv:  []corev1.EnvVar{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, env := rb.buildAuthArgsAndEnv(tt.auth)
			assert.Equal(t, tt.expectedArgs, args)
			assert.Equal(t, tt.expectedEnv, env)
		})
	}
}
