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
)

// PostgreSQLDatabaseReconciler reconciles a PostgreSQLDatabase object.
type PostgreSQLDatabaseReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=db.zernico.de,resources=postgresqldatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.zernico.de,resources=postgresqldatabases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=db.zernico.de,resources=postgresqldatabases/finalizers,verbs=update
// +kubebuilder:rbac:groups=db.zernico.de,resources=postgresqls,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *PostgreSQLDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the PostgreSQLDatabase CR
	db := &dbv1alpha1.PostgreSQLDatabase{}
	if err := r.Get(ctx, req.NamespacedName, db); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !db.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, db)
	}

	// Resolve PostgreSQL reference and connect
	pgClient, err := connectToPostgres(ctx, r.Client, db.Namespace, db.Spec.PostgresRef)
	if err != nil {
		log.Error(err, "Failed to connect to PostgreSQL")
		patchCondition(ctx, r.Client, db, &db.Status.Conditions, db.Generation,
			metav1.ConditionFalse, dbv1alpha1.ReasonFailed,
			fmt.Sprintf("Failed to connect to PostgreSQL: %v", err))
		return retryResult(db.Spec.SQLTemplate), nil
	}
	defer func() { _ = pgClient.Close(ctx) }()

	// Create the database (idempotent)
	dbName := db.DatabaseName()
	if err := pgClient.CreateDatabase(ctx, dbName); err != nil {
		log.Error(err, "Failed to create database", "database", dbName)
		patchCondition(ctx, r.Client, db, &db.Status.Conditions, db.Generation,
			metav1.ConditionFalse, dbv1alpha1.ReasonFailed,
			fmt.Sprintf("Failed to create database: %v", err))
		return retryResult(db.Spec.SQLTemplate), nil
	}

	// Add finalizer AFTER successful SQL operation (mariadb-operator pattern).
	// This prevents the finalizer from blocking deletion if the resource
	// was never successfully created.
	if !controllerutil.ContainsFinalizer(db, dbv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(db, dbv1alpha1.FinalizerName)
		if err := r.Update(ctx, db); err != nil {
			return ctrl.Result{}, err
		}
	}

	log.Info("Database reconciled", "database", dbName)
	patchCondition(ctx, r.Client, db, &db.Status.Conditions, db.Generation,
		metav1.ConditionTrue, dbv1alpha1.ReasonSucceeded,
		fmt.Sprintf("Database %q exists", dbName))

	return requeueResult(db.Spec.SQLTemplate), nil
}

func (r *PostgreSQLDatabaseReconciler) handleDeletion(ctx context.Context, db *dbv1alpha1.PostgreSQLDatabase) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(db, dbv1alpha1.FinalizerName) {
		return ctrl.Result{}, nil
	}

	if db.Spec.CleanupPolicy == dbv1alpha1.CleanupPolicyDelete {
		pgClient, err := connectToPostgres(ctx, r.Client, db.Namespace, db.Spec.PostgresRef)
		if err != nil {
			// If the PostgreSQL CR is gone, just remove the finalizer gracefully.
			// This mirrors mariadb-operator: if MariaDB is deleted, don't block.
			if apierrors.IsNotFound(err) {
				log.Info("PostgreSQL CR not found during cleanup, removing finalizer")
			} else {
				log.Error(err, "Failed to connect to PostgreSQL for cleanup")
				return ctrl.Result{RequeueAfter: defaultRetryInterval}, nil
			}
		} else {
			defer func() { _ = pgClient.Close(ctx) }()
			dbName := db.DatabaseName()
			if err := pgClient.DropDatabase(ctx, dbName); err != nil {
				log.Error(err, "Failed to drop database", "database", dbName)
				return ctrl.Result{RequeueAfter: defaultRetryInterval}, nil
			}
			log.Info("Dropped database", "database", dbName)
		}
	}

	controllerutil.RemoveFinalizer(db, dbv1alpha1.FinalizerName)
	if err := r.Update(ctx, db); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PostgreSQLDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1alpha1.PostgreSQLDatabase{}).
		Named("postgresqldatabase").
		Complete(r)
}
