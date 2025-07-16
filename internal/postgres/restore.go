package postgres

import (
	"fmt"
	"strings"

	databasev1 "postgres-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	batchv1 "k8s.io/api/batch/v1"
)

// BuildRestoreCommand constructs the pg_restore command based on the backup and restore specifications
func BuildRestoreCommand(
	backup *databasev1.PostgresBackup,
	options databasev1.RestoreOptions,
	dbConfig databasev1.DatabaseSpec,
	storage databasev1.StorageSpec,
) (string, error) {
	if backup == nil {
		return "", fmt.Errorf("backup cannot be nil")
	}

	// Base command
	cmd := "pg_restore"

	// Add common options
	cmd += fmt.Sprintf(" --host=${POSTGRES_HOST:-localhost}")
	cmd += fmt.Sprintf(" --port=${POSTGRES_PORT:-5432}")
	cmd += fmt.Sprintf(" --username=${POSTGRES_USER:-postgres}")
	cmd += fmt.Sprintf(" --dbname=%s", dbConfig.Name)

	// Add format-specific options based on backup type
	switch backup.Spec.Type {
	case "logical":
		cmd += " --format=c"
		cmd += fmt.Sprintf(" --file=%s", "/backup/data.dump")
	case "physical":
		cmd += " --format=directory"
		cmd += fmt.Sprintf(" --directory=%s", "/backup")
	default:
		return "", fmt.Errorf("unsupported backup type: %s", backup.Spec.Type)
	}

	// Add restore options
	if options.DropExisting {
		cmd += " --clean"
	}
	if options.DataOnly {
		cmd += " --data-only"
	}
	if options.SchemaOnly {
		cmd += " --schema-only"
	}
	if options.ParallelRestores > 1 {
		cmd += fmt.Sprintf(" --jobs=%d", options.ParallelRestores)
	}
	if options.Timeout != "" {
		cmd += fmt.Sprintf(" --timeout=%s", options.Timeout)
	}

	// Add database filter if specified
	if options.DatabaseFilter != nil {
		if len(options.DatabaseFilter.Include) > 0 {
			cmd += fmt.Sprintf(" --dbname=%s", strings.Join(options.DatabaseFilter.Include, ","))
		}
		if len(options.DatabaseFilter.Exclude) > 0 {
			for _, db := range options.DatabaseFilter.Exclude {
				cmd += fmt.Sprintf(" --exclude-database=%s", db)
			}
		}
	}

	// Add WAL information if available
	if backup.Status.WALStart != "" {
		cmd += fmt.Sprintf(" --wal-start=%s", backup.Status.WALStart)
	}
	if backup.Status.WALEnd != "" {
		cmd += fmt.Sprintf(" --wal-end=%s", backup.Status.WALEnd)
	}

	return cmd, nil
}

// AddBackupVolumeToJob adds the appropriate volume to the job based on the backup storage type
func AddBackupVolumeToJob(job *batchv1.Job, backup *databasev1.PostgresBackup) error {
	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}
	if backup == nil {
		return fmt.Errorf("backup cannot be nil")
	}

	switch backup.Spec.Storage.Type {
	case "local":
		// For local backups, we expect a PVC with the backup data
		pvcName, ok := backup.Spec.Storage.Config["pvcName"]
		if !ok || pvcName == "" {
			return fmt.Errorf("pvcName not specified in local backup storage config")
		}

		volume := corev1.Volume{
			Name: "backup",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		}

		// Add volume to pod spec
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, volume)

		// Add volume mount to container
		container := &job.Spec.Template.Spec.Containers[0]
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "backup",
			MountPath: "/backup",
			ReadOnly:  true,
		})

	case "s3":
		// For S3 backups, we need to configure AWS credentials and download the backup first
		// This would be done via an init container
		initContainer := corev1.Container{
			Name:    "download-backup",
			Image:   "amazon/aws-cli:latest",
			Command: []string{"sh", "-c"},
			Args: []string{
				fmt.Sprintf("aws s3 cp s3://%s/%s /backup --recursive",
					backup.Spec.Storage.Config["bucket"],
					backup.Spec.Storage.Config["path"]),
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "backup",
					MountPath: "/backup",
				},
			},
			Env: []corev1.EnvVar{
				{
					Name: "AWS_ACCESS_KEY_ID",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: backup.Spec.Storage.Config["secretName"],
							},
							Key: "accessKey",
						},
					},
				},
				{
					Name: "AWS_SECRET_ACCESS_KEY",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: backup.Spec.Storage.Config["secretName"],
							},
							Key: "secretKey",
						},
					},
				},
				{
					Name:  "AWS_DEFAULT_REGION",
					Value: backup.Spec.Storage.Config["region"],
				},
			},
		}

		// Add emptyDir volume for the downloaded backup
		volume := corev1.Volume{
			Name: "backup",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}

		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, volume)
		job.Spec.Template.Spec.InitContainers = append(job.Spec.Template.Spec.InitContainers, initContainer)

		// Add volume mount to main container
		container := &job.Spec.Template.Spec.Containers[0]
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "backup",
			MountPath: "/backup",
		})

	default:
		return fmt.Errorf("unsupported backup storage type: %s", backup.Spec.Storage.Type)
	}

	return nil
}