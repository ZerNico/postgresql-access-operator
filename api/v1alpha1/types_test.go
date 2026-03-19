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
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDatabaseName(t *testing.T) {
	tests := []struct {
		name         string
		metadataName string
		specName     string
		want         string
	}{
		{
			name:         "spec.name takes precedence",
			metadataName: "my-app",
			specName:     "myapp_staging",
			want:         "myapp_staging",
		},
		{
			name:         "falls back to metadata.name",
			metadataName: "my-app",
			specName:     "",
			want:         "my-app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &PostgreSQLDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: tt.metadataName},
				Spec:       PostgreSQLDatabaseSpec{Name: tt.specName},
			}
			if got := db.DatabaseName(); got != tt.want {
				t.Errorf("DatabaseName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRoleName(t *testing.T) {
	tests := []struct {
		name         string
		metadataName string
		specName     string
		want         string
	}{
		{
			name:         "spec.name takes precedence",
			metadataName: "my-app",
			specName:     "myapp_staging",
			want:         "myapp_staging",
		},
		{
			name:         "falls back to metadata.name",
			metadataName: "my-app",
			specName:     "",
			want:         "my-app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &PostgreSQLUser{
				ObjectMeta: metav1.ObjectMeta{Name: tt.metadataName},
				Spec:       PostgreSQLUserSpec{Name: tt.specName},
			}
			if got := user.RoleName(); got != tt.want {
				t.Errorf("RoleName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSQLTemplateDefaults(t *testing.T) {
	t.Run("zero-value SQLTemplate has nil intervals and empty cleanupPolicy", func(t *testing.T) {
		tmpl := SQLTemplate{}
		if tmpl.RequeueInterval != nil {
			t.Error("expected RequeueInterval to be nil")
		}
		if tmpl.RetryInterval != nil {
			t.Error("expected RetryInterval to be nil")
		}
		if tmpl.CleanupPolicy != "" {
			t.Errorf("expected empty CleanupPolicy, got %q", tmpl.CleanupPolicy)
		}
	})

	t.Run("SQLTemplate with custom intervals", func(t *testing.T) {
		tmpl := SQLTemplate{
			RequeueInterval: &metav1.Duration{Duration: 60 * time.Second},
			RetryInterval:   &metav1.Duration{Duration: 10 * time.Second},
			CleanupPolicy:   CleanupPolicyDelete,
		}
		if tmpl.RequeueInterval.Duration != 60*time.Second {
			t.Errorf("RequeueInterval = %v, want 60s", tmpl.RequeueInterval.Duration)
		}
		if tmpl.RetryInterval.Duration != 10*time.Second {
			t.Errorf("RetryInterval = %v, want 10s", tmpl.RetryInterval.Duration)
		}
		if tmpl.CleanupPolicy != CleanupPolicyDelete {
			t.Errorf("CleanupPolicy = %q, want %q", tmpl.CleanupPolicy, CleanupPolicyDelete)
		}
	})
}

func TestCleanupPolicyConstants(t *testing.T) {
	if CleanupPolicySkip != "Skip" {
		t.Errorf("CleanupPolicySkip = %q, want %q", CleanupPolicySkip, "Skip")
	}
	if CleanupPolicyDelete != "Delete" {
		t.Errorf("CleanupPolicyDelete = %q, want %q", CleanupPolicyDelete, "Delete")
	}
}

func TestPostgresRefNamespaceFallback(t *testing.T) {
	t.Run("explicit namespace", func(t *testing.T) {
		ref := PostgresRef{Name: "pg-prod", Namespace: "postgresql"}
		if ref.Namespace != "postgresql" {
			t.Errorf("Namespace = %q, want %q", ref.Namespace, "postgresql")
		}
	})

	t.Run("empty namespace should be empty (controller handles fallback)", func(t *testing.T) {
		ref := PostgresRef{Name: "pg-prod"}
		if ref.Namespace != "" {
			t.Errorf("Namespace = %q, want empty", ref.Namespace)
		}
	})
}
