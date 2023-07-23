---
title: "Custom Resource Definitions(CRD)とは？"
emoji: "✨"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: ["kubernetes", "CRD"]
published: true
---

# 自己紹介
都内SaaS 企業でSRE エンジニア ２年目のFujihara Akitoです。
週一程度簡単なmemo書を残したいと思っています。

# Custom Resource Definitions(CRD)とは？
[Custom Resource](https://kubernetes.io/ja/docs/concepts/extend-kubernetes/api-extension/custom-resources) とは Kubernetes APIの拡張機能です。
Kubernetes APIは特定の種類のオブジェクト コレクションを保管するエンドポイントです。 例えばDeployment, Podなどのリソースのオブジェクト コレクションなどのが含まれます。
Custom ResourceはKubernetes APIの拡張し、独自のAPIを導入することを可能にするオブジェクトです。

もっと簡単に言ってしまうと、`kubectl create pod ・・・` のように独自で作成できるresourceのことです。

[Custom Resource Definitions](https://kubernetes.io/ja/docs/concepts/extend-kubernetes/api-extension/custom-resources/#customresourcedefinition) とは Kubernetes APIを拡張して独自のリソース(CR)を定義することができるものです。
Kubernetes Clusterに対してCRDオブジェクトを定義することで、指定した名前、スキーマで新しいカスタムリソースが作成されます。
CRD自体はKubernetesのオブジェクトの一種で他のリソースと同様にyamlなどで作成することが可能です。

# どんな仕組みでCustom Resourceは作られる?
## CRが作られる流れ
![](/images/k8s-custom-resource-definitions/crd-controller-architecture.drawio.png)

1. CRD & Custom Controllerを作成
  - CRD & Custom Controllerの作成をapi serverにリクエストする
  - CRDは定義するとapi serverにCRのオブジェクトが作成される
    -  CRのオブジェクトが作成される=`kind: CR name`のような独自のAPIが作成される
  - Custom Controller(kind: deployment)が作成される
    - CRの定義で必要なリソースの作成・削除などを行う。例えばCRでPodやSecretなど必要な場合はapi serverにリクエストして作成。
    - CRの定義を検知して理想状態を維持する(Reconciliation Loop)。k8s controller-managerと役割がかなり似ている。

## Custom Controllerの役割を理解するためにk8s controller-managerの役割を理解
自分のmemo書記事よりちゃんと理解したいならそもそもこちらのスライドがとても分かりやすい...
[ゼロから始めるKubernetes Controller / Under the Kubernetes Controller(ReplicaSetをApply~ デリバリされるまで)](https://speakerdeck.com/govargo/under-the-kubernetes-controller-36f9b71b-9781-4846-9625-23c31da93014?slide=18)
Controllerが話の主旨ではありませんがControllerの役割が理解しやすかったと思います。

# CRDを作成してみる

### crd作成
crd.yaml
```
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: sample.crds.example.com
spec:
  group: crds.example.com
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                crd-text:
                  type: string
  scope: Namespaced
  names:
    plural: sample
    singular: sample
    kind: Sample
```
作成
```
❯ k apply -f crd.yaml
customresourcedefinition.apiextensions.k8s.io/sample.crds.example.com created
❯ k get crd
NAME                      CREATED AT
sample.crds.example.com   2023-07-22T15:37:22Z
```

### CR作成
sample-cr.yaml
```
kind: Sample
metadata:
  name: sample-custom-resource
spec:
  crd-text: "Create Sample Custom Resource!" 
```
作成
```
❯ k apply -f sample-cr.yaml
sample.crds.example.com/sample-custom-resource created
❯ k get sample
NAME                     AGE
sample-custom-resource   6s
```

CRD & CRの作成をしてみました。
Custom Controllerが存在しないので定義されたオブジェクトが存在するだけになっています。
CRD & CRの作成までしたので、`kind: Sample`のオブジェクトがapi serverに存在しており、CRがetcdに保存されています。
etcdを確認すると

```
❯ k exec -it etcd-kind-control-plane sh
kubectl exec [POD] [COMMAND] is DEPRECATED and will be removed in a future version. Use kubectl exec [POD] -- [COMMAND] instead.

sh-5.1# echo /etc/kubernetes/pki/etcd/*
/etc/kubernetes/pki/etcd/ca.crt /etc/kubernetes/pki/etcd/ca.key /etc/kubernetes/pki/etcd/healthcheck-client.crt /etc/kubernetes/pki/etcd/healthcheck-client.key /etc/kubernetes/pki/etcd/peer.crt /etc/kubernetes/pki/etcd/peer.key /etc/kubernetes/pki/etcd/server.crt /etc/kubernetes/pki/etcd/server.key

sh-5.1# export ETCDCTL_API=3
sh-5.1# export ETCDCTL_CACERT=/etc/kubernetes/pki/etcd/ca.crt
sh-5.1# export ETCDCTL_CERT=/etc/kubernetes/pki/etcd/server.crt
sh-5.1# export ETCDCTL_KEY=/etc/kubernetes/pki/etcd/server.key
sh-5.1# etcdctl get "" --prefix --keys-onlyeys-only

sh-5.1# etcdctl get "" --prefix --keys-only
/registry/apiextensions.k8s.io/customresourcedefinitions/sample.crds.example.com
・・・
/registry/crds.example.com/sample/default/sample-custom-resource
・・・
```
CRD & CRの保存までされていることが確認できます。

また、実際にCustom ControllerとCRDの作成には
- Kubenetes Way
- Kubebuilder
- Operator SDK

などのフレームワークが用いられます。
