package controllers

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	databasev1 "postgres-operator/api/v1"
	"postgres-operator/internal/k8s"
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

func (r *PostgresRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the PostgresRestore instance
	restore := &databasev1.PostgresRestore{}
	err := r.Get(ctx, req.NamespacedName, restore)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("PostgresRestore resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get PostgresRestore")
		return ctrl.Result{}, err
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(restore, "database.example.com/postgres-restore") {
		controllerutil.AddFinalizer(restore, "database.example.com/postgres-restore")
		return ctrl.Result{}, r.Update(ctx, restore)
	}

	// Handle deletion
	if restore.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, restore)
	}

	// Get the referenced cluster
	cluster := &databasev1.PostgresCluster{}
	clusterKey := types.NamespacedName{
		Name:      restore.Spec.TargetCluster.Name,
		Namespace: restore.Spec.TargetCluster.Namespace,
	}
	if restore.Spec.TargetCluster.Namespace == "" {
		clusterKey.Namespace = restore.Namespace
	}

	err = r.Get(ctx, clusterKey, cluster)
	if err != nil {
		logger.Error(err, "Failed to get referenced PostgresCluster")
		restore.Status.Phase = "Failed"
		restore.Status.Message = fmt.Sprintf("Failed to get cluster %s: %v", restore.Spec.TargetCluster.Name, err)
		return ctrl.Result{}, r.Status().Update(ctx, restore)
	}

	// Set defaults
	r.setDefaults(restore)

	// Reconcile the restore
	if err := r.reconcileRestore(ctx, restore, cluster); err != nil {
		logger.Error(err, "Failed to reconcile PostgreSQL restore")
		restore.Status.Phase = "Failed"
		restore.Status.Message = fmt.Sprintf("Failed to reconcile restore: %v", err)
		r.Status().Update(ctx, restore)
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

func (r *PostgresRestoreReconciler) setDefaults(restore *databasev1.PostgresRestore) {
	if restore.Spec.Options.Timeout == "" {
		restore.Spec.Options.Timeout = "1h"
	}
}

func (r *PostgresRestoreReconciler) reconcileRestore(ctx context.Context, restore *databasev1.PostgresRestore, cluster *databasev1.PostgresCluster) error {
	// Get the referenced backup
	backup := &databasev1.PostgresBackup{}
	backupKey := types.NamespacedName{
		Name:      restore.Spec.BackupRef.Name,
		Namespace: restore.Spec.BackupRef.Namespace,
	}
	if restore.Spec.BackupRef.Namespace == "" {
		backupKey.Namespace = restore.Namespace
	}

	if err := r.Get(ctx, backupKey, backup); err != nil {
		return fmt.Errorf("failed to get backup %s: %w", restore.Spec.BackupRef.Name, err)
	}

	// Create restore job
	job, err := r.createRestoreJob(restore, cluster, backup)
	if err != nil {
		return fmt.Errorf("failed to create restore job: %w", err)
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(restore, job, r.Scheme); err != nil {
		return err
	}

	// Create or update the job using k8s utils
	if err := k8s.CreateOrUpdate(ctx, r.Client, job); err != nil {
		return fmt.Errorf("failed to create restore job: %w", err)
	}

	// Update restore status based on job status
	return r.updateRestoreStatus(ctx, restore, job)
}

func (r *PostgresRestoreReconciler) createRestoreJob(restore *databasev1.PostgresRestore, cluster *databasev1.PostgresCluster, backup *databasev1.PostgresBackup) (*batchv1.Job, error) {
	// Get PVC name from cluster's storage config
	pvcName := cluster.Spec.Storage.VolumeName
	if pvcName == "" {
		return nil, fmt.Errorf("volumeName not set in cluster storage config")
	}

	// Get backup PVC name from backup storage config
	backupPvcName, ok := backup.Spec.Storage.Config["pvcName"]
	if !ok || backupPvcName == "" {
		return nil, fmt.Errorf("pvcName not set in backup storage config")
	}

	jobName := restore.Name + "-job"
	if len(jobName) > 63 {
		jobName = jobName[:63]
	}

	image := "postgres:" + cluster.Spec.PostgresVersion

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: restore.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "postgres-restore",
				"app.kubernetes.io/instance":   restore.Spec.TargetCluster.Name,
				"app.kubernetes.io/managed-by": "postgres-operator",
				"database.example.com/cluster": restore.Spec.TargetCluster.Name,
				"database.example.com/backup":  backup.Name,
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "restore",
							Image:   image,
							Command: []string{"sh", "-c"},
							Args: []string{
								fmt.Sprintf("pg_restore -h %s -U postgres -d %s -Fd /backup %s %s %s",
									cluster.Status.CurrentPrimary,
									cluster.Spec.Database.Name,
									r.getRestoreFlags(restore),
									"--jobs=1",
									fmt.Sprintf("--timeout=%s", restore.Spec.Options.Timeout),
								),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "backup-volume",
									MountPath: "/backup",
									ReadOnly:  true,
								},
								{
									Name:      "data-volume",
									MountPath: "/var/lib/postgresql/data",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name: "PGPASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: cluster.Spec.Database.Name + "-password",
											},
											Key: "password",
										},
									},
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Volumes: []corev1.Volume{
						{
							Name: "backup-volume",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: backupPvcName,
								},
							},
						},
						{
							Name: "data-volume",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					},
				},
			},
		},
	}

	return job, nil
}

