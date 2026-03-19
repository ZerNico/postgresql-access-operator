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

package controller

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbv1alpha1 "github.com/zernico/postgresql-access-operator/api/v1alpha1"
	pgsql "github.com/zernico/postgresql-access-operator/internal/sql"
)

const (
	// defaultRequeueInterval is the default requeue interval for drift detection.
	defaultRequeueInterval = 30 * time.Second

	// maxRequeueJitter is the maximum random jitter added to the requeue interval
	// to prevent thundering herd when many resources requeue at the same time.
	maxRequeueJitter = 5 * time.Second

	// defaultRetryInterval is the default retry interval on errors.
	defaultRetryInterval = 5 * time.Second
)

// connectToPostgres resolves a PostgresRef, reads superuser credentials, and connects.
// This is the shared helper used by all SQL resource controllers (Database, User, Grant).
func connectToPostgres(ctx context.Context, c client.Client, namespace string, ref dbv1alpha1.PostgresRef) (*pgsql.Client, error) {
	pgNS := ref.Namespace
	if pgNS == "" {
		pgNS = namespace
	}

	pg := &dbv1alpha1.PostgreSQL{}
	if err := c.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: pgNS}, pg); err != nil {
		return nil, fmt.Errorf("getting PostgreSQL CR %s/%s: %w", pgNS, ref.Name, err)
	}

	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Name:      pg.Spec.SuperuserSecretKeyRef.Name,
		Namespace: pgNS,
	}, secret); err != nil {
		return nil, fmt.Errorf("getting superuser secret: %w", err)
	}

	password, ok := secret.Data[pg.Spec.SuperuserSecretKeyRef.Key]
	if !ok {
		return nil, fmt.Errorf("key %q not found in secret %s/%s",
			pg.Spec.SuperuserSecretKeyRef.Key, pgNS, pg.Spec.SuperuserSecretKeyRef.Name)
	}

	return pgsql.Connect(ctx, pgsql.ConnectConfig{
		Host:     pg.Spec.Host,
		Port:     pg.Spec.Port,
		Database: pg.Spec.Database,
		Username: pg.Spec.SuperuserUsername,
		Password: string(password),
	})
}

// connectToDatabase connects to a specific database on the referenced PostgreSQL instance.
// This is needed for Grant operations because schema-level GRANT statements must be
// executed while connected to the target database.
func connectToDatabase(ctx context.Context, c client.Client, namespace string, ref dbv1alpha1.PostgresRef, database string) (*pgsql.Client, error) {
	pgNS := ref.Namespace
	if pgNS == "" {
		pgNS = namespace
	}

	pg := &dbv1alpha1.PostgreSQL{}
	if err := c.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: pgNS}, pg); err != nil {
		return nil, fmt.Errorf("getting PostgreSQL CR %s/%s: %w", pgNS, ref.Name, err)
	}

	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Name:      pg.Spec.SuperuserSecretKeyRef.Name,
		Namespace: pgNS,
	}, secret); err != nil {
		return nil, fmt.Errorf("getting superuser secret: %w", err)
	}

	password, ok := secret.Data[pg.Spec.SuperuserSecretKeyRef.Key]
	if !ok {
		return nil, fmt.Errorf("key %q not found in secret %s/%s",
			pg.Spec.SuperuserSecretKeyRef.Key, pgNS, pg.Spec.SuperuserSecretKeyRef.Name)
	}

	return pgsql.Connect(ctx, pgsql.ConnectConfig{
		Host:     pg.Spec.Host,
		Port:     pg.Spec.Port,
		Database: database,
		Username: pg.Spec.SuperuserUsername,
		Password: string(password),
	})
}

// patchCondition updates a status condition using Patch (not Update) to avoid conflicts.
// This mirrors the mariadb-operator pattern of using client.MergeFrom for optimistic patching.
func patchCondition(ctx context.Context, c client.Client, obj client.Object, conditions *[]metav1.Condition,
	generation int64, status metav1.ConditionStatus, reason, message string) {
	patch := client.MergeFrom(obj.DeepCopyObject().(client.Object))
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               dbv1alpha1.ConditionTypeReady,
		Status:             status,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
	if err := c.Status().Patch(ctx, obj, patch); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to patch status condition")
	}
}

// requeueResult returns a requeue result using the per-resource interval (if set)
// or the global default, with random jitter to prevent thundering herd.
func requeueResult(tmpl dbv1alpha1.SQLTemplate) ctrl.Result {
	interval := defaultRequeueInterval
	if tmpl.RequeueInterval != nil {
		interval = tmpl.RequeueInterval.Duration
	}
	return ctrl.Result{RequeueAfter: addJitter(interval)}
}

// retryResult returns a retry result using the per-resource interval (if set)
// or the global default.
func retryResult(tmpl dbv1alpha1.SQLTemplate) ctrl.Result {
	interval := defaultRetryInterval
	if tmpl.RetryInterval != nil {
		interval = tmpl.RetryInterval.Duration
	}
	return ctrl.Result{RequeueAfter: addJitter(interval)}
}

// addJitter adds random jitter (up to maxRequeueJitter) to prevent thundering herd.
func addJitter(d time.Duration) time.Duration {
	return d + time.Duration(rand.Int64N(int64(maxRequeueJitter)))
}

// getSecretValue reads a specific key from a Secret.
func getSecretValue(ctx context.Context, c client.Client, namespace string, ref dbv1alpha1.SecretKeyRef) (string, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, secret); err != nil {
		return "", fmt.Errorf("getting secret %s/%s: %w", namespace, ref.Name, err)
	}
	value, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %s/%s", ref.Key, namespace, ref.Name)
	}
	return string(value), nil
}
