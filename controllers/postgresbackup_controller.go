package controllers

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
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
	"postgres-operator/internal/postgres"
)

// PostgresBackupReconciler reconciles a PostgresBackup object
type PostgresBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=database.example.com,resources=postgresbackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=database.example.com,resources=postgresbackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=database.example.com,resources=postgresbackups/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *PostgresBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the PostgresBackup instance
	backup := &databasev1.PostgresBackup{}
	err := r.Get(ctx, req.NamespacedName, backup)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("PostgresBackup resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get PostgresBackup")
		return ctrl.Result{}, err
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(backup, "database.example.com/postgres-backup") {
		controllerutil.AddFinalizer(backup, "database.example.com/postgres-backup")
		return ctrl.Result{}, r.Update(ctx, backup)
	}

	// Handle deletion
	if backup.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, backup)
	}

	// Get the referenced cluster
	cluster := &databasev1.PostgresCluster{}
	clusterKey := types.NamespacedName{
		Name:      backup.Spec.ClusterRef.Name,
		Namespace: backup.Spec.ClusterRef.Namespace,
	}
	if backup.Spec.ClusterRef.Namespace == "" {
		clusterKey.Namespace = backup.Namespace
	}

	err = r.Get(ctx, clusterKey, cluster)
	if err != nil {
		logger.Error(err, "Failed to get referenced PostgresCluster")
		backup.Status.Phase = "Failed"
		backup.Status.Message = fmt.Sprintf("Failed to get cluster %s: %v", backup.Spec.ClusterRef.Name, err)
		return ctrl.Result{}, r.Status().Update(ctx, backup)
	}

	// Set defaults
	r.setDefaults(backup)

	// Reconcile the backup
    if err := r.reconcileBackup(ctx, backup, cluster); err != nil {
        logger.Error(err, "Failed to reconcile PostgreSQL backup")
        backup.Status.Phase = "Failed"
        backup.Status.Message = fmt.Sprintf("Failed to reconcile backup: %v", err)
        if err := r.Status().Update(ctx, backup); err != nil {
            logger.Error(err, "Failed to update backup status")
        }
        return ctrl.Result{}, err
    }

    return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

func (r *PostgresBackupReconciler) setDefaults(backup *databasev1.PostgresBackup) {
	if backup.Spec.Type == "" {
		backup.Spec.Type = "full"
	}
	if backup.Spec.Options.Compression == "" {
		backup.Spec.Options.Compression = "gzip"
	}
	if backup.Spec.Options.ParallelJobs == 0 {
		backup.Spec.Options.ParallelJobs = 1
	}
	if backup.Spec.Options.Timeout == "" {
		backup.Spec.Options.Timeout = "1h"
	}
}

func (r *PostgresBackupReconciler) reconcileBackup(ctx context.Context, backup *databasev1.PostgresBackup, cluster *databasev1.PostgresCluster) error {
	// Use the internal BackupManager
	backupManager := postgres.NewBackupManager(r.Client)
	
	// CHANGED: Call CreateBackupJob instead of CreateBackup
	job, err := backupManager.CreateBackupJob(backup, cluster)
	if err != nil {
		return fmt.Errorf("failed to create backup job: %w", err)
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(backup, job, r.Scheme); err != nil {
		return err
	}

	// Create or update the job using k8s utils
	if err := k8s.CreateOrUpdate(ctx, r.Client, job); err != nil {
		return fmt.Errorf("failed to create backup job: %w", err)
	}

	// Update backup status based on job status
	return r.updateBackupStatus(ctx, backup, job)
}

func (r *PostgresBackupReconciler) updateBackupStatus(ctx context.Context, backup *databasev1.PostgresBackup, job *batchv1.Job) error {
	// Get the current job status
	currentJob := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, currentJob)
	if err != nil {
		return err
	}

	// Update backup status based on job status
	if currentJob.Status.CompletionTime != nil {
		backup.Status.Phase = "Completed"
		backup.Status.Message = "Backup completed successfully"
		backup.Status.CompletionTime = currentJob.Status.CompletionTime
	} else if currentJob.Status.Failed > 0 {
		backup.Status.Phase = "Failed"
		backup.Status.Message = "Backup job failed"
		// Try to get failure reason from job conditions
		for _, condition := range currentJob.Status.Conditions {
			if condition.Type == batchv1.JobFailed {
				backup.Status.Message = fmt.Sprintf("Backup job failed: %s", condition.Message)
				break
			}
		}
	} else if currentJob.Status.Active > 0 {
		backup.Status.Phase = "Running"
		backup.Status.Message = "Backup job is running"
		if backup.Status.StartTime == nil {
			backup.Status.StartTime = currentJob.Status.StartTime
		}
	} else {
		backup.Status.Phase = "Pending"
		backup.Status.Message = "Backup job is pending"
	}

	// Update additional status fields
	backup.Status.JobName = currentJob.Name
	backup.Status.Conditions = r.buildBackupConditions(currentJob)

	return r.Status().Update(ctx, backup)
}

func (r *PostgresBackupReconciler) buildBackupConditions(job *batchv1.Job) []metav1.Condition {
	var conditions []metav1.Condition
	
	for _, jobCondition := range job.Status.Conditions {
		condition := metav1.Condition{
			Type:               string(jobCondition.Type),
			Status:             metav1.ConditionStatus(jobCondition.Status),
			LastTransitionTime: jobCondition.LastTransitionTime,
			Reason:             jobCondition.Reason,
			Message:            jobCondition.Message,
		}
		conditions = append(conditions, condition)
	}
	
	return conditions
}

func (r *PostgresBackupReconciler) handleDeletion(ctx context.Context, backup *databasev1.PostgresBackup) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling deletion of PostgresBackup", "backup", backup.Name)

	// Delete associated backup job if it exists
	if backup.Status.JobName != "" {
		job := &batchv1.Job{}
		err := r.Get(ctx, types.NamespacedName{Name: backup.Status.JobName, Namespace: backup.Namespace}, job)
		if err == nil {
			logger.Info("Deleting backup job", "job", backup.Status.JobName)
			if err := r.Delete(ctx, job); client.IgnoreNotFound(err) != nil {
				logger.Error(err, "Failed to delete backup job")
				return ctrl.Result{}, err
			}
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(backup, "database.example.com/postgres-backup")
	return ctrl.Result{}, r.Update(ctx, backup)
}

func (r *PostgresBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1.PostgresBackup{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}