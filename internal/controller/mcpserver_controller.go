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
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"text/template"
	"time"

	mcpv1 "github.com/v2dY/project/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Embed templates
//
//go:embed templates/buildah-script.sh
var buildahScriptTemplate string

//go:embed templates/package.json
var packageJsonTemplate string

//go:embed templates/entrypoint.sh
var entrypointTemplate string

// Template data structure
type TemplateData struct {
	MCPServerName string
	OpenAPIUrl    string
	BasePath      string
	ImageName     string
}

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
func (r *MCPServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the MCPServer instance
	mcpServer := &mcpv1.MCPServer{}
	err := r.Get(ctx, req.NamespacedName, mcpServer)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("mcp server resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get mcp server")
		return ctrl.Result{}, err
	}

	// Set initial status if not available
	if len(mcpServer.Status.Conditions) == 0 {
		meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{
			Type: typeAvailableMCPServer, Status: metav1.ConditionUnknown,
			Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, mcpServer); err != nil {
			log.Error(err, "Failed to update Server status")
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, mcpServer); err != nil {
			log.Error(err, "Failed to re-fetch mcp server")
			return ctrl.Result{}, err
		}
	}

	// Handle build phase
	buildCompleted, requeueAfter, err := r.reconcileBuild(ctx, mcpServer)
	if err != nil {
		log.Error(err, "Build phase failed")
		meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{
			Type: typeAvailableMCPServer, Status: metav1.ConditionFalse, Reason: "BuildFailed",
			Message: fmt.Sprintf("Build failed: %s", err)})
		if statusErr := r.Status().Update(ctx, mcpServer); statusErr != nil {
			log.Error(statusErr, "Failed to update Server status")
		}
		return ctrl.Result{}, err
	}

	if !buildCompleted {
		// Build still in progress, requeue
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	log.Info("Buildah Job completed successfully, proceeding with Deployment")

	// Check if the deployment already exists, if not create a new one
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		dep, err := r.deploymentForMCPServer(mcpServer)
		if err != nil {
			log.Error(err, "Failed to define new Deployment resource for MCPServer")
			meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{
				Type: typeAvailableMCPServer, Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create Deployment for the custom resource (%s): (%s)", mcpServer.Name, err)})
			if err := r.Status().Update(ctx, mcpServer); err != nil {
				log.Error(err, "Failed to update Server status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}

		log.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		if err = r.Create(ctx, dep); err != nil {
			log.Error(err, "Failed to create new Deployment")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	// Update deployment replicas if needed
	size := mcpServer.Spec.Replicas
	if *found.Spec.Replicas != size {
		found.Spec.Replicas = &size
		if err = r.Update(ctx, found); err != nil {
			log.Error(err, "Failed to update Deployment",
				"Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)

			// Re-fetch the mcp server Custom Resource before updating the status
			if err := r.Get(ctx, req.NamespacedName, mcpServer); err != nil {
				log.Error(err, "Failed to re-fetch mcp server")
				return ctrl.Result{}, err
			}

			// Update status
			meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{
				Type: typeAvailableMCPServer, Status: metav1.ConditionFalse, Reason: "Resizing",
				Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", mcpServer.Name, err)})

			if err := r.Status().Update(ctx, mcpServer); err != nil {
				log.Error(err, "Failed to update Server status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Update status to successful
	meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{
		Type: typeAvailableMCPServer, Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", mcpServer.Name, size)})
	if err := r.Status().Update(ctx, mcpServer); err != nil {
		log.Error(err, "Failed to update Server status")
		return ctrl.Result{}, err
	}

	// Check if the service already exists, if not create a new one
	service := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, service)
	if err != nil && apierrors.IsNotFound(err) {
		svc, err := r.serviceForMCPServer(mcpServer)
		if err != nil {
			log.Error(err, "Failed to define new Service resource for MCPServer")
			return ctrl.Result{}, err
		}

		log.Info("Creating a new Service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
		if err = r.Create(ctx, svc); err != nil {
			log.Error(err, "Failed to create new Service")
			return ctrl.Result{}, err
		}
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
		Complete(r)
}

// renderTemplate renders a template with the given data
func (r *MCPServerReconciler) renderTemplate(templateStr string, data TemplateData) (string, error) {
	tmpl, err := template.New("template").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// getTemplateData creates template data from MCPServer
func (r *MCPServerReconciler) getTemplateData(mcpServer *mcpv1.MCPServer) TemplateData {
	return TemplateData{
		MCPServerName: mcpServer.Name,
		OpenAPIUrl:    mcpServer.Spec.Url,
		BasePath:      mcpServer.Spec.BasePath,
		ImageName:     fmt.Sprintf("docker-registry.registry.svc.cluster.local:5000/mcp-server:%s", mcpServer.Name),
	}
}

// deploymentForMCPServer returns a MCPServer Deployment object
func (r *MCPServerReconciler) deploymentForMCPServer(mcpServer *mcpv1.MCPServer) (*appsv1.Deployment, error) {
	replicas := mcpServer.Spec.Replicas
	image := fmt.Sprintf("docker-registry.registry.svc.cluster.local:5000/mcp-server:%s", mcpServer.Name)

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
						ImagePullPolicy: corev1.PullAlways,
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot:             ptr.To(true),
							RunAsUser:                ptr.To(int64(1001)),
							AllowPrivilegeEscalation: ptr.To(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
						Ports: []corev1.ContainerPort{{
							ContainerPort: 3001,
						}},
						Env: mcpServer.Spec.Env,
						// Remove Args since entrypoint will handle everything
					}},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(mcpServer, dep, r.Scheme); err != nil {
		return nil, err
	}
	return dep, nil
}

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
	// Prepare template data
	data := r.getTemplateData(mcpServer)

	// Render templates
	buildScript, err := r.renderTemplate(buildahScriptTemplate, data)
	if err != nil {
		return nil, fmt.Errorf("failed to render buildah script: %w", err)
	}

	packageJson, err := r.renderTemplate(packageJsonTemplate, data)
	if err != nil {
		return nil, fmt.Errorf("failed to render package.json: %w", err)
	}

	entrypoint, err := r.renderTemplate(entrypointTemplate, data)
	if err != nil {
		return nil, fmt.Errorf("failed to render entrypoint.sh: %w", err)
	}

	// Create Job with buildah script and templates
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
						Env: []corev1.EnvVar{
							{Name: "PACKAGE_JSON", Value: packageJson},
							{Name: "ENTRYPOINT_SH", Value: entrypoint},
						},
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

	if err := ctrl.SetControllerReference(mcpServer, job, r.Scheme); err != nil {
		return nil, err
	}

	return job, nil
}

// reconcileBuild handles the build phase - creates and monitors buildah job
// Returns: (buildCompleted bool, requeue time.Duration, error)
func (r *MCPServerReconciler) reconcileBuild(ctx context.Context, mcpServer *mcpv1.MCPServer) (bool, time.Duration, error) {
	log := logf.FromContext(ctx)

	// Check if Buildah job already exists, if not create one
	buildahJob := &batchv1.Job{}
	buildahJobName := fmt.Sprintf("buildah-%s", mcpServer.Name)
	err := r.Get(ctx, types.NamespacedName{Name: buildahJobName, Namespace: mcpServer.Namespace}, buildahJob)

	if err != nil && apierrors.IsNotFound(err) {
		// Create Buildah job to pre-build image
		job, err := r.buildahJobForMCPServer(mcpServer)
		if err != nil {
			return false, 0, fmt.Errorf("failed to create buildah job: %w", err)
		}

		log.Info("Creating Buildah Job", "Job.Namespace", job.Namespace, "Job.Name", job.Name)
		if err = r.Create(ctx, job); err != nil {
			return false, 0, fmt.Errorf("failed to create buildah job: %w", err)
		}

		// Job created, requeue to check its status
		return false, 30 * time.Second, nil
	} else if err != nil {
		return false, 0, fmt.Errorf("failed to get buildah job: %w", err)
	}

	// Check if Buildah job is completed successfully
	if buildahJob.Status.Succeeded == 0 {
		if buildahJob.Status.Failed > 0 {
			return false, 0, fmt.Errorf("buildah job failed")
		}
		// Job still running, requeue
		log.Info("Buildah Job still running, waiting...")
		return false, 30 * time.Second, nil
	}

	log.Info("Buildah Job completed successfully")
	return true, 0, nil
}
