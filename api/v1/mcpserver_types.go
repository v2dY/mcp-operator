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

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// PodTemplateConfig defines pod-level configuration options
type PodTemplateConfig struct {
    // +optional
    Resources *ResourceConfig `json:"resources,omitempty"`
    // +optional
    Security *SecurityConfig `json:"security,omitempty"`
    // +optional
    Health *HealthConfig `json:"health,omitempty"`
    // +optional
    Runtime *RuntimeConfig `json:"runtime,omitempty"`
    // +optional
    Scheduling *SchedulingConfig `json:"scheduling,omitempty"`
}

// ResourceConfig defines resource requests and limits
type ResourceConfig struct {
    // +optional
    Requests corev1.ResourceList `json:"requests,omitempty"`
    // +optional
    Limits corev1.ResourceList `json:"limits,omitempty"`
}

// SecurityConfig defines security context settings
type SecurityConfig struct {
    // +optional
    SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
}

// HealthConfig defines health check probes
type HealthConfig struct {
    // +optional
    ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`
    // +optional
    LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`
}

// RuntimeConfig defines runtime settings
type RuntimeConfig struct {
    // +optional
    Env []corev1.EnvVar `json:"env,omitempty"`
    // +optional
    ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
}

// SchedulingConfig defines pod scheduling settings
type SchedulingConfig struct {
    // +optional
    NodeSelector map[string]string `json:"nodeSelector,omitempty"`
    // +optional
    Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
    // +optional
    Affinity *corev1.Affinity `json:"affinity,omitempty"`
}
// MCPServerSpec defines the desired state of MCPServer.8443
type MCPServerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +optional
	DeploymentName string `json:"deploymentName,omitempty"`
	// +optional
	// +kubebuilder:validation:Minimum=0
	Replicas int32  `json:"replicas,omitempty"`
	Url      string `json:"url,omitempty"`
	BasePath string `json:"basePath,omitempty"`
	// +optional
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"` // backward compatibility
	// +optional
	// Containers []corev1.Container `json:"containers,omitempty"`
	PodTemplate *PodTemplateConfig `json:"podTemplate,omitempty"`
}

// MCPServerStatus defines the observed state of MCPServer.
type MCPServerStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MCPServer is the Schema for the mcpservers API.
type MCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MCPServerSpec   `json:"spec,omitempty"`
	Status MCPServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MCPServerList contains a list of MCPServer.
type MCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MCPServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MCPServer{}, &MCPServerList{})
}
