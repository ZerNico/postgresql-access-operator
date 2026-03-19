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

// PostgreSQLGrantSpec defines the desired state of PostgreSQLGrant.
type PostgreSQLGrantSpec struct {
	// Shared SQL template fields (requeueInterval, retryInterval, cleanupPolicy).
	SQLTemplate `json:",inline"`

	// postgresRef is a cross-namespace reference to the PostgreSQL instance.
	// +required
	PostgresRef PostgresRef `json:"postgresRef"`

	// privileges is the list of privileges to grant.
	// Example: ["ALL PRIVILEGES"], ["SELECT", "INSERT", "UPDATE", "DELETE"]
	// +required
	// +kubebuilder:validation:MinItems=1
	Privileges []string `json:"privileges"`

	// database is the database to grant privileges on.
	// +required
	Database string `json:"database"`

	// schema is the schema to grant privileges on.
	// PostgreSQL requires schema-level grants for table access.
	// +optional
	// +kubebuilder:default="public"
	Schema string `json:"schema,omitempty"`

	// role is the role to grant privileges to.
	// +required
	Role string `json:"role"`
}

// PostgreSQLGrantStatus defines the observed state of PostgreSQLGrant.
type PostgreSQLGrantStatus struct {
	// conditions represent the current state of the PostgreSQLGrant resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Database",type=string,JSONPath=`.spec.database`
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.role`
// +kubebuilder:printcolumn:name="PostgreSQL",type=string,JSONPath=`.spec.postgresRef.name`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PostgreSQLGrant grants privileges on a database/schema to a role on a referenced PostgreSQL instance.
type PostgreSQLGrant struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec PostgreSQLGrantSpec `json:"spec"`

	// +optional
	Status PostgreSQLGrantStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PostgreSQLGrantList contains a list of PostgreSQLGrant
type PostgreSQLGrantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []PostgreSQLGrant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgreSQLGrant{}, &PostgreSQLGrantList{})
}
