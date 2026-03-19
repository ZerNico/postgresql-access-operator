/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PostgreSQLUserSpec defines the desired state of PostgreSQLUser.
type PostgreSQLUserSpec struct {
	// Shared SQL template fields (requeueInterval, retryInterval, cleanupPolicy).
	SQLTemplate `json:",inline"`

	// postgresRef is a cross-namespace reference to the PostgreSQL instance.
	// +required
	PostgresRef PostgresRef `json:"postgresRef"`

	// name is the role name to create in PostgreSQL.
	// If empty, defaults to metadata.name.
	// +optional
	Name string `json:"name,omitempty"`

	// passwordSecretKeyRef references the Secret containing the role password.
	// The Secret must exist in the same namespace as this PostgreSQLUser CR.
	// +required
	PasswordSecretKeyRef SecretKeyRef `json:"passwordSecretKeyRef"`
}

// PostgreSQLUserStatus defines the observed state of PostgreSQLUser.
type PostgreSQLUserStatus struct {
	// conditions represent the current state of the PostgreSQLUser resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// RoleName returns the role name to use, falling back to metadata.name.
func (u *PostgreSQLUser) RoleName() string {
	if u.Spec.Name != "" {
		return u.Spec.Name
	}
	return u.Name
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="PostgreSQL",type=string,JSONPath=`.spec.postgresRef.name`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PostgreSQLUser creates a role (with LOGIN) on a referenced PostgreSQL instance.
type PostgreSQLUser struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec PostgreSQLUserSpec `json:"spec"`

	// +optional
	Status PostgreSQLUserStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PostgreSQLUserList contains a list of PostgreSQLUser
type PostgreSQLUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []PostgreSQLUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgreSQLUser{}, &PostgreSQLUserList{})
}
