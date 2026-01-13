package main

import (
	"context"
	"fmt"
	"log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	ctx := context.Background()

	// in-cluster config
	config, err := clientcmd.BuildConfigFromFlags("", "/home/iops/.kube/smen-dev-eks.cfg")
	//config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("failed to get cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
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

	fmt.Println("Unique images:")
	for img := range images {
		fmt.Println(img)
	}
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
