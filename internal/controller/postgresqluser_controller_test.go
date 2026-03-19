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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dbv1alpha1 "github.com/zernico/postgresql-access-operator/api/v1alpha1"
	pgsql "github.com/zernico/postgresql-access-operator/internal/sql"
)

var _ = Describe("PostgreSQLUser Controller", func() {
	reconciler := &PostgreSQLUserReconciler{}
	var pgClient *pgsql.Client

	BeforeEach(func() {
		reconciler = &PostgreSQLUserReconciler{
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

	Context("Happy path: creating a user/role", func() {
		const userCRName = "test-user-happy"
		const roleName = "happy_test_role"
		const secretName = "test-user-happy-db"

		BeforeEach(func() {
			// Create the password secret in default namespace
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: "default",
				},
				Data: map[string][]byte{"password": []byte("testpass123")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		})

		AfterEach(func() {
			_ = pgClient.DropRole(ctx, roleName)
			user := &dbv1alpha1.PostgreSQLUser{}
			key := types.NamespacedName{Name: userCRName, Namespace: "default"}
			if err := k8sClient.Get(ctx, key, user); err == nil {
				controllerutil.RemoveFinalizer(user, dbv1alpha1.FinalizerName)
				_ = k8sClient.Update(ctx, user)
				_ = k8sClient.Delete(ctx, user)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: "default"}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should create the role in PostgreSQL and set Ready=True", func() {
			user := &dbv1alpha1.PostgreSQLUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      userCRName,
					Namespace: "default",
				},
				Spec: dbv1alpha1.PostgreSQLUserSpec{
					PostgresRef: dbv1alpha1.PostgresRef{
						Name:      "postgresql-staging",
						Namespace: "postgresql",
					},
					Name: roleName,
					PasswordSecretKeyRef: dbv1alpha1.SecretKeyRef{
						Name: secretName,
						Key:  "password",
					},
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())

			key := types.NamespacedName{Name: userCRName, Namespace: "default"}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Verify role was created in PostgreSQL
			exists, err := pgClient.RoleExists(ctx, roleName)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue(), "role should exist in PostgreSQL")

			// Verify status
			Expect(k8sClient.Get(ctx, key, user)).To(Succeed())
			cond := meta.FindStatusCondition(user.Status.Conditions, dbv1alpha1.ConditionTypeReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))

			// Verify finalizer
			Expect(controllerutil.ContainsFinalizer(user, dbv1alpha1.FinalizerName)).To(BeTrue())
		})
	})

	Context("Deletion with cleanupPolicy: Delete", func() {
		const userCRName = "test-user-delete"
		const roleName = "delete_policy_role"
		const secretName = "test-user-delete-db"

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: "default"},
				Data:       map[string][]byte{"password": []byte("testpass123")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		})

		AfterEach(func() {
			_ = pgClient.DropRole(ctx, roleName)
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: "default"}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should DROP the role when CR is deleted", func() {
			user := &dbv1alpha1.PostgreSQLUser{
				ObjectMeta: metav1.ObjectMeta{Name: userCRName, Namespace: "default"},
				Spec: dbv1alpha1.PostgreSQLUserSpec{
					SQLTemplate: dbv1alpha1.SQLTemplate{
						CleanupPolicy: dbv1alpha1.CleanupPolicyDelete,
					},
					PostgresRef: dbv1alpha1.PostgresRef{
						Name:      "postgresql-staging",
						Namespace: "postgresql",
					},
					Name: roleName,
					PasswordSecretKeyRef: dbv1alpha1.SecretKeyRef{
						Name: secretName,
						Key:  "password",
					},
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())

			key := types.NamespacedName{Name: userCRName, Namespace: "default"}

			// Create
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			exists, err := pgClient.RoleExists(ctx, roleName)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())

			// Delete the CR
			Expect(k8sClient.Get(ctx, key, user)).To(Succeed())
			Expect(k8sClient.Delete(ctx, user)).To(Succeed())

			// Reconcile deletion
			Expect(k8sClient.Get(ctx, key, user)).To(Succeed())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			// Verify role was dropped
			exists, err = pgClient.RoleExists(ctx, roleName)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse(), "role should be dropped")
		})
	})
})
