---
title: "controller-runtimeさわってみた"
emoji: "✨"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: ["kubernetes", "controllerruntime"]
published: true
---

# 自己紹介
都内SaaS 企業でSRE エンジニア ２年目のFujihara Akitoです。
週一程度簡単なmemo書を残したいと思っています。

# controller-runtimeとは？
controller-runtimeとは、Kubernetesが標準で提供している(client-go)[https://github.com/kubernetes/client-go], (apimachinery)[https://github.com/kubernetes/apimachinery], (api)[https://github.com/kubernetes/api]などのパッケージを抽象化・隠蔽し、より簡単にカスタムコントローラーを実装可能にしたライブラリ.

![](/images/k8s-custom-resource-definitions/kubebuilder-architecture.webp)
https://book.kubebuilder.io/architecture.html から引用

## 重要なcomponent

- Manager
    - 複数のcontrollerを起動&管理するComponent. また、Custom Controller を実行するためのhelthcheck, leaderElection, metricsなどの機能も提供している.

- Client & Cache
    - k8s api serverとやりとりするためのComponent. また、監視対象のリソースをインメモリにキャッシュする機能などをもつ. 内部で利用されているclient-goの Client & Informerの実装例は「[client-goさわってみた](https://zenn.dev/fujihara_akito/articles/use-client-go)」を参照.

- Controller
    -  定義したReconciler関数の内容を制御ループで実行するComponent. Watch関数からSourceを実行して対象ResourceのQueueを書き込み、Start関数でQueueに対してReconcileを実行する

- Reconciler
    -  Controllerの一部であり、k8s operatorにおいて重要なReconciliation Loopの役割を担うComponent. Controllerの役割については「[Custom Resource Definitions(CRD)とは？](https://zenn.dev/fujihara_akito/articles/k8s-custom-resource-definitions)」を参照.

# controller-runtimeでpodのcontrollerを作成

main.go
```
package main

import (
	"context"
	"flag"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = ctrl.Log.WithName("Maneger")

func main() {
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	cfg, _ := config.GetConfig()
	mgr, _ := manager.New(cfg, manager.Options{})

	Reconciler := reconcile.Func(Reconciler)

	ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(Reconciler)

	ctx := context.Background()
	if err := mgr.Start(ctx); err != nil {
		log.Error(err, "can't start manager")
	}
}

func Reconciler(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log.Info("Reconciler is called", "Pod", req.Name)
	return reconcile.Result{}, nil
}
```

command
```
❯ kind create cluster
・・・・

❯ go run ./main.go
2023-08-05T23:49:09+09:00	INFO	controller-runtime.metrics	Metrics server is starting to listen	{"addr": ":8080"}
2023-08-05T23:49:09+09:00	INFO	starting server	{"path": "/metrics", "kind": "metrics", "addr": "[::]:8080"}
2023-08-05T23:49:09+09:00	INFO	Starting EventSource	{"controller": "pod", "controllerGroup": "", "controllerKind": "Pod", "source": "kind source: *v1.Pod"}
2023-08-05T23:49:09+09:00	INFO	Starting Controller	{"controller": "pod", "controllerGroup": "", "controllerKind": "Pod"}
2023-08-05T23:49:09+09:00	INFO	Starting workers	{"controller": "pod", "controllerGroup": "", "controllerKind": "Pod", "worker count": 1}
2023-08-05T23:49:09+09:00	INFO	Maneger	Reconciler is called	{"Pod": "etcd-kind-control-plane"}
2023-08-05T23:49:09+09:00	INFO	Maneger	Reconciler is called	{"Pod": "kube-controller-manager-kind-control-plane"}
2023-08-05T23:49:09+09:00	INFO	Maneger	Reconciler is called	{"Pod": "kube-scheduler-kind-control-plane"}
2023-08-05T23:49:09+09:00	INFO	Maneger	Reconciler is called	{"Pod": "local-path-provisioner-75f5b54ffd-blg54"}
2023-08-05T23:49:09+09:00	INFO	Maneger	Reconciler is called	{"Pod": "kube-apiserver-kind-control-plane"}
2023-08-05T23:49:09+09:00	INFO	Maneger	Reconciler is called	{"Pod": "kindnet-7424d"}
2023-08-05T23:49:09+09:00	INFO	Maneger	Reconciler is called	{"Pod": "kube-proxy-fq44p"}
2023-08-05T23:49:09+09:00	INFO	Maneger	Reconciler is called	{"Pod": "coredns-787d4945fb-h4qwt"}
2023-08-05T23:49:09+09:00	INFO	Maneger	Reconciler is called	{"Pod": "coredns-787d4945fb-4j8h2"}
2023-08-05T23:50:50+09:00	INFO	Maneger	Reconciler is called	{"Pod": "nginx"}
2023-08-05T23:50:50+09:00	INFO	Maneger	Reconciler is called	{"Pod": "nginx"}
2023-08-05T23:50:50+09:00	INFO	Maneger	Reconciler is called	{"Pod": "nginx"}
2023-08-05T23:50:53+09:00	INFO	Maneger	Reconciler is called	{"Pod": "nginx"}
2023-08-05T23:51:15+09:00	INFO	Maneger	Reconciler is called	{"Pod": "nginx"}
2023-08-05T23:51:16+09:00	INFO	Maneger	Reconciler is called	{"Pod": "nginx"}
2023-08-05T23:51:16+09:00	INFO	Maneger	Reconciler is called	{"Pod": "nginx"}
2023-08-05T23:51:16+09:00	INFO	Maneger	Reconciler is called	{"Pod": "nginx"}
```

別でcommandを実行
```
❯ kubectl run nginx --image=nginx
pod/nginx created
❯ kubectl delete pod nginx
pod "nginx" deleted
```

