package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// PostgresUserSpec defines the desired state of PostgresUser
type PostgresUserSpec struct {
	// ClusterRef references the PostgresCluster this user belongs to
	// +kubebuilder:validation:Required
	ClusterRef ClusterReference `json:"clusterRef"`

	// Username for the PostgreSQL user. Must be a valid PostgreSQL identifier.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-zA-Z_][a-zA-Z0-9_]*$`
	Username string `json:"username"`

	// Password configuration for the user
	// +optional
	Password PasswordSpec `json:"password,omitempty"`

	// Privileges defines the roles/privileges granted to the user
	// +kubebuilder:validation:MaxItems=20
	// +optional
	Privileges []string `json:"privileges,omitempty"`

	// Databases this user should have access to
	// +kubebuilder:validation:MaxItems=100
	// +optional
	Databases []string `json:"databases,omitempty"`

	// ConnectionLimit specifies maximum concurrent connections (default: no limit)
	// +kubebuilder:validation:Minimum=-1
	// +kubebuilder:default=-1
	// +optional
	ConnectionLimit int32 `json:"connectionLimit,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// ClusterReference identifies a PostgresCluster
type ClusterReference struct {
	// Name of the PostgresCluster
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace of the PostgresCluster (defaults to same namespace as PostgresUser)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// PasswordSpec defines how to handle user password
type PasswordSpec struct {
	// SecretRef references an existing secret containing the password
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`

	// Generate indicates if a password should be auto-generated
	// +kubebuilder:default=false
	// +optional
	Generate bool `json:"generate,omitempty"`

	// Length of the generated password (default: 16)
	// +kubebuilder:validation:Minimum=8
	// +kubebuilder:validation:Maximum=128
	// +kubebuilder:default=16
	// +optional
	Length int32 `json:"length,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// SecretReference identifies a Kubernetes Secret
type SecretReference struct {
	// Name of the secret
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key within the secret (default: "password")
	// +kubebuilder:default="password"
	// +optional
	Key string `json:"key,omitempty"`
}

// +k8s:deepcopy-gen=true
// +kubebuilder:object:generate=true
// PostgresUserStatus defines the observed state of PostgresUser
type PostgresUserStatus struct {
	// Phase represents the current phase of the user (Pending, Ready, Failed)
	// +kubebuilder:validation:Enum=Pending;Ready;Failed
	// +optional
	Phase string `json:"phase,omitempty"`

	// Message provides additional information about the current state
	// +optional
	Message string `json:"message,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastPasswordChange timestamp
	// +optional
	LastPasswordChange *metav1.Time `json:"lastPasswordChange,omitempty"`

	// DatabasesGranted lists the databases the user has access to
	// +optional
	DatabasesGranted []string `json:"databasesGranted,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Username",type=string,JSONPath=`.spec.username`
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterRef.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=pguser

// PostgresUser is the Schema for the postgresusers API
type PostgresUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgresUserSpec   `json:"spec,omitempty"`
	Status PostgresUserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PostgresUserList contains a list of PostgresUser
type PostgresUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgresUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgresUser{}, &PostgresUserList{})
}