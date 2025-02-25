package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	klog "k8s.io/klog/v2"

	"kubevirt.io/csi-driver/pkg/kubevirt"
	"kubevirt.io/csi-driver/pkg/service"
)

var (
	endpoint               = flag.String("endpoint", "unix:/csi/csi.sock", "CSI endpoint")
	namespace              = flag.String("namespace", "", "Namespace to run the controllers on")
	nodeName               = flag.String("node-name", "", "The node name - the node this pods runs on")
	infraClusterNamespace  = flag.String("infra-cluster-namespace", "", "The infra-cluster namespace")
	infraClusterKubeconfig = flag.String("infra-cluster-kubeconfig", "", "the infra-cluster kubeconfig file")
	infraClusterLabels     = flag.String("infra-cluster-labels", "", "The infra-cluster labels to use when creating resources in infra cluster. 'name=value' fields separated by a comma")
)

func init() {
	flag.Set("logtostderr", "true")
	// make glog and klog coexist
	klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(klogFlags)

	// Sync the glog and klog flags.
	flag.CommandLine.VisitAll(func(f1 *flag.Flag) {
		f2 := klogFlags.Lookup(f1.Name)
		if f2 != nil {
			value := f1.Value.String()
			f2.Value.Set(value)
		}
	})
}

func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())
	handle()
	os.Exit(0)
}

func handle() {
	if service.VendorVersion == "" {
		klog.Fatalf("VendorVersion must be set at compile time")
	}
	klog.V(2).Infof("Driver vendor %v %v", service.VendorName, service.VendorVersion)

	tenantConfig, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatalf("Failed to build tenant cluster config: %v", err)
	}

	tenantClientSet, err := kubernetes.NewForConfig(tenantConfig)
	if err != nil {
		klog.Fatalf("Failed to build tenant client set: %v", err)
	}

	infraClusterConfig, err := clientcmd.BuildConfigFromFlags("", *infraClusterKubeconfig)
	if err != nil {
		klog.Fatalf("Failed to build infra cluster config: %v", err)
	}

	virtClient, err := kubevirt.NewClient(infraClusterConfig)
	if err != nil {
		klog.Fatal(err)
	}

	var nodeID string
	if *nodeName != "" {
		node, err := tenantClientSet.CoreV1().Nodes().Get(context.TODO(), *nodeName, v1.GetOptions{})
		if err != nil {
			klog.Fatal(fmt.Errorf("failed to find node by name %v: %v", nodeName, err))
		}
		// systemUUID is the VM ID
		nodeID = node.Status.NodeInfo.SystemUUID
		klog.Infof("Node name: %v, Node ID: %s", nodeName, nodeID)
	}

	infraClusterLabelsMap := parseLabels()

	driver := service.NewKubevirtCSIDriver(virtClient, *infraClusterNamespace, infraClusterLabelsMap, nodeID)

	driver.Run(*endpoint)
}

func parseLabels() map[string]string {

	infraClusterLabelsMap := map[string]string{}

	if *infraClusterLabels == "" {
		return infraClusterLabelsMap
	}

	labelStrings := strings.Split(*infraClusterLabels, ",")

	for _, label := range labelStrings {

		labelPair := strings.SplitN(label, "=", 2)

		if len(labelPair) != 2 {
			panic("Bad labels format. Should be 'key=value,key=value,...'")
		}

		infraClusterLabelsMap[labelPair[0]] = labelPair[1]
	}

	return infraClusterLabelsMap
}
