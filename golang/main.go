package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"context"
	"encoding/json"

	netv1 "k8s.io/api/networking/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig, leave empty for in-cluster")
	listenAddr := flag.String("address", "", "HTTP server listen address")
	namespace := flag.String("namespace", "", "namespace for network policy")
	selector := flag.String("selector", "", "label selector for pods in namespace")
	flag.Parse()

	// Check if the flag is set, otherwise use the environment variable
	if *kubeconfig == "" {
		*kubeconfig = getEnv("KUBECONFIG", "~/.kube/config")
	}
	if *listenAddr == "" {
		*listenAddr = getEnv("LISTEN_ADDRESS", "8081")
	}
	if *namespace == "" {
		*namespace = getEnv("NAMESPACE", "default")
	}
	if *selector == "" {
		*selector = getEnv("SELECTOR", "")
	}

	kConfig, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}

	clientset, err := kubernetes.NewForConfig(kConfig)
	if err != nil {
		panic(err)
	}

	version, err := getKubernetesVersion(clientset)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Connected to Kubernetes %s\n", version)

	// Check for required flags
	if *namespace == "" || *selector == "" {
		fmt.Println("Error: Missing required flags. Please provide values for all flags.")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Create NetworkPolicy to isolate network traffic between namespaces
	err = createNetworkPolicy(clientset, *namespace, *selector)
	if err != nil {
		log.Fatalf("Error creating NetworkPolicy: %v", err)
	}

	fmt.Printf("NetworkPolicy created to isolate traffic for namespace: %s and %s workloads\n", *namespace, *selector)

	// Check deployment health after the server has started
	// if err := checkDeploymentHealth(clientset); err != nil {
	// 	panic(err)
	// }

	if err := startServer(*listenAddr, clientset); err != nil {
		panic(err)
	}

}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// getKubernetesVersion returns a string GitVersion of the Kubernetes server defined by the clientset.
//
// If it can't connect an error will be returned, which makes it useful to check connectivity.
func getKubernetesVersion(clientset kubernetes.Interface) (string, error) {
	version, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}

	return version.String(), nil
}

// startServer launches an HTTP server with defined handlers and blocks until it's terminated or fails with an error.
//
// Expects a listenAddr to bind to.
func startServer(listenAddr string, clientset kubernetes.Interface) error {
	http.HandleFunc("/healthz", healthHandler)
	http.HandleFunc("/deployment-health", func(w http.ResponseWriter, r *http.Request) {
		deploymentHealthHandler(w, r, clientset)
	})
	http.HandleFunc("/kube-api-health", func(w http.ResponseWriter, r *http.Request) {
		if err := checkKubernetesAPIConnectivity(clientset); err != nil {
			http.Error(w, fmt.Sprintf("Kubernetes API server is unreachable: %v", err), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Kubernetes API server is reachable"))
	})

	fmt.Printf("Server listening on %s\n", listenAddr)

	return http.ListenAndServe(listenAddr, nil)
}

// healthHandler responds with the health status of the application.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)

	_, err := w.Write([]byte("ok"))
	if err != nil {
		fmt.Println("failed writing to response")
	}
}

// deploymentHealthHandler responds with the health status of deployments in the Kubernetes cluster.
func deploymentHealthHandler(w http.ResponseWriter, r *http.Request, clientset kubernetes.Interface) {
	// Retrieve deployments client
	deploymentsClient := clientset.AppsV1().Deployments(v1.NamespaceAll)

	// Define struct for holding deployment health information
	type DeploymentHealth struct {
		Name              string `json:"name"`
		DesiredReplicas   int32  `json:"desiredReplicas"`
		CurrentReplicas   int32  `json:"currentReplicas"`
		AvailableReplicas int32  `json:"availableReplicas"`
	}

	// Slice to hold information about unhealthy deployments
	var unhealthyDeployments []DeploymentHealth

	// List all deployments
	deployments, err := deploymentsClient.List(context.Background(), v1.ListOptions{})
	if err != nil {
		// If there is an error listing deployments, respond with an internal server error
		http.Error(w, fmt.Sprintf("Error listing deployments: %v", err), http.StatusInternalServerError)
		return
	}

	// Iterate through deployments
	for _, d := range deployments.Items {
		desiredReplicas := *d.Spec.Replicas
		currentReplicas := d.Status.Replicas
		availableReplicas := d.Status.AvailableReplicas

		// Check if deployment is unhealthy
		if currentReplicas != desiredReplicas || availableReplicas != desiredReplicas {
			// Append deployment health information to the slice
			unhealthyDeployments = append(unhealthyDeployments, DeploymentHealth{
				Name:              d.Name,
				DesiredReplicas:   desiredReplicas,
				CurrentReplicas:   currentReplicas,
				AvailableReplicas: availableReplicas,
			})
		}
	}

	// If there are no unhealthy deployments, respond with a message indicating all pods are healthy
	if len(unhealthyDeployments) == 0 {
		responseText := "All deployments are healthy"
		_, err := w.Write([]byte(responseText))
		if err != nil {
			fmt.Println("Failed writing response:", err)
		}
		return
	}

	// Encode response JSON
	responseJSON, err := json.Marshal(unhealthyDeployments)
	if err != nil {
		// If there is an error encoding JSON, respond with an internal server error
		http.Error(w, fmt.Sprintf("Error encoding JSON: %v", err), http.StatusInternalServerError)
		return
	}

	// Respond with JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(responseJSON)
	if err != nil {
		fmt.Println("Failed writing response:", err)
	}
}

func checkKubernetesAPIConnectivity(clientset kubernetes.Interface) error {
	_, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("failed to connect to Kubernetes API server: %v", err)
	}
	return nil
}

