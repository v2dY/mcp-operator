package controller

import (
	"context"
	"fmt"
	"strings"

	mcpv1 "github.com/v2dY/project/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	newGenImage   = "ghcr.io/v2dy/mcp-gen:0.3.0"
	PrebuiltImage = "sarco3t/openapi-mcp-generator:1.0.6"
	ContainerPort = 3001
)

type Builder interface {
	GetImage(mcpServer *mcpv1.MCPServer) (string, error)
}

type PrebuiltBuilder struct{}

func (b *PrebuiltBuilder) GetImage(mcpServer *mcpv1.MCPServer) (string, error) {
	if mcpServer.Spec.Registry != "" {
		return fmt.Sprintf("%s/mcp-server:%s", mcpServer.Spec.Registry, mcpServer.Name), nil
	}
	return PrebuiltImage, nil
}

type BuildahBuilder struct {
	buildManager *BuildManager
}

func (b *BuildahBuilder) GetImage(mcpServer *mcpv1.MCPServer) (string, error) {
	buildCompleted, requeueAfter, err := b.buildManager.ReconcileBuild(context.Background(), mcpServer)
	if err != nil {
		return "", fmt.Errorf("failed to build image: %w", err)
	}

	if !buildCompleted {
		return "", fmt.Errorf("build in progress, requeue after %v", requeueAfter)
	}

	return fmt.Sprintf("%s/mcp-server:%s", mcpServer.Spec.Registry, mcpServer.Name), nil
}

type newGenBuilder struct{}

func (k *newGenBuilder) GetImage(mcpServer *mcpv1.MCPServer) (string, error) {
	return newGenImage, nil
}

type ResourceBuilder struct {
	client.Client
	scheme *runtime.Scheme
}

func NewResourceBuilder(c client.Client, scheme *runtime.Scheme) *ResourceBuilder {
	return &ResourceBuilder{
		Client: c,
		scheme: scheme,
	}
}

func (rb *ResourceBuilder) BuildDeployment(mcpServer *mcpv1.MCPServer) (*appsv1.Deployment, error) {
	labels := GetLabels(mcpServer)

	var builder Builder
	isLegacy := mcpServer.Spec.Legacy == nil || *mcpServer.Spec.Legacy

	if mcpServer.Spec.Registry == "" {
		if !isLegacy {
			builder = &newGenBuilder{}
		} else {
			builder = &PrebuiltBuilder{}
		}
	} else {
		builder = &BuildahBuilder{buildManager: NewBuildManager(rb.Client, rb.scheme)}
	}

	image, err := builder.GetImage(mcpServer)
	if err != nil {
		return nil, fmt.Errorf("failed to get image: %w", err)
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &mcpServer.Spec.Replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					SecurityContext: rb.getDefaultPodSecurityContext(),
					Containers:      []corev1.Container{rb.buildContainer(mcpServer, image)},
				},
			},
		},
	}

	rb.mergePodTemplateConfiguration(mcpServer, dep)

	if err := ctrl.SetControllerReference(mcpServer, dep, rb.scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	return dep, nil
}

func (rb *ResourceBuilder) buildContainer(mcpServer *mcpv1.MCPServer, image string) corev1.Container {
	container := corev1.Container{
		Name:            "mcp",
		Image:           image,
		ImagePullPolicy: corev1.PullAlways,
		SecurityContext: rb.getDefaultContainerSecurityContext(),
		Ports: []corev1.ContainerPort{{
			ContainerPort: ContainerPort,
			Name:          "http",
		}},
		Env: mcpServer.Spec.Env,
	}

	isLegacy := mcpServer.Spec.Legacy == nil || *mcpServer.Spec.Legacy

	if !isLegacy {
		args := []string{"generate", "--path", mcpServer.Spec.Url}
		args = append(args, "--host", "0.0.0.0")
		args = append(args, "--port", fmt.Sprintf("%d", ContainerPort))
		if mcpServer.Spec.BasePath != "" {
			args = append(args, "--base-url", mcpServer.Spec.BasePath)
		}

		var needsShell bool
		if mcpServer.Spec.Auth != nil {
			authArgs, authEnv := rb.buildAuthArgsAndEnv(mcpServer.Spec.Auth)
			args = append(args, authArgs...)
			container.Env = append(container.Env, authEnv...)

			for _, arg := range authArgs {
				if len(arg) > 2 && arg[0] == '$' && arg[1] == '{' {
					needsShell = true
					break
				}
			}
		}

		if needsShell {
			fullCommand := "openapi-to-mcp " + strings.Join(args, " ")
			container.Command = []string{"/bin/sh", "-c"}
			container.Args = []string{fullCommand}
		} else {
			container.Args = args
		}
	} else if mcpServer.Spec.Registry == "" {
		args := []string{mcpServer.Spec.Url}
		if mcpServer.Spec.BasePath != "" {
			args = append(args, mcpServer.Spec.BasePath)
		}
		container.Args = args
	}

	rb.mergeContainerConfiguration(mcpServer, &container)
	return container
}

