package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +groupName=database.example.com

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// BackupReference references a PostgresBackup resource
type BackupReference struct {
	// Name of the backup
	Name string `json:"name"`

	// Namespace of the backup
	Namespace string `json:"namespace,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// PostgresBackupSpec defines the desired state of PostgresBackup
type PostgresBackupSpec struct {
	// ClusterRef references the PostgresCluster to backup
	ClusterRef ClusterReference `json:"clusterRef"`

	// Type of backup (full, incremental, wal)
	Type string `json:"type,omitempty"`

	// Storage configuration for this backup
	Storage BackupStorageSpec `json:"storage,omitempty"`

	// Retention policy for this backup
	RetentionPolicy RetentionPolicy `json:"retentionPolicy,omitempty"`

	// Options for the backup
	Options BackupOptions `json:"options,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// RetentionPolicy defines how many backups are kept over a specific period
type RetentionPolicy struct {
	// KeepLast specifies how many most recent backups to keep
	KeepLast int32 `json:"keepLast,omitempty"`

	// KeepDaily specifies how many daily backups to keep
	KeepDaily int32 `json:"keepDaily,omitempty"`

	// KeepWeekly specifies how many weekly backups to keep
	KeepWeekly int32 `json:"keepWeekly,omitempty"`

	// KeepMonthly specifies how many monthly backups to keep
	KeepMonthly int32 `json:"keepMonthly,omitempty"`

	// DeleteOnClusterDeletion determines if backup should be deleted when cluster is deleted
	DeleteOnClusterDeletion bool `json:"deleteOnClusterDeletion,omitempty"`
}

// +kubebuilder:validation:Enum=gzip;lz4;none
type CompressionType string

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// BackupOptions defines how the database backup will happen
type BackupOptions struct {
	// Compression type (gzip, lz4, none)
	Compression CompressionType `json:"compression,omitempty"`

	// Encryption configuration
	Encryption EncryptionSpec `json:"encryption,omitempty"`

	// Parallel jobs for backup
	ParallelJobs int32 `json:"parallelJobs,omitempty"`

	// Timeout for backup operation
	Timeout string `json:"timeout,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// EncryptionSpec defines if a resource is encrypted
type EncryptionSpec struct {
	// Enabled indicates if encryption is enabled
	Enabled bool `json:"enabled,omitempty"`

	// SecretRef references the encryption key
	SecretRef *SecretReference `json:"secretRef,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// PostgresBackupStatus defines the observed state of PostgresBackup
type PostgresBackupStatus struct {
	// Phase represents the current phase of the backup
	Phase string `json:"phase,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// StartTime of the backup
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime of the backup
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Size of the backup in bytes
	Size int64 `json:"size,omitempty"`

	// Location where the backup is stored
	Location string `json:"location,omitempty"`

	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// DatabaseVersion at the time of backup
	DatabaseVersion string `json:"databaseVersion,omitempty"`

	// WALStart position
	WALStart string `json:"walStart,omitempty"`

	// WALEnd position
	WALEnd string `json:"walEnd,omitempty"`

	// JobName of the backup job
	JobName string `json:"jobName,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=postgresbackups,scope=Namespaced,singular=postgresbackup,shortName=pgbackup;pgbackups
// +kubebuilder:storageversion
// +groupName=database.example.com  // ADD THIS (change to your domain)
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterRef.name`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.status.size`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// PostgresBackup is the Schema for the postgresbackups API
type PostgresBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgresBackupSpec   `json:"spec,omitempty"`
	Status PostgresBackupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// PostgresBackupList contains a list of PostgresBackup
type PostgresBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgresBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgresBackup{}, &PostgresBackupList{})
}
