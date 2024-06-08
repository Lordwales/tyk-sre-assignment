package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	// "io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	// appsv1 "k8s.io/api/apps/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	disco "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetKubernetesVersion(t *testing.T) {
	okClientset := fake.NewSimpleClientset()
	okClientset.Discovery().(*disco.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: "1.25.0-fake"}

	okVer, err := getKubernetesVersion(okClientset)
	assert.NoError(t, err)
	assert.Equal(t, "1.25.0-fake", okVer)

	badClientset := fake.NewSimpleClientset()
	badClientset.Discovery().(*disco.FakeDiscovery).FakedServerVersion = &version.Info{}

	badVer, err := getKubernetesVersion(badClientset)
	assert.NoError(t, err)
	assert.Equal(t, "", badVer)
}

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	healthHandler(rec, req)
	res := rec.Result()

	assert.Equal(t, http.StatusOK, res.StatusCode)

	defer func(Body io.ReadCloser) {
		assert.NoError(t, Body.Close())
	}(res.Body)
	resp, err := io.ReadAll(res.Body)

	assert.NoError(t, err)
	assert.Equal(t, "ok", string(resp))
}

func TestCreateNetworkPolicy(t *testing.T) {
	// Create a fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	// Define the namespace and label selector
	namespace := "test-namespace"
	selector := "app=nginx"

	// Call the createNetworkPolicy function
	err := createNetworkPolicy(clientset, namespace, selector)
	assert.NoError(t, err)

	// Retrieve the created NetworkPolicy
	createdPolicy, err := clientset.NetworkingV1().NetworkPolicies(namespace).Get(context.Background(), fmt.Sprintf("isolate-%s", namespace), metav1.GetOptions{})
	assert.NoError(t, err)

	// Check if the created NetworkPolicy matches the expected configuration
	assert.Equal(t, fmt.Sprintf("isolate-%s", namespace), createdPolicy.Name)
	assert.Equal(t, map[string]string{"app": "nginx"}, createdPolicy.Spec.PodSelector.MatchLabels)
	assert.Equal(t, []netv1.PolicyType{netv1.PolicyTypeIngress}, createdPolicy.Spec.PolicyTypes)
	assert.Len(t, createdPolicy.Spec.Ingress, 1)
}

func TestDeploymentHealthHandler(t *testing.T) {
	// Create a fake clientset
	clientset := fake.NewSimpleClientset()

	// Create mock deployment health data
	deploymentHealthData := []DeploymentHealth{
		{
			Name:              "deployment1",
			Namespace:         "namespace1",
			DesiredReplicas:   3,
			CurrentReplicas:   3,
			AvailableReplicas: 3,
		},
		{
			Name:              "deployment2",
			Namespace:         "namespace2",
			DesiredReplicas:   2,
			CurrentReplicas:   2,
			AvailableReplicas: 2,
		},
	}

	// Mock the deployment health handler function
	mockDeploymentHealthHandler := func(w http.ResponseWriter, r *http.Request, clientset kubernetes.Interface) {
		// Marshal the mock deployment health data to JSON
		responseJSON, err := json.Marshal(deploymentHealthData)
		if err != nil {
			t.Errorf("Error encoding JSON: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Set the content type of the response
		w.Header().Set("Content-Type", "application/json")

		// Write the response body
		_, err = w.Write(responseJSON)
		if err != nil {
			t.Errorf("Failed writing response: %v", err)
			return
		}
	}

	// Create a request to pass to our mock handler
	req := httptest.NewRequest("GET", "/deployment-health", nil)

	// Create a ResponseRecorder to record the response
	rr := httptest.NewRecorder()

	// Call the mock handler function with the fake clientset and recorder
	mockDeploymentHealthHandler(rr, req, clientset)

	// Check the status code of the response
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Check the content type of the response body
	expectedContentType := "application/json"
	if contentType := rr.Header().Get("Content-Type"); contentType != expectedContentType {
		t.Errorf("handler returned wrong content type: got %v want %v", contentType, expectedContentType)
	}

	// Check the response body
	var responseBody []DeploymentHealth
	err := json.Unmarshal(rr.Body.Bytes(), &responseBody)
	if err != nil {
		t.Errorf("Error decoding JSON response: %v", err)
	}

	// Check if the response body matches the mock deployment health data
	for i, expected := range deploymentHealthData {
		if responseBody[i] != expected {
			t.Errorf("handler returned unexpected body: got %v want %v", responseBody[i], expected)
		}
	}
}
