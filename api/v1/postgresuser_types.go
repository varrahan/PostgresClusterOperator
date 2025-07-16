package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +groupName=database.example.com

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// PostgresUserSpec defines the desired state of PostgresUser
type PostgresUserSpec struct {
	// +kubebuilder:validation:Required
	ClusterRef *ClusterReference `json:"clusterRef"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-zA-Z_][a-zA-Z0-9_]*$`
	Username string `json:"username"`
	Password *PasswordSpec `json:"password"`
	// +kubebuilder:validation:MaxItems=20
	// +optional
	Privileges []string `json:"privileges,omitempty"`
	// +kubebuilder:validation:Minimum=-1
	// +kubebuilder:default=-1
	ConnectionLimit int32 `json:"connectionLimit,omitempty"`
	InstanceSelector *InstanceSelector `json:"instanceSelector,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// Reference to associated cluster
type ClusterReference struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// Specifications to handle passwords
type PasswordSpec struct {
	SecretRef *SecretReference `json:"secretRef,omitempty"`
	// Generate indicates if a password should be auto-generated
	// +kubebuilder:default=false
	// +optional
	Generate bool `json:"generate,omitempty"`
	// +kubebuilder:validation:Minimum=8
	// +kubebuilder:validation:Maximum=128
	// +kubebuilder:default=16
	// +optional
	Length int32 `json:"length,omitempty"`
	// +optional
	RotationPolicy *PasswordRotationPolicy `json:"rotationPolicy,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// Kubernetes secrets reference
type SecretReference struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// +kubebuilder:default="password"
	// +optional
	Key string `json:"key,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// How long a password should be rotated for
type PasswordRotationPolicy struct {
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// time in hours
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=2048
	// +optional
	Interval int32 `json:"interval,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// Status of user
type PostgresUserStatus struct {
	// +kubebuilder:validation:Enum=Pending;Ready;Failed
	Phase string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	LastPasswordChange *metav1.Time `json:"lastPasswordChange,omitempty"`
	DatabasesGranted []string `json:"databasesGranted,omitempty"`
	// +optional
	InstanceStatuses []UserInstanceStatus `json:"instanceStatuses,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// Status of user per instance
type UserInstanceStatus struct {
	Name string `json:"name"`
	Ready bool `json:"ready"`
	// +optional
	Message string `json:"message,omitempty"`
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Username",type=string,JSONPath=`.spec.username`
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterRef.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=pguser
type PostgresUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgresUserSpec   `json:"spec,omitempty"`
	Status PostgresUserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type PostgresUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgresUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgresUser{}, &PostgresUserList{})
}