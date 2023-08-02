---
title: "client-goさわってみた"
emoji: "✨"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: ["kubernetes", "clientgo"]
published: true
---

# 自己紹介
都内SaaS 企業でSRE エンジニア ２年目のFujihara Akitoです。
週一程度簡単なmemo書を残したいと思っています。

# client-go とは？
client-go とは k8aとやりとりするgoのpackageで、k8s api-serverへのアクセスに利用されます。
![](/images/k8s-custom-resource-definitions/client-go-controller-interaction.jpeg)
https://github.com/kubernetes/sample-controller/blob/f8d330e5629b5a03fc2257796d1ce2f8d939b80a/docs/images/client-go-controller-interaction.jpeg

## client-goを構成する機能
1. Reflector: API serverからリソースの変更を検知して、DeltaFIFO queueに反映させる
2. Informer: API serverからリソースの変更を検知して、Objectの変更を監視する
3. Indexer: local cacheを保持しており、Controller内で使うListerはこのcacheから取り出す
4. ClientSet: API serverとのやりとりをする

## clientsetからpodを取得してみる

podlists.go
```
package main

import (
	"context"
	"fmt"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	defaultKubeConfigPath := filepath.Join(homedir.HomeDir(), ".kube", "config")

	config, _ := clientcmd.BuildConfigFromFlags("", defaultKubeConfigPath)

	clientset, _ := kubernetes.NewForConfig(config)

	pods, _ := clientset.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{})

	fmt.Println("NAMESPACE\tNAME")
	for _, pod := range pods.Items {
		fmt.Printf("%s\t%s\n", pod.GetNamespace(), pod.GetName())
	}
}
```

command
```
❯ kind create cluster

❯ go run ./podlist.go
NAMESPACE       NAME
kube-system     coredns-787d4945fb-4j8h2
kube-system     coredns-787d4945fb-h4qwt
kube-system     etcd-kind-control-plane
kube-system     kindnet-7424d
kube-system     kube-apiserver-kind-control-plane
kube-system     kube-controller-manager-kind-control-plane
kube-system     kube-proxy-fq44p
kube-system     kube-scheduler-kind-control-plane
local-path-storage      local-path-provisioner-75f5b54ffd-blg54
```

## informerでpodの変更を監視してみる
informer.go
```
package main

import (
	"log"
	"path/filepath"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	KubeConfigPath := filepath.Join(homedir.HomeDir(), ".kube", "config")

	config, _ := clientcmd.BuildConfigFromFlags("", KubeConfigPath)

	clientset, _ := kubernetes.NewForConfig(config)

	informerFactory := informers.NewSharedInformerFactory(clientset, time.Second*30)

	podInformer := informerFactory.Core().V1().Pods()

	podInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    AddFuncPod,
			UpdateFunc: UpdateFuncPod,
			DeleteFunc: DeleteFuncPod,
		},
	)

	StopCh := make(chan struct{})
	informerFactory.Start(wait.NeverStop)

	go wait.Until(func() {
		log.Println("------- running --------")
	}, time.Second*10, StopCh)
	<-StopCh
}

func AddFuncPod(obj interface{}) {
	if pod_name, err := cache.MetaNamespaceKeyFunc(obj); err != nil {
		log.Printf("Error Get Pod Obj: %s\n", err.Error())
	} else {
		log.Println("AddFunc Pod:", pod_name)
	}
}

func UpdateFuncPod(old, new interface{}) {
	if pod_name, err := cache.MetaNamespaceKeyFunc(new); err != nil {
		log.Printf("Error Get Pod Obj: %s\n", err.Error())
	} else {
		log.Println("UpdateFunc Pod:", pod_name)
	}
}

func DeleteFuncPod(obj interface{}) {
	if pod_name, err := cache.MetaNamespaceKeyFunc(obj); err != nil {
		log.Printf("Error Get Pod Obj: %s\n", err.Error())
	} else {
		log.Println("DeleteFunc Pod:", pod_name)
	}
}
```

command
```
❯ kind create cluster

❯ go run ./informer.go
2023/08/03 01:49:39 ------- running --------
2023/08/03 01:49:39 AddFunc Pod: kube-system/etcd-kind-control-plane
2023/08/03 01:49:39 AddFunc Pod: kube-system/kube-controller-manager-kind-control-plane
2023/08/03 01:49:39 AddFunc Pod: kube-system/kube-scheduler-kind-control-plane
2023/08/03 01:49:39 AddFunc Pod: local-path-storage/local-path-provisioner-75f5b54ffd-blg54
2023/08/03 01:49:39 AddFunc Pod: kube-system/kube-apiserver-kind-control-plane
2023/08/03 01:49:39 AddFunc Pod: kube-system/kindnet-7424d
2023/08/03 01:49:39 AddFunc Pod: kube-system/kube-proxy-fq44p
2023/08/03 01:49:39 AddFunc Pod: kube-system/coredns-787d4945fb-h4qwt
2023/08/03 01:49:39 AddFunc Pod: kube-system/coredns-787d4945fb-4j8h2
2023/08/03 01:49:49 ------- running --------
2023/08/03 01:49:59 ------- running --------
2023/08/03 01:50:09 ------- running --------
2023/08/03 01:50:09 UpdateFunc Pod: kube-system/kube-controller-manager-kind-control-plane
2023/08/03 01:50:09 UpdateFunc Pod: kube-system/kube-scheduler-kind-control-plane
2023/08/03 01:50:09 UpdateFunc Pod: local-path-storage/local-path-provisioner-75f5b54ffd-blg54
2023/08/03 01:50:09 UpdateFunc Pod: kube-system/kindnet-7424d
2023/08/03 01:50:09 UpdateFunc Pod: kube-system/coredns-787d4945fb-h4qwt
2023/08/03 01:50:09 UpdateFunc Pod: kube-system/coredns-787d4945fb-4j8h2
2023/08/03 01:50:09 UpdateFunc Pod: kube-system/etcd-kind-control-plane
2023/08/03 01:50:09 UpdateFunc Pod: kube-system/kube-apiserver-kind-control-plane
2023/08/03 01:50:09 UpdateFunc Pod: kube-system/kube-proxy-fq44p
2023/08/03 01:50:19 ------- running --------

・・・・・・

2023/08/03 01:53:49 ------- running --------
2023/08/03 01:53:56 AddFunc Pod: default/nginx

・・・・

2023/08/03 01:55:10 DeleteFunc Pod: default/nginx
2023/08/03 01:55:19 ------- running --------
```

別でcommandを実行
```
❯ kubectl run nginx --image=nginx
pod/nginx created
❯ kubectl delete pod nginx
pod "nginx" deleted
```

## 参考
- https://uzimihsr.github.io/post/2020-09-30-kubernetes-client-go-watch-pods/
- https://github.com/nakamasato/kubernetes-operator-basics
- https://github.com/Akito-Fujihara/client-go
