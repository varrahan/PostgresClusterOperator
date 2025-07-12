package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +groupName=database.example.com

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// PostgresRestoreSpec defines the desired state of PostgresRestore
type PostgresRestoreSpec struct {
	// BackupRef references the backup to restore from
	BackupRef BackupReference `json:"backupRef"`

	// TargetCluster references the cluster to restore to
	TargetCluster ClusterReference `json:"targetCluster"`

	// Options for the restore operation
	Options RestoreOptions `json:"options,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// RestoreOptions defines options for the restore operation
type RestoreOptions struct {
	// DropExisting drops existing databases before restore
	DropExisting bool `json:"dropExisting,omitempty"`

	// DataOnly restores only data, not schema
	DataOnly bool `json:"dataOnly,omitempty"`

	// SchemaOnly restores only schema, not data
	SchemaOnly bool `json:"schemaOnly,omitempty"`

	// Timeout for the restore operation
	Timeout string `json:"timeout,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
type PostgresRestoreStatus struct {
	// Phase represents the current phase of the restore
	Phase string `json:"phase,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// JobName is the name of the restore job
	JobName string `json:"jobName,omitempty"`

	// CompletionTime when the restore completed
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// StartTime when the restore started
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}


//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.message`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// PostgresRestore is the Schema for the postgresrestore API
type PostgresRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgresRestoreSpec   `json:"spec,omitempty"`
	Status PostgresRestoreStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
// PostgresRestoreList contains a list of PostgresRestore
type PostgresRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgresRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgresRestore{}, &PostgresRestoreList{})
}