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

	kagentv1alpha1 "github.com/kagent-dev/kagent/go/controller/api/v1alpha1"
)

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

type ResourceConfig struct {
	// +optional
	Requests corev1.ResourceList `json:"requests,omitempty"`
	// +optional
	Limits corev1.ResourceList `json:"limits,omitempty"`
}

type SecurityConfig struct {
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
}

type HealthConfig struct {
	// +optional
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`
	// +optional
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`
}

type RuntimeConfig struct {
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
}

type SchedulingConfig struct {
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
}

type AuthConfig struct {
	// +optional
	Basic *BasicAuth `json:"basic,omitempty"`
	// +optional
	Bearer *BearerAuth `json:"bearer,omitempty"`
	// +optional
	APIKey *APIKeyAuth `json:"apiKey,omitempty"`
	// +optional
	OAuth2 *OAuth2Auth `json:"oauth2,omitempty"`
}

// SecretRef is a simplified reference to a secret key
type SecretRef struct {
	// Name of the secret
	Name string `json:"name"`
	// Key within the secret
	Key string `json:"key"`
	// Optional flag to specify if the secret must exist
	// +optional
	Optional *bool `json:"optional,omitempty"`
}

// ValueOrSecret represents a value that can be either a direct string or a reference to a secret
type ValueOrSecret struct {
	// Value is the direct string value
	// +optional
	Value *string `json:"value,omitempty"`
	// SecretRef is a reference to a secret key
	// +optional
	SecretRef *SecretRef `json:"secretRef,omitempty"`
}

// BasicAuth defines basic authentication
type BasicAuth struct {
	// +kubebuilder:validation:Required
	Username string `json:"username"`
	// +kubebuilder:validation:Required
	Password *ValueOrSecret `json:"password,omitempty"`
}

// BearerAuth defines bearer token authentication
type BearerAuth struct {
	// +kubebuilder:validation:Required
	Token *ValueOrSecret `json:"token,omitempty"`
}

// APIKeyAuth defines API key authentication
type APIKeyAuth struct {
	// +kubebuilder:validation:Required
	Location string `json:"location"`
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// +kubebuilder:validation:Required
	Value *ValueOrSecret `json:"value,omitempty"`
}

// OAuth2Auth defines OAuth2 authentication
type OAuth2Auth struct {
	// +kubebuilder:validation:Required
	TokenURL string `json:"tokenUrl"`
	// +kubebuilder:validation:Required
	ClientID string `json:"clientId"`
	// +kubebuilder:validation:Required
	ClientSecret *ValueOrSecret `json:"clientSecret"`
	// +kubebuilder:validation:Required
	Scope string `json:"scope,omitempty"`
}

type MCPServerSpec struct {
	// +optional
	DeploymentName string `json:"deploymentName,omitempty"`
	// +optional
	// +kubebuilder:validation:Minimum=0
	Replicas int32  `json:"replicas,omitempty"`
	Url      string `json:"url,omitempty"`
	BasePath string `json:"basePath,omitempty"`
	// +optional
	Registry string `json:"registry,omitempty"`
	// +optional
	// +kubebuilder:default=false
	Legacy *bool `json:"legacy,omitempty"`
	// +optional
	Auth *AuthConfig `json:"auth,omitempty"`
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"` // backward compatibility
	// +optional
	PodTemplate *PodTemplateConfig `json:"podTemplate,omitempty"`
	// +optional
	Kagent *kagentv1alpha1.AgentSpec `json:"kagent,omitempty"`
}

type MCPServerStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
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
