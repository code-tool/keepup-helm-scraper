package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"keepup-helm-scrapper/src/config"
	"keepup-helm-scrapper/src/rules"
	"log"
	"maps"
	"net/http"
	"os"
	"regexp"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
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

	kubeconfig, err := clientcmd.BuildConfigFromFlags("", "/home/iops/.kube/smen-dev-eks.cfg")
	//config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("failed to get cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		log.Fatalf("failed to create clientset: %v", err)
	}

	images := map[string]struct{}{}

	namespaces, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatalf("failed to list namespaces: %v", err)
	}

	for _, ns := range namespaces.Items {
		namespace := ns.Name

		collectDeployments(ctx, clientset, namespace, images)
		collectStatefulSets(ctx, clientset, namespace, images)
		collectDaemonSets(ctx, clientset, namespace, images)
	}

	/*fmt.Println("Unique images:")
	for img := range images {
		fmt.Println(img)
	}*/

	rules, err := rules.LoadRules(config.GetEnvConfig().RULES_FILE)
	if err != nil {
		log.Fatalf("Can't configure RULES_FILE: %v", err)
	}

	var versionRe = regexp.MustCompile(`(\d+)\.(\d+)(\.\d+)?`)

	fmt.Println("Matched images:")
	theSortedSliceOfKeys := slices.Sorted(maps.Keys(images))
	var imagesInstalled []HelmChartInfo
	for _, img := range theSortedSliceOfKeys {
		for _, rule := range rules {
			if rule.DetectionRegex.MatchString(img) {
				fmt.Printf("Matched %s → %s\n", img, rule.ApplicationName)

				fmt.Println("str str", rule.VersionRegex.FindString(img))
				if v, ok := normalizeSemVer(rule.VersionRegex.FindString(img), versionRe); ok {
					fmt.Printf("%-90s → %s\n", img, v)
					imagesInstalled = append(imagesInstalled, HelmChartInfo{
						ChartName: rule.ApplicationName,
						Version:   v,
						Namespace: "***masked***",
					})
				} else {
					fmt.Printf("%-90s → no version\n", img)
				}

				break
			}
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

	sendDataToAPI(jsonData)
	//fmt.Println(imagesInstalled)
}

func collectDeployments(ctx context.Context, cs *kubernetes.Clientset, ns string, images map[string]struct{}) {
	list, _ := cs.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	for _, d := range list.Items {
		collectPodSpec(d.Spec.Template.Spec, images)
	}
}

func collectStatefulSets(ctx context.Context, cs *kubernetes.Clientset, ns string, images map[string]struct{}) {
	list, _ := cs.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	for _, s := range list.Items {
		collectPodSpec(s.Spec.Template.Spec, images)
	}
}

func collectDaemonSets(ctx context.Context, cs *kubernetes.Clientset, ns string, images map[string]struct{}) {
	list, _ := cs.AppsV1().DaemonSets(ns).List(ctx, metav1.ListOptions{})
	for _, d := range list.Items {
		collectPodSpec(d.Spec.Template.Spec, images)
	}
}

func collectPodSpec(spec corev1.PodSpec, images map[string]struct{}) {
	for _, c := range spec.Containers {
		images[c.Image] = struct{}{}
	}
	for _, c := range spec.InitContainers {
		images[c.Image] = struct{}{}
	}
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
