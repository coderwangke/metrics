package main

import (
	"flag"
	"fmt"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/labels"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
	metricsapi "k8s.io/metrics/pkg/apis/metrics"
	metricsv1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var supportedMetricsAPIVersions = []string{
	"v1beta1",
}

type K9sClient struct {
	Client         *kubernetes.Clientset
	PodClient      corev1client.PodsGetter
	MetricsClient   metricsclientset.Interface
}

func main() {
	var kubeconfig *string
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	metricsset, err := metricsclientset.NewForConfig(config)

	k := &K9sClient{
		Client:    clientset,
		PodClient: clientset.CoreV1(),
		MetricsClient: metricsset,
	}

	items, err := k.getMetricsItems()
	if err != nil {
		panic(err.Error())
	}
	for _, m := range items.Items {
		var cpu, mem int64
		for _, c := range m.Containers {
			cpu += c.Usage.Cpu().MilliValue()
			mem += c.Usage.Memory().Value() / (1024 * 1024)
		}
		fmt.Printf("name: %s, cpu: %dm, mem: %dMi\n", m.Name, cpu, mem)
	}
}

//func dialOrDie() kubernetes.Interface {
//	var client kubernetes.Interface
//	var err error
//
//	if client, err = kubernetes.NewForConfig(nil); err != nil {
//		panic(err)
//	}
//	return client
//}
//
func (k *K9sClient) supportMetrics() bool {
	apiGroups, err := k.Client.Discovery().ServerGroups()
	if err != nil {
		return false
	}

	for _, discoveredAPIGroup := range apiGroups.Groups {
		if discoveredAPIGroup.Name != metricsapi.GroupName {
			continue
		}
		for _, version := range discoveredAPIGroup.Versions {
			for _, supportedVersion := range supportedMetricsAPIVersions {
				if version.Version == supportedVersion {
					return true
				}
			}
		}
	}
	return false
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func (k *K9sClient) getMetricsItems() (*metricsapi.PodMetricsList, error) {
	selector := labels.Everything()
	metrics, err := getMetricsFromMetricsAPI(k.MetricsClient, "default", "", false, selector)

	if err != nil {
		fmt.Println(err.Error())
		return nil, errors.WithStack(err)
	}

	return metrics, nil
}

func getMetricsFromMetricsAPI(metricsClient metricsclientset.Interface, namespace, resourceName string, allNamespaces bool, selector labels.Selector) (*metricsapi.PodMetricsList, error) {
	var err error
	ns := metav1.NamespaceAll
	if !allNamespaces {
		ns = namespace
	}
	versionedMetrics := &metricsv1beta1api.PodMetricsList{}
	if resourceName != "" {
		m, err := metricsClient.MetricsV1beta1().PodMetricses(ns).Get(resourceName, metav1.GetOptions{})
		if err != nil {
			return nil, errors.WithStack(err)
		}
		versionedMetrics.Items = []metricsv1beta1api.PodMetrics{*m}
	} else {
		versionedMetrics, err = metricsClient.MetricsV1beta1().PodMetricses(ns).List(metav1.ListOptions{LabelSelector: selector.String()})
		if err != nil {
			fmt.Println(err.Error())
			return nil, errors.WithStack(err)
		}
	}
	metrics := &metricsapi.PodMetricsList{}
	err = metricsv1beta1api.Convert_v1beta1_PodMetricsList_To_metrics_PodMetricsList(versionedMetrics, metrics, nil)
	if err != nil {
		fmt.Println(err.Error())
		return nil, errors.WithStack(err)
	}
	return metrics, nil
}