package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +groupName=database.example.com

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// BackupReference references a PostgresBackup resource
type BackupReference struct {
	Name string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// Specifications for the backup
type PostgresBackupSpec struct {
	ClusterRef ClusterReference `json:"clusterRef"`
	Type string `json:"type,omitempty"`
	Storage BackupStorageSpec `json:"storage,omitempty"`
	RetentionPolicy RetentionPolicy `json:"retentionPolicy,omitempty"`
    Instances *InstanceSelector `json:"instance,omitempty"`
    
    // If true, backs up all instance-specific configs
    IncludeInstanceConfig bool `json:"includeInstanceConfig,omitempty"`

	// Options for backup
	Options BackupOptions `json:"options,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// Defines how many backups we keep and how often
type RetentionPolicy struct {
	KeepLast int32 `json:"keepLast,omitempty"`
	KeepDaily int32 `json:"keepDaily,omitempty"`
	KeepWeekly int32 `json:"keepWeekly,omitempty"`
	KeepMonthly int32 `json:"keepMonthly,omitempty"`
	DeleteOnClusterDeletion bool `json:"deleteOnClusterDeletion,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// Options for database backup parameters
type BackupOptions struct {
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=9
	Compression int32 `json:"compression,omitempty"`
	Encryption EncryptionSpec `json:"encryption,omitempty"`
	ParallelJobs int32 `json:"parallelJobs,omitempty"`
	Timeout string `json:"timeout,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// Used to see if we will encrypt with backup
type EncryptionSpec struct {
	Enabled bool `json:"enabled,omitempty"`
	SecretRef *SecretReference `json:"secretRef,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
type BackupSpec struct {
	Enabled bool `json:"enabled,omitempty"`
	Schedule string `json:"schedule,omitempty"`
	RetentionPolicy string `json:"retentionPolicy,omitempty"`
	Storage BackupStorageSpec `json:"storage"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
type BackupStorageSpec struct {
	// Type of backup storage (s3, gcs, azure, local)
	Type string `json:"type"`
	Config map[string]string `json:"config"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// Defines the status of our backup in a given period
type PostgresBackupStatus struct {
	Phase string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
	StartTime *metav1.Time `json:"startTime,omitempty"`
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	Size int64 `json:"size,omitempty"`
	Location string `json:"location,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	WALStart string `json:"walStart,omitempty"`
	WALEnd string `json:"walEnd,omitempty"`
	JobName string `json:"jobName,omitempty"`
	TargetInstance string `json:"targetInstance"`
	Database DatabaseStatus `json:"database"` 
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
type DatabaseStatus struct {
    Name       string            `json:"name"`
	// source is "cluster" or "instance"
    Source     string            `json:"source"`
    Config map[string]string `json:"config,omitempty"`
    Exists     bool              `json:"exists"`
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
// Standard backup schema
type PostgresBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgresBackupSpec   `json:"spec,omitempty"`
	Status PostgresBackupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// Provides a list of backups in the event of backup corruptions or errors
type PostgresBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgresBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgresBackup{}, &PostgresBackupList{})
}
