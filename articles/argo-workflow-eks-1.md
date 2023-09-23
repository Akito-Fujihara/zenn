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