func (r *PostgresRestoreReconciler) getRestoreFlags(restore *databasev1.PostgresRestore) string {
	var flags string
	if restore.Spec.Options.DropExisting {
		flags += "--clean "
	}
	if restore.Spec.Options.DataOnly {
		flags += "--data-only "
	}
	if restore.Spec.Options.SchemaOnly {
		flags += "--schema-only "
	}
	return flags
}

func (r *PostgresRestoreReconciler) updateRestoreStatus(ctx context.Context, restore *databasev1.PostgresRestore, job *batchv1.Job) error {
	currentJob := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, currentJob)
	if err != nil {
		return err
	}

	if currentJob.Status.CompletionTime != nil {
		restore.Status.Phase = "Completed"
		restore.Status.Message = "Restore completed successfully"
		restore.Status.CompletionTime = currentJob.Status.CompletionTime
	} else if currentJob.Status.Failed > 0 {
		restore.Status.Phase = "Failed"
		restore.Status.Message = "Restore job failed"
		for _, condition := range currentJob.Status.Conditions {
			if condition.Type == batchv1.JobFailed {
				restore.Status.Message = fmt.Sprintf("Restore job failed: %s", condition.Message)
				break
			}
		}
	} else if currentJob.Status.Active > 0 {
		restore.Status.Phase = "Running"
		restore.Status.Message = "Restore job is running"
	} else {
		restore.Status.Phase = "Pending"
		restore.Status.Message = "Restore job is pending"
	}

	restore.Status.JobName = currentJob.Name
	restore.Status.Conditions = r.buildRestoreConditions(currentJob)

	return r.Status().Update(ctx, restore)
}

func (r *PostgresRestoreReconciler) buildRestoreConditions(job *batchv1.Job) []metav1.Condition {
	var conditions []metav1.Condition
	for _, jobCondition := range job.Status.Conditions {
		conditions = append(conditions, metav1.Condition{
			Type:               string(jobCondition.Type),
			Status:             metav1.ConditionStatus(jobCondition.Status),
			LastTransitionTime: jobCondition.LastTransitionTime,
			Reason:             jobCondition.Reason,
			Message:            jobCondition.Message,
		})
	}
	return conditions
}

func (r *PostgresRestoreReconciler) handleDeletion(ctx context.Context, restore *databasev1.PostgresRestore) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling deletion of PostgresRestore", "restore", restore.Name)

	if restore.Status.JobName != "" {
		job := &batchv1.Job{}
		err := r.Get(ctx, types.NamespacedName{Name: restore.Status.JobName, Namespace: restore.Namespace}, job)
		if err == nil {
			logger.Info("Deleting restore job", "job", restore.Status.JobName)
			if err := r.Delete(ctx, job); client.IgnoreNotFound(err) != nil {
				logger.Error(err, "Failed to delete restore job")
				return ctrl.Result{}, err
			}
		}
	}

	controllerutil.RemoveFinalizer(restore, "database.example.com/postgres-restore")
	return ctrl.Result{}, r.Update(ctx, restore)
}

func (r *PostgresRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1.PostgresRestore{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}