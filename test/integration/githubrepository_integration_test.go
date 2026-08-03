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

package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	githubv1alpha1 "github.com/pierinho13/github-platform-operator/api/v1alpha1"
	"github.com/pierinho13/github-platform-operator/internal/controller"
	githubclient "github.com/pierinho13/github-platform-operator/internal/github"
)

const (
	integrationNamespace = "github-simulated-integration"
	providerName         = "simulated-github"
	repositoryName       = "test-repository"
	integrationToken     = "simulated-token"
	desiredDescription   = "managed by the simulated GitHub integration test"
	readyCondition       = "Ready"
)

// TestGitHubRepositoryReconcilesThroughKubernetesAndGitHub exercises the same
// path used in production: Kubernetes API -> controller-runtime manager ->
// reconciler -> REST-backed GitHub client -> GitHub API -> CR status. Only the
// final network boundary is simulated, so the test needs no external token.
func TestGitHubRepositoryReconcilesThroughKubernetesAndGitHub(t *testing.T) {
	scheme := integrationScheme(t)
	githubAPI := newSimulatedGitHubAPI()
	server := httptest.NewServer(githubAPI)
	t.Cleanup(server.Close)

	testEnvironment := &envtest.Environment{
		CRDDirectoryPaths:        []string{crdDirectory(t)},
		ErrorIfCRDPathMissing:    true,
		BinaryAssetsDirectory:    envtestBinaryDirectory(t),
		AttachControlPlaneOutput: false,
	}
	config, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() {
		if stopErr := testEnvironment.Stop(); stopErr != nil {
			t.Errorf("stop envtest: %v", stopErr)
		}
	})

	manager, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
	})
	if err != nil {
		t.Fatalf("create controller manager: %v", err)
	}

	httpClient := server.Client()
	clientFactory := &githubclient.RESTClientFactory{HTTPClient: httpClient}
	if err := (&controller.GitHubProviderConfigReconciler{
		Client:    manager.GetClient(),
		APIReader: manager.GetAPIReader(),
		Scheme:    manager.GetScheme(),
	}).SetupWithManager(manager); err != nil {
		t.Fatalf("register GitHubProviderConfig controller: %v", err)
	}
	if err := (&controller.GitHubRepositoryReconciler{
		Client:              manager.GetClient(),
		APIReader:           manager.GetAPIReader(),
		Scheme:              manager.GetScheme(),
		GitHubClientFactory: clientFactory,
		GitHubTokenProvider: githubclient.NewCachedTokenProvider(httpClient),
	}).SetupWithManager(manager); err != nil {
		t.Fatalf("register GitHubRepository controller: %v", err)
	}

	managerContext, cancelManager := context.WithCancel(context.Background())
	managerErrors := make(chan error, 1)
	go func() {
		managerErrors <- manager.Start(managerContext)
	}()
	t.Cleanup(func() {
		cancelManager()
		select {
		case managerErr := <-managerErrors:
			if managerErr != nil {
				t.Errorf("controller manager stopped with an error: %v", managerErr)
			}
		case <-time.After(10 * time.Second):
			t.Error("controller manager did not stop")
		}
	})

	if !manager.GetCache().WaitForCacheSync(managerContext) {
		t.Fatal("controller manager cache did not synchronize")
	}

	kubernetesClient := manager.GetClient()
	createIntegrationResources(t, managerContext, kubernetesClient, server.URL)

	var reconciled githubv1alpha1.GitHubRepository
	eventually(t, 20*time.Second, func() (bool, error) {
		if err := kubernetesClient.Get(
			managerContext,
			types.NamespacedName{Namespace: integrationNamespace, Name: repositoryName},
			&reconciled,
		); err != nil {
			return false, err
		}

		condition := meta.FindStatusCondition(reconciled.Status.Conditions, readyCondition)
		return condition != nil &&
			condition.Status == metav1.ConditionTrue &&
			reconciled.Status.ObservedGeneration == reconciled.Generation &&
			reconciled.Status.Description == desiredDescription, nil
	})

	if reconciled.Status.RepositoryID != 4242 {
		t.Errorf("repository ID is %d, want 4242", reconciled.Status.RepositoryID)
	}
	if reconciled.Status.URL != "https://github.com/k8sready/test-repository" {
		t.Errorf("repository URL is %q", reconciled.Status.URL)
	}
	assertSimulatedGitHubContract(t, githubAPI, 1)

	// Trigger another real controller event. The second reconciliation must GET
	// the remote state but must not send another PATCH once it is converged.
	reconciled.Annotations = map[string]string{"integration-test": "reconcile-again"}
	if err := kubernetesClient.Update(managerContext, &reconciled); err != nil {
		t.Fatalf("trigger second reconciliation: %v", err)
	}
	eventually(t, 20*time.Second, func() (bool, error) {
		getCalls, patchCalls, contractErrors := githubAPI.snapshot()
		return getCalls >= 2 && patchCalls == 1 && len(contractErrors) == 0, nil
	})
	assertSimulatedGitHubContract(t, githubAPI, 1)
}

func integrationScheme(t *testing.T) *k8sruntime.Scheme {
	t.Helper()

	scheme := k8sruntime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("register core Kubernetes scheme: %v", err)
	}
	if err := githubv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("register GitHub API scheme: %v", err)
	}
	return scheme
}

