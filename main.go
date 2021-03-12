package main

import (
	"context"
	"flag"
	"fmt"
	"sync"
	"time"

	tm "github.com/buger/goterm"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	internalStore "github.com/wwitzel3/client-go-scratch/internal/store"
	//
	// Uncomment to load all auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

// const resync = time.Second * 60

var gvrs = []schema.GroupVersionResource{
	{Group: "apiregistration.k8s.io", Version: "v1", Resource: "apiservices"},
	{Group: "apps", Version: "v1", Resource: "replicasets"},
	{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"},
	{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
	{Version: "v1", Resource: "pods"},
	{Version: "v1", Resource: "configmaps"},
	{Group: "batch", Version: "v1beta1", Resource: "cronjobs"},
	{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"},
	{Group: "apps", Version: "v1", Resource: "daemonsets"},
	{Group: "apps", Version: "v1", Resource: "deployments"},
	//{Group: "extensions", Version: "v1beta1", Resource: "deployments"},
	//{Group: "extensions", Version: "v1beta1", Resource: "replicasets"},
	{Version: "v1", Resource: "events"},
	{Group: "autoscaling", Version: "v1", Resource: "horizontalpodautoscalers"},
	{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
	{Group: "batch", Version: "v1", Resource: "jobs"},
	{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "mutatingwebhookconfigurations"},
	{Version: "v1", Resource: "nodes"},
	{Version: "v1", Resource: "namespaces"},
	{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"},
	{Version: "v1", Resource: "serviceaccounts"},
	{Version: "v1", Resource: "secrets"},
	{Version: "v1", Resource: "services"},
	{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "podmetrics"},
	{Version: "v1", Resource: "persistentvolumes"},
	{Version: "v1", Resource: "persistentvolumeclaims"},
	{Version: "v1", Resource: "replicationcontrollers"},
	{Group: "apps", Version: "v1", Resource: "statefulsets"},
	{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"},
	{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
	{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "validatingwebhookconfigurations"},
	{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"},
}

// type InformerWithStopCh struct {
// 	stopCh   chan struct{}
// 	informer informers.GenericInformer
// }

func main() {
	kubeconfig := flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	dynamic, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// resourceInformers := map[schema.GroupVersionResource]*InformerWithStopCh{}
	// removeCh := make(chan schema.GroupVersionResource)

	// go func() {
	// 	for {
	// 		gvr := <-removeCh
	// 		resourceInformers[gvr] = nil
	// 	}
	// }()

	//informerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamic, resync, "default", nil)
	// informerFactory := dynamicinformer.NewDynamicSharedInformerFactory(dynamic, resync)

	// for _, gvr := range gvrs {
	// 	informer := informerFactory.ForResource(gvr)
	// 	stopCh := make(chan struct{})
	// 	informer.Informer().SetWatchErrorHandler(watchErrorHandler(gvr, stopCh, removeCh))
	// 	resourceInformers[gvr] = &InformerWithStopCh{stopCh: stopCh, informer: informer}
	// }

	// for _, informer := range resourceInformers {
	// 	go informer.informer.Informer().Run(informer.stopCh)
	// 	cache.WaitForCacheSync(informer.stopCh, informer.informer.Informer().HasSynced)
	// }

	ctx := context.Background()
	dc, err := internalStore.NewDynamicCache(ctx, dynamic, "default")

	if err != nil {
		panic(err.Error())
	}

	for _, gvr := range gvrs {
		dc.Watch(gvr)
	}

	fmt.Println("started informers")

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		for i := 0; i < 10; i++ {
			tm.Clear()
			tm.MoveCursor(1, 1)
			tm.Println("Printing current resource list")
			tm.Println("Current Time:", time.Now().Format(time.RFC1123))
			for _, gvr := range gvrs {
				lister, err := dc.ListerForResource(gvr)
				if err != nil {
					tm.Println("unable to get lister", err.Error())
					continue
				}
				if gvr == gvrs[9] {
					objects, err := lister.List(labels.Everything())
					if err != nil {
						tm.Println("lister error", err.Error())
						continue
					}
					for _, obj := range objects {
						u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
						if err != nil {
							continue
						}
						name, found, err := unstructured.NestedString(u, "metadata", "name")
						if err != nil || !found {
							tm.Println(gvr, "error")
						} else {
							tm.Println(gvr, name)
						}
					}
				}
			}
			tm.Flush()
			time.Sleep(3 * time.Second)
		}
		wg.Done()
	}()

	wg.Wait()

	key := "default/nginx-deployment"

	fmt.Println("getting deployment by name default/nginx-deployment ..")
	lister, err := dc.ListerForResource(gvrs[9])
	if err != nil {
		fmt.Println(err.Error())
	}
	obj, err := lister.Get(key)

	if err != nil {
		fmt.Println(err.Error())
	} else {
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err == nil {
			fmt.Println(u)
		}
	}

	fmt.Println("terminating ...")
}
