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

// PostgreSQLDatabaseSpec defines the desired state of PostgreSQLDatabase.
type PostgreSQLDatabaseSpec struct {
	// Shared SQL template fields (requeueInterval, retryInterval, cleanupPolicy).
	SQLTemplate `json:",inline"`

	// postgresRef is a cross-namespace reference to the PostgreSQL instance.
	// +required
	PostgresRef PostgresRef `json:"postgresRef"`

	// name is the database name to create in PostgreSQL.
	// If empty, defaults to metadata.name.
	// +optional
	Name string `json:"name,omitempty"`
}

// PostgreSQLDatabaseStatus defines the observed state of PostgreSQLDatabase.
type PostgreSQLDatabaseStatus struct {
	// conditions represent the current state of the PostgreSQLDatabase resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// DatabaseName returns the database name to use, falling back to metadata.name.
func (d *PostgreSQLDatabase) DatabaseName() string {
	if d.Spec.Name != "" {
		return d.Spec.Name
	}
	return d.Name
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Database",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="PostgreSQL",type=string,JSONPath=`.spec.postgresRef.name`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PostgreSQLDatabase creates a database on a referenced PostgreSQL instance.
type PostgreSQLDatabase struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec PostgreSQLDatabaseSpec `json:"spec"`

	// +optional
	Status PostgreSQLDatabaseStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PostgreSQLDatabaseList contains a list of PostgreSQLDatabase
type PostgreSQLDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []PostgreSQLDatabase `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgreSQLDatabase{}, &PostgreSQLDatabaseList{})
}
