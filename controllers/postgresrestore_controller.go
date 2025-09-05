package controllers

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	databasev1 "postgres-operator/api/v1"
	"postgres-operator/internal/postgres"
)

// PostgresRestoreReconciler reconciles a PostgresRestore object
type PostgresRestoreReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=database.example.com,resources=postgresrestores,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.example.com,resources=postgresrestores/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.example.com,resources=postgresrestores/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=pods/log,verbs=get

func (r *PostgresRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Initialize restore object
	restore := &databasev1.PostgresRestore{}
	if err := r.Get(ctx, req.NamespacedName, restore); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("PostgresRestore resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get PostgresRestore")
		return ctrl.Result{}, fmt.Errorf("failed to get PostgresRestore: %w", err)
	}

	// Handle finalizer
	if restore.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, restore)
	}

	if !controllerutil.ContainsFinalizer(restore, "database.example.com/postgres-restore") {
		controllerutil.AddFinalizer(restore, "database.example.com/postgres-restore")
		if err := r.Update(ctx, restore); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip if already completed
	if restore.Status.Phase == "Completed" {
		logger.Info("Restore already completed, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// Get referenced cluster
	cluster := &databasev1.PostgresCluster{}
	clusterKey := types.NamespacedName{
		Name:      restore.Spec.TargetCluster.Name,
		Namespace: restore.Spec.TargetCluster.Namespace,
	}
	if restore.Spec.TargetCluster.Namespace == "" {
		clusterKey.Namespace = restore.Namespace
	}

	if err := r.Get(ctx, clusterKey, cluster); err != nil {
		return r.handleError(ctx, restore, "Failed to get referenced PostgresCluster", err)
	}

	// Get referenced backup
	backup := &databasev1.PostgresBackup{}
	backupKey := types.NamespacedName{
		Name:      restore.Spec.BackupRef.Name,
		Namespace: restore.Spec.BackupRef.Namespace,
	}
	if restore.Spec.BackupRef.Namespace == "" {
		backupKey.Namespace = restore.Namespace
	}

	if err := r.Get(ctx, backupKey, backup); err != nil {
		return r.handleError(ctx, restore, "Failed to get referenced PostgresBackup", err)
	}

	// Set defaults
	r.setDefaults(restore)

	// Reconcile the restore
	if err := r.reconcileRestore(ctx, restore, cluster, backup); err != nil {
		return r.handleError(ctx, restore, "Failed to reconcile PostgreSQL restore", err)
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *PostgresRestoreReconciler) handleError(ctx context.Context, restore *databasev1.PostgresRestore, message string, err error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Error(err, message)

	restore.Status.Phase = "Failed"
	restore.Status.Message = fmt.Sprintf("%s: %v", message, err)
	if statusErr := r.Status().Update(ctx, restore); statusErr != nil {
		logger.Error(statusErr, "Failed to update status")
		return ctrl.Result{}, statusErr
	}

	return ctrl.Result{}, err
}

func (r *PostgresRestoreReconciler) setDefaults(restore *databasev1.PostgresRestore) {
	if restore.Spec.Options.Timeout == "" {
		restore.Spec.Options.Timeout = "1h"
	}
	if restore.Spec.Options.ParallelRestores == 0 {
		restore.Spec.Options.ParallelRestores = 1
	}
}

func (r *PostgresRestoreReconciler) reconcileRestore(
	ctx context.Context,
	restore *databasev1.PostgresRestore,
	cluster *databasev1.PostgresCluster,
	backup *databasev1.PostgresBackup,
) error {
	logger := log.FromContext(ctx)

	// Get target instances
	targetInstances, err := r.getTargetInstances(cluster, restore)
	if err != nil {
		return fmt.Errorf("failed to get target instances: %w", err)
	}

	// Initialize status if needed
	if len(restore.Status.InstanceStatuses) != len(targetInstances) {
		restore.Status.InstanceStatuses = make([]databasev1.InstanceRestoreStatus, len(targetInstances))
		for i, instance := range targetInstances {
			restore.Status.InstanceStatuses[i] = databasev1.InstanceRestoreStatus{
				Name:   instance.Name,
				Phase:  "Pending",
				JobName: fmt.Sprintf("%s-%s-restore", restore.Name, instance.Name),
			}
		}
		if err := r.Status().Update(ctx, restore); err != nil {
			return fmt.Errorf("failed to initialize instance statuses: %w", err)
		}
	}

	// Process each instance
	for i, instance := range targetInstances {
		instanceStatus := &restore.Status.InstanceStatuses[i]

		// Skip if already completed
		if instanceStatus.Phase == "Completed" {
			continue
		}

		// Check if job exists
		job := &batchv1.Job{}
		err := r.Get(ctx, types.NamespacedName{Name: instanceStatus.JobName, Namespace: restore.Namespace}, job)
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to check for existing job: %w", err)
		}

		// Create job if not exists
		if errors.IsNotFound(err) {
			job, err = r.createRestoreJob(restore, cluster, backup, instance)
			if err != nil {
				return fmt.Errorf("failed to create job spec: %w", err)
			}

			if err := controllerutil.SetControllerReference(restore, job, r.Scheme); err != nil {
				return fmt.Errorf("failed to set controller reference: %w", err)
			}

			if err := r.Create(ctx, job); err != nil {
				return fmt.Errorf("failed to create job: %w", err)
			}

			logger.Info("Created restore job", "job", job.Name, "instance", instance.Name)
			instanceStatus.Phase = "Running"
			instanceStatus.StartTime = &metav1.Time{Time: time.Now()}
		}

		// Update job status
		if err := r.updateJobStatus(ctx, job, instanceStatus); err != nil {
			return fmt.Errorf("failed to update job status: %w", err)
		}
	}

	// Update overall restore status
	return r.updateRestoreStatus(ctx, restore)
}

func (r *PostgresRestoreReconciler) getTargetInstances(
	cluster *databasev1.PostgresCluster,
	restore *databasev1.PostgresRestore,
) ([]databasev1.PostgresInstanceSpec, error) {
	// If no instance selector, return all instances
	if restore.Spec.Instances == nil {
		return cluster.Spec.Instances, nil
	}

	var instances []databasev1.PostgresInstanceSpec
	selector := restore.Spec.Instances

	// Handle name selection
	if len(selector.Names) > 0 {
		for _, name := range selector.Names {
			found := false
			for _, instance := range cluster.Spec.Instances {
				if instance.Name == name {
					instances = append(instances, instance)
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("instance %s not found in cluster", name)
			}
		}
		return instances, nil
	}

	// Handle role selection
	if selector.Role != "" {
		for _, instance := range cluster.Spec.Instances {
			if instance.Role == selector.Role {
				instances = append(instances, instance)
			}
		}
		if len(instances) > 0 {
			return instances, nil
		}
		return nil, fmt.Errorf("no instances with role %s found", selector.Role)
	}

	// Handle label selection
	if selector.LabelSelector != nil {
		labelSelector, err := metav1.LabelSelectorAsSelector(selector.LabelSelector)
		if err != nil {
			return nil, fmt.Errorf("invalid label selector: %w", err)
		}

		for _, instance := range cluster.Spec.Instances {
			// In a real implementation, you'd use actual instance labels
			// Here we simulate with basic labels
			instanceLabels := map[string]string{
				"instance": instance.Name,
				"role":     instance.Role,
			}
			if labelSelector.Matches(labels.Set(instanceLabels)) {
				instances = append(instances, instance)
			}
		}
		return instances, nil
	}

	// Default to all instances
	return cluster.Spec.Instances, nil
}

func (r *PostgresRestoreReconciler) createRestoreJob(
	restore *databasev1.PostgresRestore,
	cluster *databasev1.PostgresCluster,
	backup *databasev1.PostgresBackup,
	instance databasev1.PostgresInstanceSpec,
) (*batchv1.Job, error) {
	// Get storage configuration
	storage := cluster.Spec.Storage
	if instance.Storage != nil {
		storage = *instance.Storage
	}

	// Get database configuration
	dbConfig := cluster.Spec.Database
	if instance.Database != nil {
		dbConfig = *instance.Database
	}

	// Build restore command
	cmd, err := postgres.BuildRestoreCommand(backup, restore.Spec.Options, dbConfig, storage)
	if err != nil {
		return nil, fmt.Errorf("failed to build restore command: %w", err)
	}

	// Var for BackoffLimit
	BackoffLimitPtrVal := int32(0)
	// Create job object
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-restore", restore.Name, instance.Name),
			Namespace: restore.Namespace,
			Labels: map[string]string{
				"app":                        "postgres-restore",
				"postgres-restore":           restore.Name,
				"postgres-instance":          instance.Name,
				"postgres-cluster":           cluster.Name,
				"postgres-backup":            backup.Name,
				"database.example.com/restore": "true",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To(BackoffLimitPtrVal),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":               "postgres-restore",
						"postgres-restore":  restore.Name,
						"postgres-instance": instance.Name,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:    "restore",
							Image:   fmt.Sprintf("postgres:%s", cluster.Spec.PostgresVersion),
							Command: []string{"/bin/sh", "-c"},
							Args:    []string{cmd},
							Env: []corev1.EnvVar{
								{
									Name: "PGPASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: fmt.Sprintf("%s-credentials", cluster.Name),
											},
											Key: "postgres-password",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/lib/postgresql/data",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: storage.VolumeName,
								},
							},
						},
					},
				},
			},
		},
	}

	// Add backup volume
	if err := postgres.AddBackupVolumeToJob(job, backup); err != nil {
		return nil, fmt.Errorf("failed to add backup volume: %w", err)
	}

	return job, nil
}