func (rb *ResourceBuilder) buildAuthArgsAndEnv(auth *mcpv1.AuthConfig) ([]string, []corev1.EnvVar) {
	var args []string
	var env []corev1.EnvVar

	if auth.Basic != nil {
		args = append(args, "--auth-type", "basic")
		args = append(args, "--basic-username", auth.Basic.Username)

		if auth.Basic.Password != nil {
			argValue, envVar := rb.resolveValueOrSecret(auth.Basic.Password, "BASIC_PASSWORD")
			if envVar != nil {
				env = append(env, *envVar)
			}
			args = append(args, "--basic-password", argValue)
		}
	}

	if auth.Bearer != nil && auth.Bearer.Token != nil {
		args = append(args, "--auth-type", "bearer")
		argValue, envVar := rb.resolveValueOrSecret(auth.Bearer.Token, "BEARER_TOKEN")
		if envVar != nil {
			env = append(env, *envVar)
		}
		args = append(args, "--bearer-token", argValue)
	}

	if auth.APIKey != nil {
		args = append(args, "--auth-type", "api_key")
		args = append(args, "--api-key-location", auth.APIKey.Location)
		args = append(args, "--api-key-name", auth.APIKey.Name)

		if auth.APIKey.Value != nil {
			argValue, envVar := rb.resolveValueOrSecret(auth.APIKey.Value, "API_KEY_VALUE")
			if envVar != nil {
				env = append(env, *envVar)
			}
			args = append(args, "--api-key-value", argValue)
		}
	}

	if auth.OAuth2 != nil {
		args = append(args, "--auth-type", "oauth2")
		args = append(args, "--oauth-token-url", auth.OAuth2.TokenURL)
		args = append(args, "--oauth-client-id", auth.OAuth2.ClientID)

		if auth.OAuth2.ClientSecret != nil {
			argValue, envVar := rb.resolveValueOrSecret(auth.OAuth2.ClientSecret, "OAUTH_CLIENT_SECRET")
			if envVar != nil {
				env = append(env, *envVar)
			}
			args = append(args, "--oauth-client-secret", argValue)
		}

		if auth.OAuth2.Scope != "" {
			args = append(args, "--oauth-scope", auth.OAuth2.Scope)
		}
	}

	if args == nil {
		args = []string{}
	}
	if env == nil {
		env = []corev1.EnvVar{}
	}

	return args, env
}

func (rb *ResourceBuilder) resolveValueOrSecret(valueOrSecret *mcpv1.ValueOrSecret, envVarName string) (string, *corev1.EnvVar) {
	if valueOrSecret == nil {
		return "", nil
	}

	if valueOrSecret.Value != nil {
		return *valueOrSecret.Value, nil
	}

	if valueOrSecret.SecretRef != nil {
		env := &corev1.EnvVar{
			Name: envVarName,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: valueOrSecret.SecretRef.Name,
					},
					Key:      valueOrSecret.SecretRef.Key,
					Optional: valueOrSecret.SecretRef.Optional,
				},
			},
		}
		return "${" + envVarName + "}", env
	}

	return "", nil
} // buildAuthArgs builds authentication arguments for new gen command (deprecated, use buildAuthArgsAndEnv)
func (rb *ResourceBuilder) buildAuthArgs(auth *mcpv1.AuthConfig) []string {
	args, _ := rb.buildAuthArgsAndEnv(auth)
	return args
}

// BuildService builds a service for MCPServer
func (rb *ResourceBuilder) BuildService(mcpServer *mcpv1.MCPServer) (*corev1.Service, error) {
	labels := GetLabels(mcpServer)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Port:       ContainerPort,
				TargetPort: intstr.FromInt(ContainerPort),
				Protocol:   corev1.ProtocolTCP,
				Name:       "http",
			}},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	if err := ctrl.SetControllerReference(mcpServer, svc, rb.scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	return svc, nil
}

