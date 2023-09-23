---
title: "EKSで始めるArgo Workflows"
emoji: "✨"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: ["kubernetes", "eks", "argoworkflows"]
published: false
---

# 自己紹介
都内SaaS 企業でSRE エンジニア ２年目のFujihara Akitoです。
週一程度簡単なmemo書を残したいと思っています。

# Argo Workflowsとは？
ArgoWorkflows とは kubernetes上で実行されるコンテナネイティブなワークフローエンジンであり、ジョブの実行順序の制御や並列実行などを柔軟に実行させることができます。
また、リッチなGUIがあるのも特徴です。

例えば、ジョブAを実行した後にジョブB, Cを実行し、BとCのジョブを待ってからジョブDを実行するというようなオーケストレーションを簡単に行うことができます。
![](/images/argo-workflows/sample-workflow-1.png)

また、https://argoproj.github.io/argo-workflows/architecture/ を参考に簡単にできそうなことを見てみます。
![](/images/argo-workflows/argo-workflows-architecture.png)

できそうなこと
- webhookによるworkflowの発火など
- prometheus metrics の取得
- DBにworkflowの実行結果を保存
- S3などのストレージにworkflowのログ保管やジョブ間のfile共有
- OAuth Providerを活用した認証・認可

Argo Workflowsのコンポーネント
- Argo Server: Argo Workflows の API と UI を公開するサーバーで、ssoなどの認証なども行う
- Workflow Controller: Argo Workflows のカスタムコントローラー

# Argo WorkflowsをEKSで試してみる

https://github.com/Akito-Fujihara/integration-argoworkflows
にterraformをmanifestを載せています。

### terraoformでaws resourceの作成

変更点
- https://github.com/Akito-Fujihara/integration-argoworkflows/blob/add3e972cb5ecbb393074a83c33a4b0f4deffc41/terraform/sg.tf#L13 の部分を `cidr_blocks = ["0.0.0.0/0"]` に置き換える
- aws-authの設定でiam userを利用している場合には [aws-authについて](https://zenn.dev/fujihara_akito/articles/k8s-aws-auth) & [参考code](https://github.com/Akito-Fujihara/aws-auth-and-irsa/blob/4d07dfd51fbae7f753955180b06dd60101b22775/eks.tf#L26-L32) を参考に設定を変更して `terraform apply` してください

### Argo Workflows の作成

- manifest管理にはhelm & helmfile を使用しています。
- Argo Workflows の設定
    - 認証モードをserver
    - Argo Server の Service を NodePort に設定 
[argoworkflows.yaml](https://github.com/Akito-Fujihara/integration-argoworkflows/blob/main/kubernetes/argoworkflows.yaml)
```
server:
  serviceType: NodePort
  serviceNodePort: 30000
  extraArgs:
    - --auth-mode=server
```
- `helmfile apply -f helmfile.yaml` で manifestを作成してください

sampleのworkflowも一緒に作成されるのでhello-workflow(上記の画像)も実行されると思います。