func (r *PostgresRestoreReconciler) updateJobStatus(
	ctx context.Context,
	job *batchv1.Job,
	instanceStatus *databasev1.InstanceRestoreStatus,
) error {
	if job == nil || job.Name == "" {
		return nil
	}

	// Get current job status
	currentJob := &batchv1.Job{}
	if err := r.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, currentJob); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get job: %w", err)
	}

	// Update status based on job state
	if currentJob.Status.CompletionTime != nil {
		instanceStatus.Phase = "Completed"
		instanceStatus.CompletionTime = currentJob.Status.CompletionTime
		instanceStatus.Message = "Restore completed successfully"
	} else if currentJob.Status.Failed > 0 {
		instanceStatus.Phase = "Failed"
		instanceStatus.Message = "Restore job failed"
		for _, condition := range currentJob.Status.Conditions {
			if condition.Type == batchv1.JobFailed {
				instanceStatus.Message = condition.Message
				break
			}
		}
	} else if currentJob.Status.Active > 0 {
		instanceStatus.Phase = "Running"
		instanceStatus.Message = "Restore in progress"
		if instanceStatus.StartTime == nil {
			instanceStatus.StartTime = currentJob.Status.StartTime
		}
	}

	return nil
}

func (r *PostgresRestoreReconciler) updateRestoreStatus(
	ctx context.Context,
	restore *databasev1.PostgresRestore,
) error {
	// Initialize status counters
	var completed, failed, running int

	// Check each instance status
	for _, status := range restore.Status.InstanceStatuses {
		switch status.Phase {
		case "Completed":
			completed++
		case "Failed":
			failed++
		case "Running":
			running++
		}
	}

	// Determine overall status
	totalInstances := len(restore.Status.InstanceStatuses)
	switch {
	case failed > 0:
		restore.Status.Phase = "Failed"
		restore.Status.Message = fmt.Sprintf("%d/%d instances failed", failed, totalInstances)
	case completed == totalInstances:
		restore.Status.Phase = "Completed"
		restore.Status.Message = "All instances restored successfully"
		restore.Status.CompletionTime = &metav1.Time{Time: time.Now()}
	case running > 0:
		restore.Status.Phase = "Running"
		restore.Status.Message = fmt.Sprintf("%d/%d instances running", running, totalInstances)
	default:
		restore.Status.Phase = "Pending"
		restore.Status.Message = fmt.Sprintf("Waiting for %d instances to start", totalInstances)
	}

	// Update conditions
	restore.Status.Conditions = r.buildConditions(restore)

	// Update status
	return r.Status().Update(ctx, restore)
}

