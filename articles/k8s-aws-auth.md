---
title: "terraform ハンズオン EKS 認証認可①(aws-auth)"
emoji: "✨"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: ["kubernetes", "aws-auth"]
published: false
---

# 自己紹介
都内SaaS 企業でSRE エンジニア ２年目のFujihara Akitoです。
週一程度簡単なmemo書を残したいと思っています。

# EKSの認証認可
EKSの認証認可の方法には大きく2種類あります。
- aws-auth
- IAM Roles for Service Accounts (IRSA)
今回の記事ではまずaws-iam-authenticatorについて説明します。

# EKSのaws-authによる認証認可
IAMのエンティティとKubernetesのUserAccount/Groupを紐づけを定義をaws-auth(kind: ConfigMap)で行う方法です。
2019年ごろまではaws-iam-authenticatorが必要だったらしいのですが、今はEKS API の GetToken API を利用することで kubectl & awscliで実行出来るようになったらしいです。([参考](https://dev.classmethod.jp/articles/eks-update-get-token-cmd/))

実際にコンソール上からkubectl commandを実行して出力されるまでのフローは以下の図です。




# 参考文献
- https://yomon.hatenablog.com/entry/2020/11/eks_system_masters
- https://katainaka0503.hatenablog.com/entry/2019/12/07/091737#Kubernetes%E3%81%AE%E8%AA%8D%E8%A8%BC%E8%AA%8D%E5%8F%AFAdmissionControl
- https://zenn.dev/take4s5i/articles/aws-eks-authentication
- https://44smkn.hatenadiary.com/entry/2021/02/06/185748
- https://docs.aws.amazon.com/eks/latest/userguide/add-user-role.html
