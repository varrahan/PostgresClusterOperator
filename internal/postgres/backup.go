package postgres

import (
	"fmt"
	"context"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	databasev1 "postgres-operator/api/v1"
)

// BackupManager handles backup operations
type BackupManager struct {
	client.Client
}

// NewBackupManager creates a new backup manager
func NewBackupManager(client client.Client) *BackupManager {
	return &BackupManager{
		Client: client,
	}
}

// CreateBackupJob creates a Kubernetes Job to perform the backup
func (bm *BackupManager) CreateBackupJob(backup *databasev1.PostgresBackup, cluster *databasev1.PostgresCluster) (*batchv1.Job, error) {
	// Get PVC name from cluster's backup storage config
	pvcName, ok := cluster.Spec.Backup.Storage.Config["pvcName"]
	if !ok || pvcName == "" {
		return nil, fmt.Errorf("pvcName not set in cluster backup storage config")
	}

	// Build job name (CR name + "-job")
	jobName := backup.Name + "-job"
	if len(jobName) > 63 {
		jobName = jobName[:63]
	}

	// Construct image using cluster's PostgresVersion
	image := "postgres:" + cluster.Spec.PostgresVersion

	// Create job object
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: backup.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "postgres-backup",
				"app.kubernetes.io/instance":   backup.Spec.ClusterRef.Name,
				"app.kubernetes.io/managed-by": "postgres-operator",
				"database.example.com/cluster": backup.Spec.ClusterRef.Name,
				"database.example.com/type":    backup.Spec.Type,
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":       "postgres-backup",
						"app.kubernetes.io/instance":   backup.Spec.ClusterRef.Name,
						"app.kubernetes.io/managed-by": "postgres-operator",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "backup",
							Image:   image,
							Command: []string{"pg_basebackup"},
							Args: []string{
								"-D", "/backup",
								"-h", cluster.Status.CurrentPrimary,
								"-U", "postgres",
								"--compress=" + string(backup.Spec.Options.Compression),
								"--jobs", fmt.Sprintf("%d", backup.Spec.Options.ParallelJobs),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "backup-storage",
									MountPath: "/backup",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name: "PGPASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												// Use database name + "-password" convention
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
							Name: "backup-storage",
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

// ListBackups lists all backups for a cluster
func (bm *BackupManager) ListBackups(ctx context.Context, clusterName, namespace string) (*databasev1.PostgresBackupList, error) {
	backupList := &databasev1.PostgresBackupList{}
	
	err := bm.List(ctx, backupList, client.InNamespace(namespace), client.MatchingLabels{
		"database.example.com/cluster": clusterName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list backups: %w", err)
	}

	return backupList, nil
}

// DeleteBackup deletes a backup
func (bm *BackupManager) DeleteBackup(ctx context.Context, backupName, namespace string) error {
	backup := &databasev1.PostgresBackup{}
	err := bm.Get(ctx, types.NamespacedName{Name: backupName, Namespace: namespace}, backup)
	if err != nil {
		return fmt.Errorf("failed to get backup: %w", err)
	}

	err = bm.Delete(ctx, backup)
	if err != nil {
		return fmt.Errorf("failed to delete backup: %w", err)
	}

	return nil
}