func (rb *ResourceBuilder) getDefaultPodSecurityContext() *corev1.PodSecurityContext {
	return &corev1.PodSecurityContext{
		RunAsNonRoot: ptr.To(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

func (rb *ResourceBuilder) getDefaultContainerSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		RunAsNonRoot:             ptr.To(true),
		RunAsUser:                ptr.To(int64(1001)),
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}
}

// mergeContainerConfiguration merges container configuration from podTemplate
func (rb *ResourceBuilder) mergeContainerConfiguration(mcpServer *mcpv1.MCPServer, container *corev1.Container) {
	if mcpServer.Spec.PodTemplate == nil {
		return
	}

	podTemplate := mcpServer.Spec.PodTemplate

	// Merge resources
	if podTemplate.Resources != nil {
		container.Resources = corev1.ResourceRequirements{}
		if podTemplate.Resources.Requests != nil {
			container.Resources.Requests = podTemplate.Resources.Requests
		}
		if podTemplate.Resources.Limits != nil {
			container.Resources.Limits = podTemplate.Resources.Limits
		}
	}

	// Merge health probes
	if podTemplate.Health != nil {
		if podTemplate.Health.ReadinessProbe != nil {
			container.ReadinessProbe = podTemplate.Health.ReadinessProbe
		}
		if podTemplate.Health.LivenessProbe != nil {
			container.LivenessProbe = podTemplate.Health.LivenessProbe
		}
	}

	// Merge runtime settings
	if podTemplate.Runtime != nil {
		if podTemplate.Runtime.ImagePullPolicy != "" {
			container.ImagePullPolicy = podTemplate.Runtime.ImagePullPolicy
		}
		// Merge env vars (both old and new)
		if len(podTemplate.Runtime.Env) > 0 {
			// Start with old env vars for backward compatibility
			allEnvs := append([]corev1.EnvVar{}, mcpServer.Spec.Env...)
			// Add new env vars, with new ones taking precedence
			for _, newEnv := range podTemplate.Runtime.Env {
				found := false
				for i, existingEnv := range allEnvs {
					if existingEnv.Name == newEnv.Name {
						allEnvs[i] = newEnv // Override existing
						found = true
						break
					}
				}
				if !found {
					allEnvs = append(allEnvs, newEnv) // Add new
				}
			}
			container.Env = allEnvs
		}
	}

	// Merge security context
	if podTemplate.Security != nil && podTemplate.Security.SecurityContext != nil {
		// Merge with existing security context
		if container.SecurityContext == nil {
			container.SecurityContext = &corev1.SecurityContext{}
		}
		// Override only specified fields, keep defaults for others
		secCtx := podTemplate.Security.SecurityContext
		if secCtx.RunAsUser != nil {
			container.SecurityContext.RunAsUser = secCtx.RunAsUser
		}
		if secCtx.RunAsNonRoot != nil {
			container.SecurityContext.RunAsNonRoot = secCtx.RunAsNonRoot
		}
		if secCtx.AllowPrivilegeEscalation != nil {
			container.SecurityContext.AllowPrivilegeEscalation = secCtx.AllowPrivilegeEscalation
		}
		if secCtx.Capabilities != nil {
			container.SecurityContext.Capabilities = secCtx.Capabilities
		}
	}
}

// mergePodTemplateConfiguration merges pod-level configuration
func (rb *ResourceBuilder) mergePodTemplateConfiguration(mcpServer *mcpv1.MCPServer, dep *appsv1.Deployment) {
	if mcpServer.Spec.PodTemplate == nil || mcpServer.Spec.PodTemplate.Scheduling == nil {
		return
	}

	scheduling := mcpServer.Spec.PodTemplate.Scheduling

	// Merge nodeSelector
	if scheduling.NodeSelector != nil {
		dep.Spec.Template.Spec.NodeSelector = scheduling.NodeSelector
	}

	// Merge tolerations
	if len(scheduling.Tolerations) > 0 {
		dep.Spec.Template.Spec.Tolerations = scheduling.Tolerations
	}

	// Merge affinity
	if scheduling.Affinity != nil {
		dep.Spec.Template.Spec.Affinity = scheduling.Affinity
	}
}