func crdDirectory(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("determine integration test directory")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..", "config", "crd", "bases")
}

func envtestBinaryDirectory(t *testing.T) string {
	t.Helper()

	if configuredDirectory := os.Getenv("KUBEBUILDER_ASSETS"); configuredDirectory != "" {
		return configuredDirectory
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("determine integration test directory")
	}
	binaryRoot := filepath.Join(filepath.Dir(filename), "..", "..", "bin", "k8s")
	entries, err := os.ReadDir(binaryRoot)
	if err != nil {
		// Leave the path empty so envtest reports its standard installation
		// instructions if no project-local assets have been downloaded yet.
		return ""
	}
	for index := len(entries) - 1; index >= 0; index-- {
		if entries[index].IsDir() {
			return filepath.Join(binaryRoot, entries[index].Name())
		}
	}
	return ""
}

func createIntegrationResources(
	t *testing.T,
	ctx context.Context,
	kubernetesClient client.Client,
	apiURL string,
) {
	t.Helper()

	resources := []client.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: integrationNamespace}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "github-token", Namespace: integrationNamespace},
			StringData: map[string]string{"token": integrationToken},
		},
		&githubv1alpha1.GitHubProviderConfig{
			ObjectMeta: metav1.ObjectMeta{Name: providerName},
			Spec: githubv1alpha1.GitHubProviderConfigSpec{
				Organization: "k8sready",
				APIURL:       apiURL,
				Credentials: githubv1alpha1.GitHubProviderCredentials{
					SecretRef: &githubv1alpha1.NamespacedSecretKeyReference{
						Namespace: integrationNamespace,
						Name:      "github-token",
						Key:       "token",
					},
				},
			},
		},
		&githubv1alpha1.GitHubRepository{
			ObjectMeta: metav1.ObjectMeta{Name: repositoryName, Namespace: integrationNamespace},
			Spec: githubv1alpha1.GitHubRepositorySpec{
				ProviderConfigRef: providerName,
				Name:              repositoryName,
				Description:       stringPointer(desiredDescription),
				DeletionPolicy:    githubv1alpha1.RepositoryDeletionPolicyOrphan,
			},
		},
	}

	for _, resource := range resources {
		if err := kubernetesClient.Create(ctx, resource); err != nil {
			t.Fatalf("create %T: %v", resource, err)
		}
	}
}

func stringPointer(value string) *string {
	return &value
}

func eventually(t *testing.T, timeout time.Duration, condition func() (bool, error)) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		matched, err := condition()
		if matched {
			return
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("condition did not become true: %v", lastErr)
	}
	t.Fatal("condition did not become true before timeout")
}

type simulatedGitHubAPI struct {
	mu             sync.Mutex
	description    string
	getCalls       int
	patchCalls     int
	contractErrors []string
}

func newSimulatedGitHubAPI() *simulatedGitHubAPI {
	return &simulatedGitHubAPI{description: "description before reconciliation"}
}

func (s *simulatedGitHubAPI) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if request.URL.Path != "/repos/k8sready/test-repository" {
		s.contractErrors = append(s.contractErrors, "unexpected path: "+request.URL.Path)
		http.NotFound(writer, request)
		return
	}
	if request.Header.Get("Authorization") != "Bearer "+integrationToken {
		s.contractErrors = append(s.contractErrors, "missing or incorrect bearer token")
	}
	if request.Header.Get("X-GitHub-Api-Version") == "" {
		s.contractErrors = append(s.contractErrors, "missing X-GitHub-Api-Version header")
	}

	switch request.Method {
	case http.MethodGet:
		s.getCalls++
	case http.MethodPatch:
		s.patchCalls++
		var update struct {
			Description *string `json:"description"`
		}
		if err := json.NewDecoder(request.Body).Decode(&update); err != nil {
			s.contractErrors = append(s.contractErrors, "decode PATCH: "+err.Error())
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		if update.Description == nil {
			s.contractErrors = append(s.contractErrors, "PATCH omitted description")
		} else {
			s.description = *update.Description
		}
	default:
		s.contractErrors = append(s.contractErrors, "unexpected method: "+request.Method)
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"id":          4242,
		"html_url":    "https://github.com/k8sready/test-repository",
		"visibility":  "private",
		"description": s.description,
	})
}

func (s *simulatedGitHubAPI) snapshot() (int, int, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getCalls, s.patchCalls, append([]string(nil), s.contractErrors...)
}

func assertSimulatedGitHubContract(
	t *testing.T,
	githubAPI *simulatedGitHubAPI,
	wantPatchCalls int,
) {
	t.Helper()

	getCalls, patchCalls, contractErrors := githubAPI.snapshot()
	if getCalls == 0 {
		t.Error("simulated GitHub API received no GET request")
	}
	if patchCalls != wantPatchCalls {
		t.Errorf("simulated GitHub API received %d PATCH requests, want %d", patchCalls, wantPatchCalls)
	}
	for _, contractError := range contractErrors {
		t.Errorf("GitHub HTTP contract violation: %s", contractError)
	}
}

// Compile-time assertion that the simulated API remains a valid HTTP handler.
var _ http.Handler = (*simulatedGitHubAPI)(nil)
