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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dbv1alpha1 "github.com/zernico/postgresql-access-operator/api/v1alpha1"
)

var _ = Describe("PostgreSQL Controller", func() {
	Context("When the PostgreSQL CR references a reachable instance", func() {
		It("should report Ready=True after successful connection", func() {
			reconciler := &PostgreSQLReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			key := types.NamespacedName{Name: "postgresql-staging", Namespace: "postgresql"}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Verify status condition
			pg := &dbv1alpha1.PostgreSQL{}
			Expect(k8sClient.Get(ctx, key, pg)).To(Succeed())

			cond := meta.FindStatusCondition(pg.Status.Conditions, dbv1alpha1.ConditionTypeReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(dbv1alpha1.ReasonSucceeded))
		})
	})

	Context("When the superuser secret is missing", func() {
		const pgName = "pg-missing-secret"

		BeforeEach(func() {
			pg := &dbv1alpha1.PostgreSQL{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pgName,
					Namespace: "postgresql",
				},
				Spec: dbv1alpha1.PostgreSQLSpec{
					Host: "localhost",
					Port: 5432,
					SuperuserSecretKeyRef: dbv1alpha1.SecretKeyRef{
						Name: "nonexistent-secret",
						Key:  "password",
					},
				},
			}
			Expect(k8sClient.Create(ctx, pg)).To(Succeed())
		})

		AfterEach(func() {
			pg := &dbv1alpha1.PostgreSQL{}
			key := types.NamespacedName{Name: pgName, Namespace: "postgresql"}
			Expect(k8sClient.Get(ctx, key, pg)).To(Succeed())
			Expect(k8sClient.Delete(ctx, pg)).To(Succeed())
		})

		It("should report Ready=False", func() {
			reconciler := &PostgreSQLReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			key := types.NamespacedName{Name: pgName, Namespace: "postgresql"}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			pg := &dbv1alpha1.PostgreSQL{}
			Expect(k8sClient.Get(ctx, key, pg)).To(Succeed())

			cond := meta.FindStatusCondition(pg.Status.Conditions, dbv1alpha1.ConditionTypeReady)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(dbv1alpha1.ReasonFailed))
		})
	})
})
