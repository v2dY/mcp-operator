package controller

import (
	"context"
	"fmt"

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

// Builder interface for image building strategies
type Builder interface {
	GetImage(mcpServer *mcpv1.MCPServer) (string, error)
}

// PrebuiltBuilder strategy for using prebuilt images
type PrebuiltBuilder struct{}

// GetImage gets the image for prebuilt strategy
func (b *PrebuiltBuilder) GetImage(mcpServer *mcpv1.MCPServer) (string, error) {
	if mcpServer.Spec.Registry != "" {
		// Use the specified registry with the MCPServer name as tag
		return fmt.Sprintf("%s/mcp-server:%s", mcpServer.Spec.Registry, mcpServer.Name), nil
	}
	// Default prebuilt image when no registry is specified
	return "sarco3t/openapi-mcp-generator:1.0.6", nil
}

// BuildahBuilder strategy for building images with Buildah
type BuildahBuilder struct {
	buildManager *BuildManager
}

// GetImage builds the image with Buildah
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

// ResourceBuilder handles resource creation
type ResourceBuilder struct {
	client.Client
	scheme *runtime.Scheme
}

// NewResourceBuilder creates a new resource builder
func NewResourceBuilder(c client.Client, scheme *runtime.Scheme) *ResourceBuilder {
	return &ResourceBuilder{
		Client: c,
		scheme: scheme,
	}
}

// BuildDeployment builds a deployment for MCPServer
func (rb *ResourceBuilder) BuildDeployment(mcpServer *mcpv1.MCPServer) (*appsv1.Deployment, error) {
	labels := GetLabels(mcpServer)

	// Dynamically choose builder based on registry field
	var builder Builder
	if mcpServer.Spec.Registry == "" {
		// If no registry specified, use PrebuiltBuilder (default prebuilt image)
		builder = &PrebuiltBuilder{}
	} else {
		// If registry is specified, use BuildahBuilder to build the image and push to registry
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

// buildContainer builds container spec
func (rb *ResourceBuilder) buildContainer(mcpServer *mcpv1.MCPServer, image string) corev1.Container {
	container := corev1.Container{
		Name:            "mcp",
		Image:           image,
		ImagePullPolicy: corev1.PullAlways,
		SecurityContext: rb.getDefaultContainerSecurityContext(),
		Ports: []corev1.ContainerPort{{
			ContainerPort: 3001,
			Name:          "http",
		}},
		Env: mcpServer.Spec.Env,
	}

	// For prebuilt image, we need to pass OpenAPI URL and base path as arguments
	if mcpServer.Spec.Registry == "" {
		// Using prebuilt image sarco3t/openapi-mcp-generator:1.0.6
		args := []string{mcpServer.Spec.Url}
		if mcpServer.Spec.BasePath != "" {
			args = append(args, mcpServer.Spec.BasePath)
		}
		container.Args = args
	}

	rb.mergeContainerConfiguration(mcpServer, &container)
	return container
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
				Port:       3001,
				TargetPort: intstr.FromInt(3001),
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

// getDefaultPodSecurityContext returns default pod security context
func (rb *ResourceBuilder) getDefaultPodSecurityContext() *corev1.PodSecurityContext {
	return &corev1.PodSecurityContext{
		RunAsNonRoot: ptr.To(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// getDefaultContainerSecurityContext returns default container security context
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
