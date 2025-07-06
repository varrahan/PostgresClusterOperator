package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PostgresBackupSpec defines the desired state of PostgresBackup
type PostgresBackupSpec struct {
	// ClusterRef references the PostgresCluster to backup
	ClusterRef ClusterReference `json:"clusterRef"`

	// Type of backup (full, incremental, wal)
	Type string `json:"type,omitempty"`

	// Storage configuration for this backup
	Storage BackupStorageSpec `json:"storage,omitempty"`

	// Retention policy for this backup
	RetentionPolicy string `json:"retentionPolicy,omitempty"`

	// Options for the backup
	Options BackupOptions `json:"options,omitempty"`
}

type BackupOptions struct {
	// Compression type (gzip, lz4, none)
	Compression string `json:"compression,omitempty"`

	// Encryption configuration
	Encryption EncryptionSpec `json:"encryption,omitempty"`

	// Parallel jobs for backup
	ParallelJobs int32 `json:"parallelJobs,omitempty"`

	// Timeout for backup operation
	Timeout string `json:"timeout,omitempty"`
}

type EncryptionSpec struct {
	// Enabled indicates if encryption is enabled
	Enabled bool `json:"enabled,omitempty"`

	// SecretRef references the encryption key
	SecretRef *SecretReference `json:"secretRef,omitempty"`
}

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
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterRef.name`
//+kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.status.size`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PostgresBackup is the Schema for the postgresbackups API
type PostgresBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgresBackupSpec   `json:"spec,omitempty"`
	Status PostgresBackupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PostgresBackupList contains a list of PostgresBackup
type PostgresBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgresBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgresBackup{}, &PostgresBackupList{})
}