package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +groupName=database.example.com

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// PostgresClusterSpec defines the desired state of PostgresCluster
type PostgresClusterSpec struct {
	// Replicas is the number of PostgreSQL instances in the cluster
	Replicas int32 `json:"replicas,omitempty"`

	// PostgresVersion specifies the PostgreSQL version to use
	PostgresVersion string `json:"postgresVersion,omitempty"`

	// Database configuration
	Database DatabaseSpec `json:"database,omitempty"`

	// Storage configuration
	Storage StorageSpec `json:"storage,omitempty"`

	// Resources specifies the resource requirements
	Resources ResourceRequirements `json:"resources,omitempty"`

	// HighAvailability configuration
	HighAvailability HASpec `json:"highAvailability,omitempty"`

	// Backup configuration
	Backup BackupSpec `json:"backup,omitempty"`

	// Monitoring configuration
	Monitoring MonitoringSpec `json:"monitoring,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// DatabaseSpec defines database initialization
type DatabaseSpec struct {
	// Name of the default database
	Name string `json:"name,omitempty"`

	// InitScript for database initialization
	InitScript string `json:"initScript,omitempty"`

	// Parameters for PostgreSQL configuration
	Parameters map[string]string `json:"parameters,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// StorageSpec defines the storage used by databases within a cluster
type StorageSpec struct {
    // VolumeName specifies the name of the PersistentVolumeClaim
    VolumeName string `json:"volumeName,omitempty"`

	// Size of the storage volume
	Size string `json:"size,omitempty"`

	// StorageClass to use for the volume
	StorageClass string `json:"storageClass,omitempty"`

	// AccessModes for the volume
	AccessModes []string `json:"accessModes,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// ResourceRequirements defines the resources required to run a cluster
type ResourceRequirements struct {
	// CPU resource requirements
	CPU string `json:"cpu,omitempty"`

	// Memory resource requirements
	Memory string `json:"memory,omitempty"`

	// Storage resource requirements
	Storage string `json:"storage,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
type HASpec struct {
	// Enabled indicates if HA is enabled
	Enabled bool `json:"enabled,omitempty"`

	// SynchronousReplication enables synchronous replication
	SynchronousReplication bool `json:"synchronousReplication,omitempty"`

	// FailoverTimeout in seconds
	FailoverTimeout int32 `json:"failoverTimeout,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
type BackupSpec struct {
	// Enabled indicates if backup is enabled
	Enabled bool `json:"enabled,omitempty"`

	// Schedule for automatic backups (cron format)
	Schedule string `json:"schedule,omitempty"`

	// RetentionPolicy for backups
	RetentionPolicy string `json:"retentionPolicy,omitempty"`

	// Storage configuration for backups
	Storage BackupStorageSpec `json:"storage"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
type BackupStorageSpec struct {
	// Type of backup storage (s3, gcs, azure, local)
	Type string `json:"type"`

	// Configuration for the backup storage
	Config map[string]string `json:"config"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
type MonitoringSpec struct {
	// Enabled indicates if monitoring is enabled
	Enabled bool `json:"enabled,omitempty"`

	// Prometheus scraping configuration
	Prometheus PrometheusSpec `json:"prometheus,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
type PrometheusSpec struct {
	// Enabled indicates if Prometheus monitoring is enabled
	Enabled bool `json:"enabled,omitempty"`

	// Port for metrics endpoint
	Port int32 `json:"port,omitempty"`

	// Path for metrics endpoint
	Path string `json:"path,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// PostgresClusterStatus defines the observed state of PostgresCluster
type PostgresClusterStatus struct {
	// Phase represents the current phase of the cluster
	Phase string `json:"phase,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// ReadyReplicas is the number of ready PostgreSQL instances
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// CurrentPrimary indicates which instance is the current primary
	CurrentPrimary string `json:"currentPrimary,omitempty"`

	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastBackup timestamp
	LastBackup *metav1.Time `json:"lastBackup,omitempty"`

	// DatabaseVersion is the actual PostgreSQL version running
	DatabaseVersion string `json:"databaseVersion,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.readyReplicas
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.readyReplicas`
//+kubebuilder:printcolumn:name="Primary",type=string,JSONPath=`.status.currentPrimary`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PostgresCluster is the Schema for the postgresclusters API
type PostgresCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgresClusterSpec   `json:"spec,omitempty"`
	Status PostgresClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PostgresClusterList contains a list of PostgresCluster
type PostgresClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgresCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgresCluster{}, &PostgresClusterList{})
}