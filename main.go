package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// HelmChartInfo represents each Helm release
type HelmChartInfo struct {
	ChartName string `json:"chart_name"`
	Version   string `json:"version"`
	Namespace string `json:"namespace"`
}

// ClusterInfo represents the full cluster metadata
type ClusterInfo struct {
	ClusterName string          `json:"cluster_name"`
	KubeVersion string          `json:"kube_version"`
	HelmCharts  []HelmChartInfo `json:"helm_charts"`
}

// HelmRelease represents Helm metadata inside the secret
type HelmRelease struct {
	Chart struct {
		Metadata struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"metadata"`
	} `json:"chart"`
}

func main() {
	// Create in-cluster Kubernetes client
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Failed to create cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create clientset: %v", err)
	}

	// Get cluster metadata
	clusterName := getClusterName(clientset)
	kubeVersion := getKubernetesVersion(clientset)

	var helmCharts []HelmChartInfo

	namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Failed to list namespaces: %v", err)
	}

	// Iterate through namespaces and fetch Helm releases
	for _, ns := range namespaces.Items {
		secrets, err := clientset.CoreV1().Secrets(ns.Name).List(context.TODO(), metav1.ListOptions{
			LabelSelector: "owner=helm",
		})
		if err != nil {
			log.Printf("Failed to get secrets in namespace %s: %v", ns.Name, err)
			continue
		}

		for _, secret := range secrets.Items {
			releaseData, ok := secret.Data["release"]
			if !ok {
				continue
			}

			decodedData, err := base64.StdEncoding.DecodeString(string(releaseData))
			if err != nil {
				log.Printf("Failed to decode base64: %v", err)
				continue
			}

			gzReader, err := gzip.NewReader(bytes.NewReader(decodedData))
			if err != nil {
				log.Printf("Failed to create gzip reader: %v", err)
				continue
			}
			defer gzReader.Close()

			var decompressedData bytes.Buffer
			if _, err := io.Copy(&decompressedData, gzReader); err != nil {
				log.Printf("Failed to decompress: %v", err)
				continue
			}

			var helmRelease HelmRelease
			if err := json.Unmarshal(decompressedData.Bytes(), &helmRelease); err != nil {
				log.Printf("Failed to parse JSON: %v", err)
				continue
			}

			// Append to JSON output list
			helmCharts = append(helmCharts, HelmChartInfo{
				ChartName: helmRelease.Chart.Metadata.Name,
				Version:   helmRelease.Chart.Metadata.Version,
				Namespace: ns.Name,
			})
		}
	}

	output := ClusterInfo{
		ClusterName: clusterName,
		KubeVersion: kubeVersion,
		HelmCharts:  helmCharts,
	}

	// Convert JSON to a byte buffer
	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Fatalf("Failed to convert to JSON: %v", err)
	}

	// Send the JSON data to the remote API
	sendDataToAPI(jsonData)
}

// sendDataToAPI sends collected Helm release data to the remote API
func sendDataToAPI(jsonData []byte) {
	apiURL := os.Getenv("API_URL")
	apiToken := os.Getenv("API_TOKEN")

	if apiURL == "" || apiToken == "" {
		log.Println("API_URL or API_TOKEN not set, skipping API request")
		return
	}

	req, err := http.NewRequest("PUT", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-token", apiToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to send data to API: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Println("Successfully sent data to API")
	} else {
		log.Printf("API request failed with status: %d", resp.StatusCode)
	}
}

// getClusterName fetches the cluster name from an environment variable or ConfigMap
func getClusterName(clientset *kubernetes.Clientset) string {
	if envClusterName := os.Getenv("CLUSTER_NAME"); envClusterName != "" {
		log.Printf("Using cluster name from environment: %s", envClusterName)
		return envClusterName
	}

	configMap, err := clientset.CoreV1().ConfigMaps("kube-system").Get(context.TODO(), "kubeadm-config", metav1.GetOptions{})
	if err == nil {
		clusterName, ok := configMap.Data["ClusterConfiguration"]
		if ok {
			log.Printf("Using cluster name from kubeadm-config: %s", clusterName)
			return clusterName
		}
	}

	log.Println("Cluster name not found, using default 'unknown-cluster'")
	return "unknown-cluster"
}

// getKubernetesVersion fetches the Kubernetes API server version
func getKubernetesVersion(clientset *kubernetes.Clientset) string {
	versionInfo, err := clientset.Discovery().ServerVersion()
	if err != nil {
		log.Println("Failed to fetch Kubernetes version, using 'unknown-version'")
		return "unknown-version"
	}
	return versionInfo.GitVersion
}
