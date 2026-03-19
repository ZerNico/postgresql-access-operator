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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dbv1alpha1 "github.com/zernico/postgresql-access-operator/api/v1alpha1"
	pgsql "github.com/zernico/postgresql-access-operator/internal/sql"
)

var _ = Describe("PostgreSQLGrant Controller", func() {
	reconciler := &PostgreSQLGrantReconciler{}
	var pgClient *pgsql.Client

	const (
		grantDBName   = "grant_test_db"
		grantRoleName = "grant_test_role"
	)

	BeforeEach(func() {
		reconciler = &PostgreSQLGrantReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
		var err error
		pgClient, err = pgsql.Connect(ctx, pgsql.ConnectConfig{
			Host: pgHost, Port: pgPort, Database: testPgDB,
			Username: testPgUser, Password: testPgPassword,
		})
		Expect(err).NotTo(HaveOccurred())

		// Setup prerequisites: database and role
		Expect(pgClient.CreateDatabase(ctx, grantDBName)).To(Succeed())
		Expect(pgClient.CreateOrUpdateRole(ctx, grantRoleName, "grantpass")).To(Succeed())
	})

	AfterEach(func() {
		_ = pgClient.DropRole(ctx, grantRoleName)
		_ = pgClient.DropDatabase(ctx, grantDBName)
		_ = pgClient.Close(ctx)
	})

	Context("Happy path: granting privileges", func() {
		const grantCRName = "test-grant-happy"

		AfterEach(func() {
			grant := &dbv1alpha1.PostgreSQLGrant{}
			key := types.NamespacedName{Name: grantCRName, Namespace: "default"}
			if err := k8sClient.Get(ctx, key, grant); err == nil {
				controllerutil.RemoveFinalizer(grant, dbv1alpha1.FinalizerName)
				_ = k8sClient.Update(ctx, grant)
				_ = k8sClient.Delete(ctx, grant)
			}
		})

		It("should grant privileges and set Ready=True", func() {
			grant := &dbv1alpha1.PostgreSQLGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      grantCRName,
					Namespace: "default",
				},
				Spec: dbv1alpha1.PostgreSQLGrantSpec{
					PostgresRef: dbv1alpha1.PostgresRef{
						Name:      "postgresql-staging",
						Namespace: "postgresql",
					},
					Privileges: []string{"ALL PRIVILEGES"},
					Database:   grantDBName,
					Schema:     "public",
					Role:       grantRoleName,
				},
			}
			Expect(k8sClient.Create(ctx, grant)).To(Succeed())

			key := types.NamespacedName{Name: grantCRName, Namespace: "default"}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Verify status
			Expect(k8sClient.Get(ctx, key, grant)).To(Succeed())
			cond := meta.FindStatusCondition(grant.Status.Conditions, dbv1alpha1.ConditionTypeReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))

			// Verify finalizer
			Expect(controllerutil.ContainsFinalizer(grant, dbv1alpha1.FinalizerName)).To(BeTrue())
		})
	})

	Context("Deletion with cleanupPolicy: Skip (default)", func() {
		const grantCRName = "test-grant-skip"

		It("should remove finalizer without revoking", func() {
			grant := &dbv1alpha1.PostgreSQLGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      grantCRName,
					Namespace: "default",
				},
				Spec: dbv1alpha1.PostgreSQLGrantSpec{
					SQLTemplate: dbv1alpha1.SQLTemplate{
						CleanupPolicy: dbv1alpha1.CleanupPolicySkip,
					},
					PostgresRef: dbv1alpha1.PostgresRef{
						Name:      "postgresql-staging",
						Namespace: "postgresql",
					},
					Privileges: []string{"ALL PRIVILEGES"},
					Database:   grantDBName,
					Schema:     "public",
					Role:       grantRoleName,
				},
			}
			Expect(k8sClient.Create(ctx, grant)).To(Succeed())

			key := types.NamespacedName{Name: grantCRName, Namespace: "default"}

			// Create
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			// Delete
			Expect(k8sClient.Get(ctx, key, grant)).To(Succeed())
			Expect(k8sClient.Delete(ctx, grant)).To(Succeed())

			// Reconcile deletion
			Expect(k8sClient.Get(ctx, key, grant)).To(Succeed())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