func createNetworkPolicy(clientset kubernetes.Interface, namespace, selector string) error {
	// Check if the NetworkPolicy already exists
	_, err := clientset.NetworkingV1().NetworkPolicies(namespace).Get(context.Background(), fmt.Sprintf("isolate-%s", namespace), v1.GetOptions{})
	if err == nil {
		// NetworkPolicy already exists, you can choose to update it if needed
		return nil
	}

	policy := &netv1.NetworkPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name: fmt.Sprintf("isolate-%s", namespace),
		},
		//specifications for the NetworkPolicy
		Spec: netv1.NetworkPolicySpec{
			PodSelector: v1.LabelSelector{
				MatchLabels: parseLabelSelector(selector), // the particular workload in destination namespace
			},
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeIngress},
			Ingress: []netv1.NetworkPolicyIngressRule{
				{
					From: []netv1.NetworkPolicyPeer{
						{
							PodSelector: &v1.LabelSelector{},
							// NamespaceSelector: &v1.LabelSelector{},
						},
					},
				},
			},
		},
	}

	// Apply the NetworkPolicy to the destination namespace
	_, err = clientset.NetworkingV1().NetworkPolicies(namespace).Create(context.Background(), policy, v1.CreateOptions{})
	return err
}

func parseLabelSelector(selector string) map[string]string {
	labels := make(map[string]string)
	// Split the label selector string by commas and parse key-value pairs
	labelPairs := strings.Split(selector, ",")
	for _, pair := range labelPairs {
		parts := strings.Split(pair, "=")
		if len(parts) != 2 {
			continue
		}
		labels[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return labels
}

// checkDeploymentHealth checks the health of all deployments in the Kubernetes cluster.
// func checkDeploymentHealth(clientset kubernetes.Interface) error {
// 	// Retrieve a client for accessing deployments across all namespaces
// 	deploymentsClient := clientset.AppsV1().Deployments(v1.NamespaceAll)

// 	// List all deployments
// 	deployments, err := deploymentsClient.List(context.Background(), v1.ListOptions{})
// 	if err != nil {
// 		// Print error if listing deployments fails
// 		return fmt.Errorf("Error listing deployments: %v\n", err)
// 	}

// 	// Iterate through each deployment
// 	for _, d := range deployments.Items {
// 		// Retrieve desired number of replicas from deployment spec
// 		desiredReplicas := *d.Spec.Replicas
// 		// Retrieve current number of replicas
// 		currentReplicas := d.Status.Replicas
// 		// Retrieve number of available replicas
// 		availableReplicas := d.Status.AvailableReplicas

// 		// Check if current replicas or available replicas differ from desired replicas
// 		if currentReplicas != desiredReplicas || availableReplicas != desiredReplicas {
// 			// Print deployment health status if unhealthy
// 			return fmt.Errorf("Deployment %s is unhealthy: Desired=%d, Current=%d, Available=%d\n",
// 				d.Name, desiredReplicas, currentReplicas, availableReplicas)
// 		}
// 	}

// 	return nil
// }
