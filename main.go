package main

import (
	"fmt"
	"log"
	"strings"

	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			log.Println("panic occurred: but we survived", err)
		}
	}()

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}
	sugar := logger.Sugar()
	config, _ := rest.InClusterConfig()
	clientset, _ := kubernetes.NewForConfig(config)

	factory := informers.NewSharedInformerFactory(clientset, 0)
	informer := factory.Core().V1().Pods().Informer()
	stopper := make(chan struct{})
	defer close(stopper)

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if obj.(*v1.Pod) != nil {
				pod := obj.(*v1.Pod)
				if pod.Name != "" && pod.Namespace != "" {
					sugar.Info("New pod added ", pod.Name, "in Namespace", pod.Namespace)
					str := "New pod added Name: " + pod.Name + ". in Namespace: " + pod.Namespace
					if strings.Contains(pod.Name, "coredns") || strings.Contains(pod.Name, "kindnet") || strings.Contains(pod.Name, "kube-controller-manager") || strings.Contains(pod.Name, "kube-scheduler") || strings.Contains(pod.Name, "local-path-provisioner") || strings.Contains(pod.Name, "etcd") || strings.Contains(pod.Name, "proxy") || strings.Contains(pod.Name, "control-planein") || strings.Contains(pod.Name, "kube") {
						sugar.Info("Not sending Slack notif")
					} else {
						SendSlackMsg(str)
					}
				}
			}
		},

		UpdateFunc: func(old interface{}, new interface{}) {
			oldPod := old.(*v1.Pod)
			//newPod := new.(*v1.Pod)
			defer func() {
				if err := recover(); err != nil {
					log.Println("panic occurred in upadate:", err)
				}
			}()
			if oldPod.Status.ContainerStatuses[0].State.Terminated != nil {
				sugar.Info("\n old state ContainerStatuses Terminated Reason ", oldPod.Status.ContainerStatuses[0].State.Terminated.Reason, "Messgae", oldPod.Status.ContainerStatuses[0].State.Terminated.Message)

				//sending slack message
				sugar.Info("sending slack notif")
				str := "The following pod is Terminated Name: " + oldPod.Name + ". Reason: " + oldPod.Status.ContainerStatuses[0].State.Terminated.Reason
				SendSlackMsg(str)
			}
		},
	})

	go informer.Run(stopper)
	if !cache.WaitForCacheSync(stopper, informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}
	<-stopper

}
