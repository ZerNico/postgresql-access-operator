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

// CleanupPolicy defines what happens when a CR is deleted.
// +kubebuilder:validation:Enum=Skip;Delete
type CleanupPolicy string

const (
	// CleanupPolicySkip means deleting the CR does NOT drop the resource in PostgreSQL.
	CleanupPolicySkip CleanupPolicy = "Skip"
	// CleanupPolicyDelete means deleting the CR WILL drop the resource in PostgreSQL.
	CleanupPolicyDelete CleanupPolicy = "Delete"
)

// SQLTemplate defines shared fields for all SQL resource CRDs (Database, User, Grant).
// Mirrors the mariadb-operator SQLTemplate pattern.
type SQLTemplate struct {
	// requeueInterval is the interval for periodic re-reconciliation (drift detection).
	// If not set, the global default (30s) is used.
	// +optional
	RequeueInterval *metav1.Duration `json:"requeueInterval,omitempty"`

	// retryInterval is the interval for retrying on errors.
	// If not set, the resource is requeued immediately on error.
	// +optional
	RetryInterval *metav1.Duration `json:"retryInterval,omitempty"`

	// cleanupPolicy defines what happens when this CR is deleted.
	// Skip (default): the resource is NOT dropped from PostgreSQL.
	// Delete: the resource IS dropped from PostgreSQL.
	// +optional
	// +kubebuilder:default="Skip"
	CleanupPolicy CleanupPolicy `json:"cleanupPolicy,omitempty"`
}

// PostgresRef is a cross-namespace reference to a PostgreSQL CR.
type PostgresRef struct {
	// name is the name of the PostgreSQL CR.
	// +required
	Name string `json:"name"`

	// namespace is the namespace of the PostgreSQL CR.
	// If empty, defaults to the namespace of the referencing resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// SecretKeyRef is a reference to a key within a Kubernetes Secret.
type SecretKeyRef struct {
	// name is the name of the Secret.
	// +required
	Name string `json:"name"`

	// key is the key within the Secret data.
	// +required
	Key string `json:"key"`
}

const (
	// ConditionTypeReady indicates whether the resource is ready.
	ConditionTypeReady = "Ready"

	// ReasonSucceeded indicates the operation completed successfully.
	ReasonSucceeded = "Succeeded"

	// ReasonFailed indicates the operation failed.
	ReasonFailed = "Failed"

	// ReasonConnecting indicates the operator is connecting to PostgreSQL.
	ReasonConnecting = "Connecting"

	// FinalizerName is the finalizer used by all SQL resource controllers.
	FinalizerName = "db.zernico.de/finalizer"
)
