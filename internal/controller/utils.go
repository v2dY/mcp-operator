package controller

import (
	"fmt"
	"reflect"

	mcpv1 "github.com/v2dY/project/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// GetLabels returns standard labels for MCPServer resources
func GetLabels(mcpServer *mcpv1.MCPServer) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       labelAppName,
		"app.kubernetes.io/instance":   mcpServer.Name,
		"app.kubernetes.io/managed-by": operatorName,
		"app.kubernetes.io/component":  labelComponent,
	}
}

// GetTemplateData creates template data from MCPServer
func GetTemplateData(mcpServer *mcpv1.MCPServer) TemplateData {
	if mcpServer == nil {
		return TemplateData{}
	}

	imageName := fmt.Sprintf("%s/%s", mcpServer.Spec.Registry, mcpServer.Name)
	if mcpServer.Spec.Registry == "" {
		imageName = mcpServer.Name
	}

	return TemplateData{
		MCPServerName: mcpServer.Name,
		Namespace:     mcpServer.Namespace,
		OpenAPIUrl:    mcpServer.Spec.Url,
		BasePath:      mcpServer.Spec.BasePath,
		Registry:      mcpServer.Spec.Registry,
		ImageName:     imageName,
	}
}

// DetectDeploymentChanges compares desired vs actual deployment and returns what changed
func DetectDeploymentChanges(desired, actual *appsv1.Deployment) *DeploymentChanges {
	changes := &DeploymentChanges{
		Changes: []string{},
	}

	// Check replicas
	if *desired.Spec.Replicas != *actual.Spec.Replicas {
		changes.Replicas = true
		changes.Changes = append(changes.Changes,
			fmt.Sprintf("replicas: %d -> %d", *actual.Spec.Replicas, *desired.Spec.Replicas))
	}

	// Check pod template changes
	if podTemplateChanged(&desired.Spec.Template, &actual.Spec.Template) {
		changes.PodTemplate = true
		// Get detailed pod template changes
		podChanges := detectPodTemplateChanges(&desired.Spec.Template, &actual.Spec.Template)
		changes.Changes = append(changes.Changes, podChanges...)
	}

	return changes
}

// podTemplateChanged checks if pod template specs are different
func podTemplateChanged(desired, actual *corev1.PodTemplateSpec) bool {
	// Compare containers (main focus)
	if len(desired.Spec.Containers) != len(actual.Spec.Containers) {
		return true
	}

	for i, desiredContainer := range desired.Spec.Containers {
		if i >= len(actual.Spec.Containers) {
			return true
		}
		actualContainer := actual.Spec.Containers[i]

		// Check key container fields that can be changed via podTemplate
		if desiredContainer.Image != actualContainer.Image ||
			!reflect.DeepEqual(desiredContainer.Resources, actualContainer.Resources) ||
			!reflect.DeepEqual(desiredContainer.Env, actualContainer.Env) ||
			!reflect.DeepEqual(desiredContainer.ReadinessProbe, actualContainer.ReadinessProbe) ||
			!reflect.DeepEqual(desiredContainer.LivenessProbe, actualContainer.LivenessProbe) ||
			desiredContainer.ImagePullPolicy != actualContainer.ImagePullPolicy ||
			!reflect.DeepEqual(desiredContainer.SecurityContext, actualContainer.SecurityContext) {
			return true
		}
	}

	// Check pod-level settings from podTemplate.scheduling
	if !reflect.DeepEqual(desired.Spec.SecurityContext, actual.Spec.SecurityContext) ||
		!reflect.DeepEqual(desired.Spec.NodeSelector, actual.Spec.NodeSelector) ||
		!reflect.DeepEqual(desired.Spec.Tolerations, actual.Spec.Tolerations) ||
		!reflect.DeepEqual(desired.Spec.Affinity, actual.Spec.Affinity) {
		return true
	}

	return false
}

// detectPodTemplateChanges returns human-readable list of pod template changes
func detectPodTemplateChanges(desired, actual *corev1.PodTemplateSpec) []string {
	changes := []string{}

	// Check containers
	for i, desiredContainer := range desired.Spec.Containers {
		if i >= len(actual.Spec.Containers) {
			changes = append(changes, fmt.Sprintf("container[%d]: added", i))
			continue
		}
		actualContainer := actual.Spec.Containers[i]

		if desiredContainer.Image != actualContainer.Image {
			changes = append(changes, fmt.Sprintf("container[%d].image: %s -> %s",
				i, actualContainer.Image, desiredContainer.Image))
		}

		if !reflect.DeepEqual(desiredContainer.Resources, actualContainer.Resources) {
			changes = append(changes, fmt.Sprintf("container[%d].resources: updated", i))
		}

		if !reflect.DeepEqual(desiredContainer.Env, actualContainer.Env) {
			changes = append(changes, fmt.Sprintf("container[%d].env: updated", i))
		}

		if !reflect.DeepEqual(desiredContainer.ReadinessProbe, actualContainer.ReadinessProbe) {
			changes = append(changes, fmt.Sprintf("container[%d].readinessProbe: updated", i))
		}

		if !reflect.DeepEqual(desiredContainer.LivenessProbe, actualContainer.LivenessProbe) {
			changes = append(changes, fmt.Sprintf("container[%d].livenessProbe: updated", i))
		}

		if desiredContainer.ImagePullPolicy != actualContainer.ImagePullPolicy {
			changes = append(changes, fmt.Sprintf("container[%d].imagePullPolicy: %s -> %s",
				i, actualContainer.ImagePullPolicy, desiredContainer.ImagePullPolicy))
		}

		if !reflect.DeepEqual(desiredContainer.SecurityContext, actualContainer.SecurityContext) {
			changes = append(changes, fmt.Sprintf("container[%d].securityContext: updated", i))
		}
	}

	// Check pod-level changes
	if !reflect.DeepEqual(desired.Spec.SecurityContext, actual.Spec.SecurityContext) {
		changes = append(changes, "pod.securityContext: updated")
	}

	if !reflect.DeepEqual(desired.Spec.NodeSelector, actual.Spec.NodeSelector) {
		changes = append(changes, "nodeSelector: updated")
	}

	if !reflect.DeepEqual(desired.Spec.Tolerations, actual.Spec.Tolerations) {
		changes = append(changes, "tolerations: updated")
	}

	if !reflect.DeepEqual(desired.Spec.Affinity, actual.Spec.Affinity) {
		changes = append(changes, "affinity: updated")
	}

	return changes
}
