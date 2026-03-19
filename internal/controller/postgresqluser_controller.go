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

// PostgreSQLUserReconciler reconciles a PostgreSQLUser object.
type PostgreSQLUserReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=db.zernico.de,resources=postgresqlusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.zernico.de,resources=postgresqlusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=db.zernico.de,resources=postgresqlusers/finalizers,verbs=update

func (r *PostgreSQLUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the PostgreSQLUser CR
	user := &dbv1alpha1.PostgreSQLUser{}
	if err := r.Get(ctx, req.NamespacedName, user); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !user.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, user)
	}

	// Resolve PostgreSQL reference and connect
	pgClient, err := connectToPostgres(ctx, r.Client, user.Namespace, user.Spec.PostgresRef)
	if err != nil {
		log.Error(err, "Failed to connect to PostgreSQL")
		patchCondition(ctx, r.Client, user, &user.Status.Conditions, user.Generation,
			metav1.ConditionFalse, dbv1alpha1.ReasonFailed,
			fmt.Sprintf("Failed to connect to PostgreSQL: %v", err))
		return retryResult(user.Spec.SQLTemplate), nil
	}
	defer func() { _ = pgClient.Close(ctx) }()

	// Read the user password from the referenced Secret (same namespace as the User CR)
	password, err := getSecretValue(ctx, r.Client, user.Namespace, user.Spec.PasswordSecretKeyRef)
	if err != nil {
		log.Error(err, "Failed to read password secret")
		patchCondition(ctx, r.Client, user, &user.Status.Conditions, user.Generation,
			metav1.ConditionFalse, dbv1alpha1.ReasonFailed,
			fmt.Sprintf("Failed to read password secret: %v", err))
		return retryResult(user.Spec.SQLTemplate), nil
	}

	// Create or update the role (idempotent)
	roleName := user.RoleName()
	if err := pgClient.CreateOrUpdateRole(ctx, roleName, password); err != nil {
		log.Error(err, "Failed to create/update role", "role", roleName)
		patchCondition(ctx, r.Client, user, &user.Status.Conditions, user.Generation,
			metav1.ConditionFalse, dbv1alpha1.ReasonFailed,
			fmt.Sprintf("Failed to create/update role: %v", err))
		return retryResult(user.Spec.SQLTemplate), nil
	}

	// Add finalizer AFTER successful SQL operation
	if !controllerutil.ContainsFinalizer(user, dbv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(user, dbv1alpha1.FinalizerName)
		if err := r.Update(ctx, user); err != nil {
			return ctrl.Result{}, err
		}
	}

	log.Info("Role reconciled", "role", roleName)
	patchCondition(ctx, r.Client, user, &user.Status.Conditions, user.Generation,
		metav1.ConditionTrue, dbv1alpha1.ReasonSucceeded,
		fmt.Sprintf("Role %q exists", roleName))

	return requeueResult(user.Spec.SQLTemplate), nil
}

func (r *PostgreSQLUserReconciler) handleDeletion(ctx context.Context, user *dbv1alpha1.PostgreSQLUser) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(user, dbv1alpha1.FinalizerName) {
		return ctrl.Result{}, nil
	}

	if user.Spec.CleanupPolicy == dbv1alpha1.CleanupPolicyDelete {
		pgClient, err := connectToPostgres(ctx, r.Client, user.Namespace, user.Spec.PostgresRef)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("PostgreSQL CR not found during cleanup, removing finalizer")
			} else {
				log.Error(err, "Failed to connect to PostgreSQL for cleanup")
				return ctrl.Result{RequeueAfter: defaultRetryInterval}, nil
			}
		} else {
			defer func() { _ = pgClient.Close(ctx) }()
			roleName := user.RoleName()
			if err := pgClient.DropRole(ctx, roleName); err != nil {
				log.Error(err, "Failed to drop role", "role", roleName)
				return ctrl.Result{RequeueAfter: defaultRetryInterval}, nil
			}
			log.Info("Dropped role", "role", roleName)
		}
	}

	controllerutil.RemoveFinalizer(user, dbv1alpha1.FinalizerName)
	if err := r.Update(ctx, user); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PostgreSQLUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1alpha1.PostgreSQLUser{}).
		Named("postgresqluser").
		Complete(r)
}
