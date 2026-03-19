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

// PostgreSQLSpec defines connection details for an existing PostgreSQL instance.
type PostgreSQLSpec struct {
	// host is the hostname or IP address of the PostgreSQL server.
	// +required
	Host string `json:"host"`

	// port is the TCP port of the PostgreSQL server.
	// +optional
	// +kubebuilder:default=5432
	Port int32 `json:"port,omitempty"`

	// database is the admin database to connect to.
	// +optional
	// +kubebuilder:default="postgres"
	Database string `json:"database,omitempty"`

	// superuserUsername is the superuser username for connecting to PostgreSQL.
	// +optional
	// +kubebuilder:default="postgres"
	SuperuserUsername string `json:"superuserUsername,omitempty"`

	// superuserSecretKeyRef references the Secret containing the superuser password.
	// +required
	SuperuserSecretKeyRef SecretKeyRef `json:"superuserSecretKeyRef"`
}

// PostgreSQLStatus defines the observed state of PostgreSQL.
type PostgreSQLStatus struct {
	// conditions represent the current state of the PostgreSQL resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Host",type=string,JSONPath=`.spec.host`
// +kubebuilder:printcolumn:name="Port",type=integer,JSONPath=`.spec.port`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PostgreSQL represents a reference to an existing PostgreSQL instance.
// It stores connection details used by Database, User, and Grant CRDs.
type PostgreSQL struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec PostgreSQLSpec `json:"spec"`

	// +optional
	Status PostgreSQLStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PostgreSQLList contains a list of PostgreSQL
type PostgreSQLList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []PostgreSQL `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgreSQL{}, &PostgreSQLList{})
}
