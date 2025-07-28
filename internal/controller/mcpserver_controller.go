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

package controller

import (
	"context"
	"fmt"
	"time"
	mcpv1 "github.com/v2dY/project/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// MCPServerReconciler reconciles a MCPServer object
type MCPServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	// typeAvailableMCPServer represents the status of the Deployment reconciliation
	typeAvailableMCPServer = "Available"
	labelAppName           = "project"
)

// +kubebuilder:rbac:groups=mcp.my.domain,resources=mcpservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mcp.my.domain,resources=mcpservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mcp.my.domain,resources=mcpservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MCPServer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *MCPServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the MCPServer instance
	// The purpose is check if the Custom Resource for the Kind MCPServer
	// is applied on the cluster if not we return nil to stop the reconciliation
	mcpServer := &mcpv1.MCPServer{}
	err := r.Get(ctx, req.NamespacedName, mcpServer)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			log.Info("mcp server resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get mcp server")
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status is available
	if len(mcpServer.Status.Conditions) == 0 {
		meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{Type: typeAvailableMCPServer, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, mcpServer); err != nil {
			log.Error(err, "Failed to update Server status")
			return ctrl.Result{}, err
		}

		// Let's re-fetch the mcp server Custom Resource after updating the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raising the error "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		if err := r.Get(ctx, req.NamespacedName, mcpServer); err != nil {
			log.Error(err, "Failed to re-fetch mcp server")
			return ctrl.Result{}, err
		}
	}
	// Check if Buildah job already exists, if not create one
	buildahJob := &batchv1.Job{}
	buildahJobName := fmt.Sprintf("buildah-%s", mcpServer.Name)
	err = r.Get(ctx, types.NamespacedName{Name: buildahJobName, Namespace: mcpServer.Namespace}, buildahJob)
	if err != nil && apierrors.IsNotFound(err) {
		// Create Buildah job to pre-build image
		job, err := r.buildahJobForMCPServer(mcpServer)
		if err != nil {
			log.Error(err, "Failed to define new Buildah Job for MCPServer")
			meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{
				Type: typeAvailableMCPServer, Status: metav1.ConditionFalse, Reason: "BuildFailed",
				Message: fmt.Sprintf("Failed to create Buildah Job: %s", err)})
			if err := r.Status().Update(ctx, mcpServer); err != nil {
				log.Error(err, "Failed to update Server status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}

		log.Info("Creating Buildah Job", "Job.Namespace", job.Namespace, "Job.Name", job.Name)
		if err = r.Create(ctx, job); err != nil {
			log.Error(err, "Failed to create Buildah Job")
			return ctrl.Result{}, err
		}

		// Job created, requeue to check its status
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Buildah Job")
		return ctrl.Result{}, err
	}

	// Check if Buildah job is completed successfully
	if buildahJob.Status.Succeeded == 0 {
		// Job is still running or failed, wait
		if buildahJob.Status.Failed > 0 {
			log.Error(fmt.Errorf("buildah job failed"), "Buildah Job failed")
			meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{
				Type: typeAvailableMCPServer, Status: metav1.ConditionFalse, Reason: "BuildFailed",
				Message: "Buildah Job failed to build image"})
			if err := r.Status().Update(ctx, mcpServer); err != nil {
				log.Error(err, "Failed to update Server status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, fmt.Errorf("buildah job failed")
		}
		// Job still running, requeue
		log.Info("Buildah Job still running, waiting...")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	log.Info("Buildah Job completed successfully, proceeding with Deployment")


	// Check if the deployment already exists, if not create a new one
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new deployment
		dep, err := r.deploymentForMCPServer(mcpServer)
		if err != nil {
			log.Error(err, "Failed to define new Deployment resource for MCPServer")

			// The following implementation will update the status
			meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{Type: typeAvailableMCPServer,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create Deployment for the custom resource (%s): (%s)", mcpServer.Name, err)})

			if err := r.Status().Update(ctx, mcpServer); err != nil {
				log.Error(err, "Failed to update Server status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		log.Info("Creating a new Deployment",
			"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		if err = r.Create(ctx, dep); err != nil {
			log.Error(err, "Failed to create new Deployment",
				"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}

		// Deployment created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}

	// The CRD API defines that the MCPServer type have a MCPServerSpec.Replicas field
	// to set the quantity of Deployment instances to the desired state on the cluster.
	// Therefore, the following code will ensure the Deployment size is the same as defined
	// via the Size spec of the Custom Resource which we are reconciling.
	size := mcpServer.Spec.Replicas
	if *found.Spec.Replicas != size {
		found.Spec.Replicas = &size
		if err = r.Update(ctx, found); err != nil {
			log.Error(err, "Failed to update Deployment",
				"Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)

			// Re-fetch the mcp server Custom Resource before updating the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raising the error "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, mcpServer); err != nil {
				log.Error(err, "Failed to re-fetch mcp server")
				return ctrl.Result{}, err
			}

			// The following implementation will update the status
			meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{Type: typeAvailableMCPServer,
				Status: metav1.ConditionFalse, Reason: "Resizing",
				Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", mcpServer.Name, err)})

			if err := r.Status().Update(ctx, mcpServer); err != nil {
				log.Error(err, "Failed to update Server status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		// Now, that we update the size we want to requeue the reconciliation
		// so that we can ensure that we have the latest state of the resource before
		// update. Also, it will help ensure the desired state on the cluster
		return ctrl.Result{Requeue: true}, nil
	}

	// The following implementation will update the status
	meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{Type: typeAvailableMCPServer,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", mcpServer.Name, size)})

	if err := r.Status().Update(ctx, mcpServer); err != nil {
		log.Error(err, "Failed to update Server status")
		return ctrl.Result{}, err
	}

	// Check if the service already exists, if not create a new one
	service := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, service)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new service
		svc, err := r.serviceForMCPServer(mcpServer)
		if err != nil {
			log.Error(err, "Failed to define new Service resource for MCPServer")
			return ctrl.Result{}, err
		}

		log.Info("Creating a new Service",
			"Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
		if err = r.Create(ctx, svc); err != nil {
			log.Error(err, "Failed to create new Service",
				"Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
			return ctrl.Result{}, err
		}
		// Service created successfully, requeue to ensure state
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MCPServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1.MCPServer{}).
		Owns(&appsv1.Deployment{}).
		Named("mcp-server").
		Complete(r)
}

// deploymentForMCPServer returns a MCPServer Deployment object
func (r *MCPServerReconciler) deploymentForMCPServer(
	mcpServer *mcpv1.MCPServer) (*appsv1.Deployment, error) {
	replicas := mcpServer.Spec.Replicas
	// Use pre-built image created by Buildah job
	image := fmt.Sprintf("docker-registry.registry.svc.cluster.local:5000/mcp-server:%s", mcpServer.Name)

	// Set default ImagePullPolicy to Always if not specified
	imagePullPolicy := corev1.PullAlways

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": labelAppName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": labelAppName},
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{{
						Image:           image,
						Name:            "mcp",
						ImagePullPolicy: imagePullPolicy,
						// Ensure restrictive context for the container
						// More info: https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot:             ptr.To(true),
							RunAsUser:                ptr.To(int64(1001)),
							AllowPrivilegeEscalation: ptr.To(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{
									"ALL",
								},
							},
						},
						Ports: []corev1.ContainerPort{{
							ContainerPort: 3001,
						}},
						Env:  mcpServer.Spec.Env,
						Args: []string{mcpServer.Spec.Url, mcpServer.Spec.BasePath},
					}},
				},
			},
		},
	}

	// Set the ownerRef for the Deployment
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
	if err := ctrl.SetControllerReference(mcpServer, dep, r.Scheme); err != nil {
		return nil, err
	}
	return dep, nil
}

// TODO: Potential improvements for Service spec:
// 1. Add ServiceSpec to MCPServer CRD to allow users to configure:
//   - Service type (ClusterIP, NodePort, LoadBalancer)
//   - Port configurations (name, port, targetPort, protocol)
//   - External IPs
//   - Session affinity settings
//   - Load balancer source ranges
//
// 2. Add validation for Service spec in the controller
// 3. Add support for multiple ports if needed
// 4. Add annotations and labels configuration
// 5. Add support for headless services
// 6. Add support for service mesh integration (e.g., Istio)
// serviceForMCPServer returns a Service for the MCPServer Deployment
func (r *MCPServerReconciler) serviceForMCPServer(mcpServer *mcpv1.MCPServer) (*corev1.Service, error) {
	labels := map[string]string{"app.kubernetes.io/name": labelAppName}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Port:       3001,
				TargetPort: intstrFromInt(3001),
				Protocol:   corev1.ProtocolTCP,
			}},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
	if err := ctrl.SetControllerReference(mcpServer, svc, r.Scheme); err != nil {
		return nil, err
	}
	return svc, nil
}

// intstrFromInt is a helper for TargetPort
func intstrFromInt(i int) intstr.IntOrString {
	return intstr.IntOrString{Type: intstr.Int, IntVal: int32(i)}
}

// buildahJobForMCPServer creates a Buildah Job to build pre-configured MCP server image
func (r *MCPServerReconciler) buildahJobForMCPServer(mcpServer *mcpv1.MCPServer) (*batchv1.Job, error) {
	// Generate unique image name for this MCP server
	imageName := fmt.Sprintf("docker-registry.registry.svc.cluster.local:5000/mcp-server:%s", mcpServer.Name)
	
	// Buildah script to build image with OpenAPI spec pre-loaded
	buildScript := fmt.Sprintf(`#!/bin/bash
set -e

echo "Starting Buildah job for MCPServer: %s"

# Create working container from base image
buildah from --name working-container registry.fedoraproject.org/fedora:latest

# Install dependencies
buildah run working-container -- dnf install -y python3 python3-pip curl

# Download OpenAPI spec from the URL
echo "Downloading OpenAPI spec from: %s"
buildah run working-container -- curl -o /tmp/openapi.json "%s"

# Copy our MCP generator code (this would be baked into base image in production)
# For now, we'll simulate with a simple echo
buildah run working-container -- mkdir -p /app
buildah run working-container -- echo "MCP Server ready with pre-loaded spec" > /app/ready.txt

# Commit the image
echo "Committing image: %s"
buildah commit working-container %s

# Push to registry (for now using insecure local registry)
echo "Pushing image to registry..."
buildah push --tls-verify=false %s

echo "Buildah job completed successfully!"
`, mcpServer.Name, mcpServer.Spec.Url, mcpServer.Spec.Url, imageName, imageName, imageName)

job := &batchv1.Job{
	ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("buildah-%s", mcpServer.Name),
		Namespace: mcpServer.Namespace,
		Labels:    map[string]string{"app": "buildah-mcp", "mcp-server": mcpServer.Name},
	},
	Spec: batchv1.JobSpec{
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				Containers: []corev1.Container{{
					Name:    "buildah",
					Image:   "quay.io/buildah/stable:latest",
					Command: []string{"/bin/bash"},
					Args:    []string{"-c", buildScript},
					SecurityContext: &corev1.SecurityContext{
						Privileged: ptr.To(true),
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("2Gi"),
							corev1.ResourceCPU:    resource.MustParse("1000m"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("1Gi"),
							corev1.ResourceCPU:    resource.MustParse("500m"),
						},
					},
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "container-storage",
						MountPath: "/var/lib/containers",
					}},
				}},
				Volumes: []corev1.Volume{{
					Name: "container-storage",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
						},
					},
				}},
			},
		},
	},
}

	// Set owner reference
	if err := ctrl.SetControllerReference(mcpServer, job, r.Scheme); err != nil {
		return nil, err
	}

	return job, nil
}
