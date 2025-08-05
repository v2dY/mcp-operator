package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	mcpv1 "github.com/v2dY/project/api/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// Build container image and tag
	buildahImage = "quay.io/buildah/stable"
	buildahTag   = "v1.40.1"
)

// BuildManager handles build operations
type BuildManager struct {
	client.Client
	scheme           *runtime.Scheme
	templateRenderer *TemplateRenderer
}

// NewBuildManager creates a new build manager
func NewBuildManager(c client.Client, scheme *runtime.Scheme) *BuildManager {
	return &BuildManager{
		Client:           c,
		scheme:           scheme,
		templateRenderer: NewTemplateRenderer(),
	}
}

// ReconcileBuild handles the build phase - creates and monitors buildah job
// Returns: (buildCompleted bool, requeue time.Duration, error)
func (bm *BuildManager) ReconcileBuild(ctx context.Context, mcpServer *mcpv1.MCPServer) (bool, time.Duration, error) {
	log := logf.FromContext(ctx)

	// Validate input
	if mcpServer == nil {
		return false, 0, fmt.Errorf("mcpServer cannot be nil")
	}

	// Check if Buildah job already exists, if not create one
	buildahJob := &batchv1.Job{}
	buildahJobName := fmt.Sprintf("%s-%s", buildJobPrefix, mcpServer.Name)
	err := bm.Get(ctx, types.NamespacedName{Name: buildahJobName, Namespace: mcpServer.Namespace}, buildahJob)

	if err != nil && apierrors.IsNotFound(err) {
		// Create Buildah job to pre-build image
		job, err := bm.buildBuildahJob(mcpServer)
		if err != nil {
			return false, 0, fmt.Errorf("failed to create buildah job: %w", err)
		}

		log.Info("Creating Buildah Job", "Job.Namespace", job.Namespace, "Job.Name", job.Name)
		if err = bm.Create(ctx, job); err != nil {
			return false, 0, fmt.Errorf("failed to create buildah job: %w", err)
		}

		// Job created, requeue to check its status
		return false, mediumRequeue, nil
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
		return false, mediumRequeue, nil
	}

	log.Info("Buildah Job completed successfully")
	return true, 0, nil
}

// buildBuildahJob creates a Buildah Job to build pre-configured MCP server image
func (bm *BuildManager) buildBuildahJob(mcpServer *mcpv1.MCPServer) (*batchv1.Job, error) {
	// Prepare template data
	data := GetTemplateData(mcpServer)

	// Render templates
	buildScript, err := bm.templateRenderer.RenderBuildahScript(data)
	if err != nil {
		return nil, fmt.Errorf("failed to render buildah script: %w", err)
	}

	entrypoint, err := bm.templateRenderer.RenderEntrypoint(data)
	if err != nil {
		return nil, fmt.Errorf("failed to render entrypoint.sh: %w", err)
	}

	packageJson, err := bm.templateRenderer.RenderPackageJson(data)
	if err != nil {
		return nil, fmt.Errorf("failed to render package.json: %w", err)
	}

	dockerfile, err := bm.templateRenderer.RenderDockerfile(data)
	if err != nil {
		return nil, fmt.Errorf("failed to render Dockerfile: %w", err)
	}

	// Create an enhanced build script that includes file writing
	// Remove the shebang from buildScript since our enhanced script already has one
	buildScriptWithoutShebang := buildScript
	if len(buildScript) > 0 && buildScript[0:2] == "#!" {
		lines := strings.Split(buildScript, "\n")
		if len(lines) > 1 {
			buildScriptWithoutShebang = strings.Join(lines[1:], "\n")
		}
	}

	enhancedBuildScript := fmt.Sprintf(`#!/bin/bash
set -e

# Ensure the build context directory exists
mkdir -p /var/lib/containers/build-context


# Write rendered templates to files
cat > /var/lib/containers/build-context/Dockerfile << 'DOCKERFILE_EOF'
%s
DOCKERFILE_EOF

cat > /var/lib/containers/build-context/package.json << 'PACKAGE_EOF'
%s
PACKAGE_EOF

cat > /var/lib/containers/build-context/entrypoint.sh << 'ENTRYPOINT_EOF'
%s
ENTRYPOINT_EOF

# Make entrypoint executable
chmod +x /var/lib/containers/build-context/entrypoint.sh

# List files in the build context directory
ls -la /var/lib/containers/build-context

# Execute the main build script
%s
`, dockerfile, packageJson, entrypoint, buildScriptWithoutShebang)

	labels := GetLabels(mcpServer)
	labels["job-type"] = "buildah"

	// Create Job with buildah script and templates
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", buildJobPrefix, mcpServer.Name),
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:    "buildah",
						Image:   fmt.Sprintf("%s:%s", buildahImage, buildahTag),
						Command: []string{"/bin/bash"},
						Args:    []string{"-c", enhancedBuildScript},
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

	if err := ctrl.SetControllerReference(mcpServer, job, bm.scheme); err != nil {
		return nil, err
	}

	return job, nil
}
