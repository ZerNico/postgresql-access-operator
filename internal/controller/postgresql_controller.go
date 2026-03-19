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
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbv1alpha1 "github.com/zernico/postgresql-access-operator/api/v1alpha1"
	pgsql "github.com/zernico/postgresql-access-operator/internal/sql"
)

// PostgreSQLReconciler reconciles a PostgreSQL object.
// It validates connectivity to the referenced PostgreSQL instance.
type PostgreSQLReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=db.zernico.de,resources=postgresqls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.zernico.de,resources=postgresqls/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=db.zernico.de,resources=postgresqls/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *PostgreSQLReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the PostgreSQL CR
	pg := &dbv1alpha1.PostgreSQL{}
	if err := r.Get(ctx, req.NamespacedName, pg); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Read the superuser password from the referenced Secret
	password, err := getSecretValue(ctx, r.Client, pg.Namespace, pg.Spec.SuperuserSecretKeyRef)
	if err != nil {
		log.Error(err, "Failed to read superuser secret")
		patchCondition(ctx, r.Client, pg, &pg.Status.Conditions, pg.Generation,
			metav1.ConditionFalse, dbv1alpha1.ReasonFailed,
			fmt.Sprintf("Failed to read superuser secret: %v", err))
		return ctrl.Result{RequeueAfter: defaultRequeueInterval}, nil
	}

	// Try to connect and ping
	pgClient, err := pgsql.Connect(ctx, pgsql.ConnectConfig{
		Host:     pg.Spec.Host,
		Port:     pg.Spec.Port,
		Database: pg.Spec.Database,
		Username: pg.Spec.SuperuserUsername,
		Password: password,
	})
	if err != nil {
		log.Error(err, "Failed to connect to PostgreSQL")
		patchCondition(ctx, r.Client, pg, &pg.Status.Conditions, pg.Generation,
			metav1.ConditionFalse, dbv1alpha1.ReasonFailed,
			fmt.Sprintf("Failed to connect: %v", err))
		return ctrl.Result{RequeueAfter: defaultRequeueInterval}, nil
	}
	defer func() { _ = pgClient.Close(ctx) }()

	if err := pgClient.Ping(ctx); err != nil {
		log.Error(err, "Failed to ping PostgreSQL")
		patchCondition(ctx, r.Client, pg, &pg.Status.Conditions, pg.Generation,
			metav1.ConditionFalse, dbv1alpha1.ReasonFailed,
			fmt.Sprintf("Ping failed: %v", err))
		return ctrl.Result{RequeueAfter: defaultRequeueInterval}, nil
	}

	// Connection successful
	log.Info("Successfully connected to PostgreSQL", "host", pg.Spec.Host, "port", pg.Spec.Port)
	patchCondition(ctx, r.Client, pg, &pg.Status.Conditions, pg.Generation,
		metav1.ConditionTrue, dbv1alpha1.ReasonSucceeded, "Connected to PostgreSQL")

	return ctrl.Result{RequeueAfter: addJitter(defaultRequeueInterval)}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PostgreSQLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbv1alpha1.PostgreSQL{}).
		Named("postgresql").
		Complete(r)
}
