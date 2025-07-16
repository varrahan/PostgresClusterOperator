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

func (bm *BackupManager) CreateBackupJob(
    backup *databasev1.PostgresBackup,
    cluster *databasev1.PostgresCluster,
    instance *databasev1.PostgresInstanceSpec,
) (*batchv1.Job, error) {
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

    // Determine target host
    targetHost := cluster.Status.CurrentPrimary // Default to cluster primary
    if instance != nil && instance.Name != "" {
        targetHost = fmt.Sprintf("%s-%s", cluster.Name, instance.Name)
    }

    // Initialize command and args based on backup type
    var command []string
    var args []string
    var env []corev1.EnvVar

    switch backup.Spec.Type {
    case "logical":
        // For logical backups (pg_dump), we need the database name
        dbName := cluster.Spec.Database.Name
        if instance != nil && instance.Database != nil && instance.Database.Name != "" {
            dbName = instance.Database.Name
        }

        command = []string{"pg_dump"}
        args = []string{
            "-h", targetHost,
            "-U", "postgres",
            "-d", dbName,
            "-f", "/backup/data.sql",
            "-F", "c",
            "-Z", fmt.Sprintf("%d", backup.Spec.Options.Compression),
        }
        if backup.Spec.Options.ParallelJobs > 1 {
            args = append(args, "-j", fmt.Sprintf("%d", backup.Spec.Options.ParallelJobs))
        }

        // Add any instance-specific parameters if needed
        if instance != nil && instance.Config != nil {
            for param, value := range instance.Config {
                args = append(args, "--"+param, value)
            }
        }

    default: // "physical" or unspecified
        // For physical backups (pg_basebackup), we don't need the database name
        command = []string{"pg_basebackup"}
        args = []string{
            "-D", "/backup",
            "-h", targetHost,
            "-U", "postgres",
            "--compress=" + string(backup.Spec.Options.Compression),
            "--jobs", fmt.Sprintf("%d", backup.Spec.Options.ParallelJobs),
        }
    }

    // Common environment variables
    env = []corev1.EnvVar{
        {
            Name: "PGPASSWORD",
            ValueFrom: &corev1.EnvVarSource{
                SecretKeyRef: &corev1.SecretKeySelector{
                    LocalObjectReference: corev1.LocalObjectReference{
                        Name: cluster.Name + "-credentials",
                    },
                    Key: "postgres-password",
                },
            },
        },
    }

    // Add instance-specific init script if available for logical backups
    if backup.Spec.Type == "logical" && instance != nil && instance.Database != nil && instance.Database.InitScript != "" {
        env = append(env, corev1.EnvVar{
            Name:  "PGINITSCRIPT",
            Value: instance.Database.InitScript,
        })
    }

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
                            Name:         "backup",
                            Image:        image,
                            Command:      command,
                            Args:         args,
                            VolumeMounts: []corev1.VolumeMount{
                                {
                                    Name:      "backup-storage",
                                    MountPath: "/backup",
                                },
                            },
                            Env: env,
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

    // Add instance-specific labels if provided
    if instance != nil {
        job.Labels["database.example.com/instance"] = instance.Name
        if instance.Role != "" {
            job.Labels["database.example.com/role"] = instance.Role
        }
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