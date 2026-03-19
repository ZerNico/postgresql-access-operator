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

var _ = Describe("PostgreSQLDatabase Controller", func() {
	reconciler := &PostgreSQLDatabaseReconciler{}
	var pgClient *pgsql.Client

	BeforeEach(func() {
		reconciler = &PostgreSQLDatabaseReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
		var err error
		pgClient, err = pgsql.Connect(ctx, pgsql.ConnectConfig{
			Host: pgHost, Port: pgPort, Database: testPgDB,
			Username: testPgUser, Password: testPgPassword,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_ = pgClient.Close(ctx)
	})

	Context("Happy path: creating a database", func() {
		const dbCRName = "test-db-happy"
		const dbName = "happy_test_db"

		AfterEach(func() {
			// Cleanup: drop DB from PostgreSQL and delete the CR
			_ = pgClient.DropDatabase(ctx, dbName)
			db := &dbv1alpha1.PostgreSQLDatabase{}
			key := types.NamespacedName{Name: dbCRName, Namespace: "default"}
			if err := k8sClient.Get(ctx, key, db); err == nil {
				controllerutil.RemoveFinalizer(db, dbv1alpha1.FinalizerName)
				_ = k8sClient.Update(ctx, db)
				_ = k8sClient.Delete(ctx, db)
			}
		})

		It("should create the database in PostgreSQL and set Ready=True", func() {
			db := &dbv1alpha1.PostgreSQLDatabase{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbCRName,
					Namespace: "default",
				},
				Spec: dbv1alpha1.PostgreSQLDatabaseSpec{
					PostgresRef: dbv1alpha1.PostgresRef{
						Name:      "postgresql-staging",
						Namespace: "postgresql",
					},
					Name: dbName,
				},
			}
			Expect(k8sClient.Create(ctx, db)).To(Succeed())

			key := types.NamespacedName{Name: dbCRName, Namespace: "default"}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Verify database was actually created in PostgreSQL
			exists, err := pgClient.DatabaseExists(ctx, dbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue(), "database should exist in PostgreSQL")

			// Verify status condition
			Expect(k8sClient.Get(ctx, key, db)).To(Succeed())
			cond := meta.FindStatusCondition(db.Status.Conditions, dbv1alpha1.ConditionTypeReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))

			// Verify finalizer was added
			Expect(controllerutil.ContainsFinalizer(db, dbv1alpha1.FinalizerName)).To(BeTrue())
		})
	})

	Context("Name fallback: spec.name empty, uses metadata.name", func() {
		const dbCRName = "fallback-db-name"

		AfterEach(func() {
			_ = pgClient.DropDatabase(ctx, dbCRName)
			db := &dbv1alpha1.PostgreSQLDatabase{}
			key := types.NamespacedName{Name: dbCRName, Namespace: "default"}
			if err := k8sClient.Get(ctx, key, db); err == nil {
				controllerutil.RemoveFinalizer(db, dbv1alpha1.FinalizerName)
				_ = k8sClient.Update(ctx, db)
				_ = k8sClient.Delete(ctx, db)
			}
		})

		It("should use metadata.name as database name", func() {
			db := &dbv1alpha1.PostgreSQLDatabase{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbCRName,
					Namespace: "default",
				},
				Spec: dbv1alpha1.PostgreSQLDatabaseSpec{
					PostgresRef: dbv1alpha1.PostgresRef{
						Name:      "postgresql-staging",
						Namespace: "postgresql",
					},
					// Name intentionally empty
				},
			}
			Expect(k8sClient.Create(ctx, db)).To(Succeed())

			key := types.NamespacedName{Name: dbCRName, Namespace: "default"}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			// The database name in PG should be the CR metadata.name
			exists, err := pgClient.DatabaseExists(ctx, dbCRName)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())
		})
	})

	Context("Deletion with cleanupPolicy: Delete", func() {
		const dbCRName = "test-db-delete"
		const dbName = "delete_policy_db"

		It("should DROP the database when CR is deleted", func() {
			db := &dbv1alpha1.PostgreSQLDatabase{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbCRName,
					Namespace: "default",
				},
				Spec: dbv1alpha1.PostgreSQLDatabaseSpec{
					SQLTemplate: dbv1alpha1.SQLTemplate{
						CleanupPolicy: dbv1alpha1.CleanupPolicyDelete,
					},
					PostgresRef: dbv1alpha1.PostgresRef{
						Name:      "postgresql-staging",
						Namespace: "postgresql",
					},
					Name: dbName,
				},
			}
			Expect(k8sClient.Create(ctx, db)).To(Succeed())

			key := types.NamespacedName{Name: dbCRName, Namespace: "default"}

			// First reconcile: create the database
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			exists, err := pgClient.DatabaseExists(ctx, dbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())

			// Delete the CR
			Expect(k8sClient.Get(ctx, key, db)).To(Succeed())
			Expect(k8sClient.Delete(ctx, db)).To(Succeed())

			// Reconcile the deletion
			Expect(k8sClient.Get(ctx, key, db)).To(Succeed())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			// Verify database was dropped
			exists, err = pgClient.DatabaseExists(ctx, dbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse(), "database should be dropped after delete with cleanupPolicy=Delete")
		})
	})

	Context("Deletion with cleanupPolicy: Skip", func() {
		const dbCRName = "test-db-skip"
		const dbName = "skip_policy_db"

		AfterEach(func() {
			_ = pgClient.DropDatabase(ctx, dbName)
		})

		It("should NOT drop the database when CR is deleted", func() {
			db := &dbv1alpha1.PostgreSQLDatabase{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbCRName,
					Namespace: "default",
				},
				Spec: dbv1alpha1.PostgreSQLDatabaseSpec{
					SQLTemplate: dbv1alpha1.SQLTemplate{
						CleanupPolicy: dbv1alpha1.CleanupPolicySkip,
					},
					PostgresRef: dbv1alpha1.PostgresRef{
						Name:      "postgresql-staging",
						Namespace: "postgresql",
					},
					Name: dbName,
				},
			}
			Expect(k8sClient.Create(ctx, db)).To(Succeed())

			key := types.NamespacedName{Name: dbCRName, Namespace: "default"}

			// First reconcile: create the database
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			// Delete the CR
			Expect(k8sClient.Get(ctx, key, db)).To(Succeed())
			Expect(k8sClient.Delete(ctx, db)).To(Succeed())

			// Reconcile the deletion
			Expect(k8sClient.Get(ctx, key, db)).To(Succeed())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			// Verify database was NOT dropped
			exists, err := pgClient.DatabaseExists(ctx, dbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue(), "database should still exist after delete with cleanupPolicy=Skip")
		})
	})

	Context("When PostgreSQL CR does not exist", func() {
		const dbCRName = "test-db-no-pg"

		AfterEach(func() {
			db := &dbv1alpha1.PostgreSQLDatabase{}
			key := types.NamespacedName{Name: dbCRName, Namespace: "default"}
			if err := k8sClient.Get(ctx, key, db); err == nil {
				_ = k8sClient.Delete(ctx, db)
			}
		})

		It("should set Ready=False and requeue", func() {
			db := &dbv1alpha1.PostgreSQLDatabase{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbCRName,
					Namespace: "default",
				},
				Spec: dbv1alpha1.PostgreSQLDatabaseSpec{
					PostgresRef: dbv1alpha1.PostgresRef{
						Name:      "nonexistent-pg",
						Namespace: "postgresql",
					},
					Name: "wont_be_created",
				},
			}
			Expect(k8sClient.Create(ctx, db)).To(Succeed())

			key := types.NamespacedName{Name: dbCRName, Namespace: "default"}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// No finalizer should be added (SQL never succeeded)
			Expect(k8sClient.Get(ctx, key, db)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(db, dbv1alpha1.FinalizerName)).To(BeFalse())

			// Status should be Failed
			cond := meta.FindStatusCondition(db.Status.Conditions, dbv1alpha1.ConditionTypeReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})
	})
})
