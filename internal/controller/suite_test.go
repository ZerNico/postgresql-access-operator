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
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	dbv1alpha1 "github.com/zernico/postgresql-access-operator/api/v1alpha1"
	// +kubebuilder:scaffold:imports
)

// Shared test variables
var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client

	// PostgreSQL testcontainer connection details
	pgHost string
	pgPort int32

	// Container reference for cleanup
	pgContainer testcontainers.Container
)

const (
	testPgPassword = "supertestpassword"
	testPgUser     = "postgres"
	testPgDB       = "postgres"
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	var err error
	err = dbv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("starting PostgreSQL testcontainer")
	pgContainer, err = postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase(testPgDB),
		postgres.WithUsername(testPgUser),
		postgres.WithPassword(testPgPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	Expect(err).NotTo(HaveOccurred())

	host, err := pgContainer.Host(ctx)
	Expect(err).NotTo(HaveOccurred())
	pgHost = host

	mappedPort, err := pgContainer.MappedPort(ctx, "5432")
	Expect(err).NotTo(HaveOccurred())
	pgPort = int32(mappedPort.Int())

	GinkgoWriter.Printf("PostgreSQL testcontainer running at %s:%d\n", pgHost, pgPort)

	By("creating test namespace and initial data")
	createTestData(ctx)
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()

	if pgContainer != nil {
		_ = pgContainer.Terminate(context.Background())
	}

	Eventually(func() error {
		return testEnv.Stop()
	}, time.Minute, time.Second).Should(Succeed())
})

// createTestData creates the PostgreSQL CR and superuser Secret that all tests reference.
func createTestData(ctx context.Context) {
	// Create the "postgresql" namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "postgresql"},
	}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())

	// Create the superuser password Secret in the "postgresql" namespace
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgresql-staging-root",
			Namespace: "postgresql",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"password": []byte(testPgPassword),
		},
	}
	Expect(k8sClient.Create(ctx, secret)).To(Succeed())

	// Create the PostgreSQL CR pointing to our testcontainer
	pg := &dbv1alpha1.PostgreSQL{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgresql-staging",
			Namespace: "postgresql",
		},
		Spec: dbv1alpha1.PostgreSQLSpec{
			Host:              pgHost,
			Port:              pgPort,
			Database:          testPgDB,
			SuperuserUsername: testPgUser,
			SuperuserSecretKeyRef: dbv1alpha1.SecretKeyRef{
				Name: "postgresql-staging-root",
				Key:  "password",
			},
		},
	}
	Expect(k8sClient.Create(ctx, pg)).To(Succeed())
}

func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
