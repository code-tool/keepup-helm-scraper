package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"keepup-helm-scrapper/src/config"
	"keepup-helm-scrapper/src/rules"
	"log"
	"net/http"
	"os"
	"regexp"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type HelmChartInfo struct {
	ChartName string `json:"chart_name"`
	Version   string `json:"version"`
	Namespace string `json:"namespace"`
}

type ClusterInfo struct {
	ClusterName string          `json:"cluster_name"`
	KubeVersion string          `json:"kube_version"`
	HelmCharts  []HelmChartInfo `json:"helm_charts"`
}

func main() {
	ctx := context.Background()

	//kubeconfig, err := clientcmd.BuildConfigFromFlags("", "/home/.kube/minikube.cfg")
	kubeconfig, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("failed to get cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		log.Fatalf("failed to create clientset: %v", err)
	}

	rules, err := rules.LoadRules(config.GetEnvConfig().RULES_FILE)
	if err != nil {
		log.Fatalf("Can't configure RULES_FILE: %v", err)
	}

	var versionRe = regexp.MustCompile(`(\d+)\.(\d+)(\.\d+)?`)

	imagesByNs, err := CollectNamespaceImages(ctx, clientset)
	if err != nil {
		log.Fatal(err)
	}

	uniqImagesByNs := make(map[string]map[string]string)
	for ns, images := range imagesByNs {
		log.Println("Processing namespace:", ns)
		for _, img := range images {
			for _, rule := range rules {
				if rule.DetectionRegex.MatchString(img) {
					log.Printf("Matched %s -> %s\n", img, rule.ApplicationName)
					if v, ok := normalizeSemVer(rule.VersionRegex.FindString(img), versionRe); ok {
						log.Printf("Normalized %-90s -> %s\n", img, v)
						if _, ok := uniqImagesByNs[ns]; !ok {
							uniqImagesByNs[ns] = make(map[string]string)
						}
						uniqImagesByNs[ns][rule.ApplicationName] = v
					} else {
						log.Printf("%-90s -> no version\n", img)
					}
				}
			}
		}
	}

	var imagesInstalled []HelmChartInfo
	for ns, versionedImage := range uniqImagesByNs {
		for v, i := range versionedImage {
			imagesInstalled = append(imagesInstalled, HelmChartInfo{
				ChartName: i,
				Version:   v,
				Namespace: ns,
			})
		}
	}

	clusterName := getClusterName()
	kubeVersion := getKubernetesVersion(clientset)
	output := ClusterInfo{
		ClusterName: clusterName,
		KubeVersion: kubeVersion,
		HelmCharts:  imagesInstalled,
	}
	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Fatalf("Failed to convert to JSON: %v", err)
	}

	log.Printf("Sending versions: %s", imagesInstalled)
	sendDataToAPI(jsonData)
}

func CollectNamespaceImages(
	ctx context.Context,
	client kubernetes.Interface,
) (map[string][]string, error) {

	// accumulate to internal set
	acc := make(map[string]map[string]int)

	namespaces, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, ns := range namespaces.Items {
		nsName := ns.Name

		if _, ok := acc[nsName]; !ok {
			acc[nsName] = make(map[string]int)
		}

		if err := collectFromDeployments(ctx, client, nsName, acc); err != nil {
			return nil, err
		}
		if err := collectFromStatefulSets(ctx, client, nsName, acc); err != nil {
			return nil, err
		}
		if err := collectFromDaemonSets(ctx, client, nsName, acc); err != nil {
			return nil, err
		}
	}

	// normalize map[string]map[string]struct{} -> map[string][]string
	result := make(map[string][]string)
	for ns, images := range acc {
		for img := range images {
			result[ns] = append(result[ns], img)
		}
	}

	return result, nil
}

func collectImages(
	spec corev1.PodSpec,
	ns string,
	acc map[string]map[string]int,
) {
	for _, c := range spec.Containers {
		acc[ns][c.Image] = 1
	}
	for _, c := range spec.InitContainers {
		acc[ns][c.Image] = 1
	}
}

func collectFromDeployments(
	ctx context.Context,
	client kubernetes.Interface,
	ns string,
	acc map[string]map[string]int,
) error {
	deploys, err := client.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, d := range deploys.Items {
		collectImages(d.Spec.Template.Spec, ns, acc)
	}
	return nil
}

func collectFromStatefulSets(
	ctx context.Context,
	client kubernetes.Interface,
	ns string,
	acc map[string]map[string]int,
) error {
	sets, err := client.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, s := range sets.Items {
		collectImages(s.Spec.Template.Spec, ns, acc)
	}
	return nil
}

func collectFromDaemonSets(
	ctx context.Context,
	client kubernetes.Interface,
	ns string,
	acc map[string]map[string]int,
) error {
	sets, err := client.AppsV1().DaemonSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, d := range sets.Items {
		collectImages(d.Spec.Template.Spec, ns, acc)
	}
	return nil
}

func normalizeSemVer(imageVer string, versionRe *regexp.Regexp) (string, bool) {
	m := versionRe.FindStringSubmatch(imageVer)
	if m == nil {
		return "", false
	}

	major := m[1]
	minor := m[2]
	patch := m[3]

	// set .0 as default patch version acc. to SemVer
	if patch == "" {
		patch = ".0"
	}

	return fmt.Sprintf("%s.%s%s", major, minor, patch), true
}

func sendDataToAPI(jsonData []byte) {
	apiURL := config.GetEnvConfig().API_URL
	apiToken := config.GetEnvConfig().API_TOKEN

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

func getClusterName() string {
	if envClusterName := os.Getenv("CLUSTER_NAME"); envClusterName != "" {
		log.Printf("Using cluster name from environment: %s", envClusterName)
		return envClusterName
	}

	log.Println("Cluster name not found, using default 'minikube'")
	return "minikube"
}

func getKubernetesVersion(clientset *kubernetes.Clientset) string {
	versionInfo, err := clientset.Discovery().ServerVersion()
	if err != nil {
		log.Println("Failed to fetch Kubernetes version, using 'unknown-version'")
		return "unknown-version"
	}
	return versionInfo.GitVersion
}
