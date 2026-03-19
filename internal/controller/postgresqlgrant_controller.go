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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbv1alpha1 "github.com/zernico/postgresql-access-operator/api/v1alpha1"
	pgsql "github.com/zernico/postgresql-access-operator/internal/sql"
)

// PostgreSQLGrantReconciler reconciles a PostgreSQLGrant object.
type PostgreSQLGrantReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=db.zernico.de,resources=postgresqlgrants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.zernico.de,resources=postgresqlgrants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=db.zernico.de,resources=postgresqlgrants/finalizers,verbs=update

func (r *PostgreSQLGrantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the PostgreSQLGrant CR
	grant := &dbv1alpha1.PostgreSQLGrant{}
	if err := r.Get(ctx, req.NamespacedName, grant); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !grant.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, grant)
	}

	// Resolve PostgreSQL reference and connect to the TARGET database
	// (schema-level grants must be run against the specific database)
	pgClient, err := connectToDatabase(ctx, r.Client, grant.Namespace, grant.Spec.PostgresRef, grant.Spec.Database)
	if err != nil {
		log.Error(err, "Failed to connect to PostgreSQL")
		patchCondition(ctx, r.Client, grant, &grant.Status.Conditions, grant.Generation,
			metav1.ConditionFalse, dbv1alpha1.ReasonFailed,
			fmt.Sprintf("Failed to connect to PostgreSQL: %v", err))
		return retryResult(grant.Spec.SQLTemplate), nil
	}
	defer func() { _ = pgClient.Close(ctx) }()

	// Grant privileges (idempotent)
	opts := pgsql.GrantOpts{
		Privileges: grant.Spec.Privileges,
		Database:   grant.Spec.Database,
		Schema:     grant.Spec.Schema,
		Role:       grant.Spec.Role,
	}
	if err := pgClient.GrantPrivileges(ctx, opts); err != nil {
		log.Error(err, "Failed to grant privileges")
		patchCondition(ctx, r.Client, grant, &grant.Status.Conditions, grant.Generation,
			metav1.ConditionFalse, dbv1alpha1.ReasonFailed,
			fmt.Sprintf("Failed to grant privileges: %v", err))
		return retryResult(grant.Spec.SQLTemplate), nil
	}

	// Add finalizer AFTER successful SQL operation
	if !controllerutil.ContainsFinalizer(grant, dbv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(grant, dbv1alpha1.FinalizerName)
		if err := r.Update(ctx, grant); err != nil {
			return ctrl.Result{}, err
		}
	}

	log.Info("Grant reconciled", "database", grant.Spec.Database, "role", grant.Spec.Role)
	patchCondition(ctx, r.Client, grant, &grant.Status.Conditions, grant.Generation,
		metav1.ConditionTrue, dbv1alpha1.ReasonSucceeded,
		fmt.Sprintf("Privileges granted on %q to %q", grant.Spec.Database, grant.Spec.Role))

	return requeueResult(grant.Spec.SQLTemplate), nil
}

func (r *PostgreSQLGrantReconciler) handleDeletion(ctx context.Context, grant *dbv1alpha1.PostgreSQLGrant) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(grant, dbv1alpha1.FinalizerName) {
		return ctrl.Result{}, nil
	}

	if grant.Spec.CleanupPolicy == dbv1alpha1.CleanupPolicyDelete {
		pgClient, err := connectToDatabase(ctx, r.Client, grant.Namespace, grant.Spec.PostgresRef, grant.Spec.Database)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("PostgreSQL CR not found during cleanup, removing finalizer")
			} else {
				log.Error(err, "Failed to connect to PostgreSQL for cleanup")
				return ctrl.Result{RequeueAfter: defaultRetryInterval}, nil
			}
		} else {
			defer func() { _ = pgClient.Close(ctx) }()
			opts := pgsql.GrantOpts{
				Privileges: grant.Spec.Privileges,
				Database:   grant.Spec.Database,
				Schema:     grant.Spec.Schema,
				Role:       grant.Spec.Role,
			}
			if err := pgClient.RevokePrivileges(ctx, opts); err != nil {
				log.Error(err, "Failed to revoke privileges")
				return ctrl.Result{RequeueAfter: defaultRetryInterval}, nil
			}
			log.Info("Revoked privileges", "database", grant.Spec.Database, "role", grant.Spec.Role)
		}
	}

	controllerutil.RemoveFinalizer(grant, dbv1alpha1.FinalizerName)
	if err := r.Update(ctx, grant); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PostgreSQLGrantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1alpha1.PostgreSQLGrant{}).
		Named("postgresqlgrant").
		Complete(r)
}
