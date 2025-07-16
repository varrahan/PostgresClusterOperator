package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +groupName=database.example.com

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// Specifcations for restoring
type PostgresRestoreSpec struct {
	BackupRef BackupReference `json:"backupRef"`
	TargetCluster ClusterReference `json:"targetCluster"`
	Options RestoreOptions `json:"options,omitempty"`
	Instances *InstanceSelector `json:"instances,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// Options when restoring from backup
type RestoreOptions struct {
	DropExisting bool `json:"dropExisting,omitempty"`
	DataOnly bool `json:"dataOnly,omitempty"`
	SchemaOnly bool `json:"schemaOnly,omitempty"`
	Timeout string `json:"timeout,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	// +optional
	ParallelRestores int32 `json:"parallelRestores,omitempty"`
	// +optional
	DatabaseFilter *DatabaseFilter `json:"databaseFilter,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// Selector for which instances are being restored. Selected by name
type InstanceSelector struct {
	Names []string `json:"names,omitempty"`
	// +optional
	Role string `json:"role,omitempty"`
	// +optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
type DatabaseFilter struct {
	Include []string `json:"include,omitempty"`
	// +optional
	Exclude []string `json:"exclude,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// Restoration status
type PostgresRestoreStatus struct {
	Phase string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
	JobName string `json:"jobName,omitempty"`
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	StartTime *metav1.Time `json:"startTime,omitempty"`
	WALStart string `json:"walStart,omitempty"`
	WALEnd string `json:"walEnd,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	InstanceStatuses []InstanceRestoreStatus `json:"instanceStatuses,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// Instance-specific status tracking
type InstanceRestoreStatus struct {
	Name string `json:"name"`
	Phase string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
	JobName string `json:"jobName,omitempty"`
	StartTime *metav1.Time `json:"startTime,omitempty"`
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.message`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type PostgresRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgresRestoreSpec   `json:"spec,omitempty"`
	Status PostgresRestoreStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
type PostgresRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgresRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgresRestore{}, &PostgresRestoreList{})
}