func (r *PostgresRestoreReconciler) buildConditions(restore *databasev1.PostgresRestore) []metav1.Condition {
	now := metav1.Now()
	var conditions []metav1.Condition

	switch restore.Status.Phase {
	case "Completed":
		conditions = append(conditions, metav1.Condition{
			Type:               "Completed",
			Status:             metav1.ConditionTrue,
			Reason:             "AllInstancesRestored",
			Message:            restore.Status.Message,
			LastTransitionTime: now,
		})
	case "Failed":
		conditions = append(conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			Reason:             "SomeInstancesFailed",
			Message:            restore.Status.Message,
			LastTransitionTime: now,
		})
	case "Running":
		conditions = append(conditions, metav1.Condition{
			Type:               "Progressing",
			Status:             metav1.ConditionTrue,
			Reason:             "RestoreInProgress",
			Message:            restore.Status.Message,
			LastTransitionTime: now,
		})
	default:
		conditions = append(conditions, metav1.Condition{
			Type:               "Progressing",
			Status:             metav1.ConditionFalse,
			Reason:             "RestorePending",
			Message:            restore.Status.Message,
			LastTransitionTime: now,
		})
	}

	return conditions
}

func (r *PostgresRestoreReconciler) handleDeletion(ctx context.Context, restore *databasev1.PostgresRestore) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Delete all associated jobs
	for _, instanceStatus := range restore.Status.InstanceStatuses {
		if instanceStatus.JobName != "" {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceStatus.JobName,
					Namespace: restore.Namespace,
				},
			}
			if err := r.Delete(ctx, job); client.IgnoreNotFound(err) != nil {
				logger.Error(err, "Failed to delete restore job", "job", instanceStatus.JobName)
				return ctrl.Result{}, err
			}
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(restore, "database.example.com/postgres-restore")
	if err := r.Update(ctx, restore); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *PostgresRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1.PostgresRestore{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}