package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PostgresUserSpec defines the desired state of PostgresUser
type PostgresUserSpec struct {
	// ClusterRef references the PostgresCluster this user belongs to
	ClusterRef ClusterReference `json:"clusterRef"`

	// Username for the PostgreSQL user
	Username string `json:"username"`

	// Password configuration
	Password PasswordSpec `json:"password,omitempty"`

	// Privileges for the user
	Privileges []string `json:"privileges,omitempty"`

	// Databases the user should have access to
	Databases []string `json:"databases,omitempty"`

	// ConnectionLimit for the user
	ConnectionLimit int32 `json:"connectionLimit,omitempty"`

	// ValidUntil timestamp for user expiration
	ValidUntil *metav1.Time `json:"validUntil,omitempty"`
}

type ClusterReference struct {
	// Name of the PostgresCluster
	Name string `json:"name"`

	// Namespace of the PostgresCluster
	Namespace string `json:"namespace,omitempty"`
}

type PasswordSpec struct {
	// SecretRef references a secret containing the password
	SecretRef *SecretReference `json:"secretRef,omitempty"`

	// Generate indicates if a password should be auto-generated
	Generate bool `json:"generate,omitempty"`

	// Length of the generated password
	Length int32 `json:"length,omitempty"`
}

type SecretReference struct {
	// Name of the secret
	Name string `json:"name"`

	// Key within the secret
	Key string `json:"key,omitempty"`
}

// PostgresUserStatus defines the observed state of PostgresUser
type PostgresUserStatus struct {
	// Phase represents the current phase of the user
	Phase string `json:"phase,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastPasswordChange timestamp
	LastPasswordChange *metav1.Time `json:"lastPasswordChange,omitempty"`

	// DatabasesGranted lists the databases the user has access to
	DatabasesGranted []string `json:"databasesGranted,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Username",type=string,JSONPath=`.spec.username`
//+kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterRef.name`
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PostgresUser is the Schema for the postgresusers API
type PostgresUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgresUserSpec   `json:"spec,omitempty"`
	Status PostgresUserStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PostgresUserList contains a list of PostgresUser
type PostgresUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgresUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgresUser{}, &PostgresUserList{})
}