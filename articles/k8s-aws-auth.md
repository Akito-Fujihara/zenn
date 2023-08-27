---
title: "terraform ハンズオン EKS 認証認可①(aws-auth)"
emoji: "✨"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: ["kubernetes", "awsauth"]
published: true
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
IAMのエンティティとKubernetesのUserAccount/Groupを紐づけを定義をaws-auth(kind: ConfigMap)で行う方法で、Webhook Token Authenticationという方式を利用しています。
2019年ごろまではaws-iam-authenticatorが必要だったらしいのですが、今はEKS API の GetToken API を利用することで kubectl & awscliで実行出来るようになったらしいです。([参考](https://dev.classmethod.jp/articles/eks-update-get-token-cmd/))

実際にコンソール上からkubectl commandを実行して出力されるまでのフローは以下の図です。
![](/images/k8s-custom-resource-definitions/k8s-aws-iam-authenticator.drawio.png)

## (準備)EKS clusterにaws-authを作成
1. awscli & terraform の install
2. [IAM Userの作成](https://docs.aws.amazon.com/ja_jp/IAM/latest/UserGuide/id_users_create.html) & [awscliの設定](https://docs.aws.amazon.com/ja_jp/cli/latest/userguide/cli-configure-role.html)
3. 環境変数にuserarn & usernameを設定
```
export TF_ENV_userarn=arn:aws:iam::000000000000:user/hogehoge
export TF_ENV_username=hogehoge
```
4. github clone & terraform apply
```
git clone https://github.com/Akito-Fujihara/aws-auth-and-irsa.git
terraform init
terraform apply
```
5. kubeconfigを設定
```
aws eks update-kubeconfig --name eks-aws-infra-and-irsa
```

## ①IAM Identity tokenの取得
上記の手順通り設定すると

kube/config
```
kind: Config
preferences: {}
users:
- name: arn:aws:eks:ap-northeast-1:000000000000:cluster/eks-aws-infra-and-irsa
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1alpha1
      args:
      - --region
      - ap-northeast-1
      - eks
      - get-token
      - --cluster-name
      - eks-aws-infra-and-irsa
      command: aws
```
ここでcommandを実行すると
```
❯ aws eks get-token --cluster-name eks-aws-infra-and-irsa
{
    "kind": "ExecCredential",
    "apiVersion": "client.authentication.k8s.io/v1beta1",
    "spec": {},
    "status": {
        "expirationTimestamp": "2023-08-27T05:08:28Z",
        "token": "k8s-aws-v1.HOGEHOGE"
    }
}
echo HOGEHOGE | base64 -d
https://sts.ap-northeast-1.amazonaws.com/?Action=GetCallerIdentity&・・・
```
のように署名付きURLを取得したのちに IAMエンティティの情報を取得します

## ② Token & API Request & ③ Webhookによる認証
①で取得したIAMユーザのトークンを元にWebhookにTokenReviewのPOSTを実行します

https://yomon.hatenablog.com/entry/2020/11/eks_system_masters
ハンズオン形式で実行できるようになっているので実際に作った環境で実装すると良さそうです。

## ④ Kubernetes User情報の取得
実際に渡されたトークンをもとに正しいIAM userであるかどうか、aws-authを確認してどのgroupに所属しているかの情報を取得します

```
# 今回はuserにsystem:masterが紐づいていることがわかる
❯ k get cm aws-auth -o yaml
apiVersion: v1
data:
・・・
  mapUsers: |
    - "groups":
      - "system:masters"
      "userarn": "arn:aws:iam::000000000000:user/hogehoge"
      "username": "hogehoge"
・・・
```

TokenReviewのレスポンス例
```
{
  "kind": "TokenReview",
  "apiVersion": "authentication.k8s.io/v1",
  "metadata": {
    "creationTimestamp": null
  },
  "spec": {
    "token": "k8s-aws-v1....."
  },
  "status": {
    "authenticated": true,
    "user": {
      "username": "kubernetes-admin",
      "uid": "heptio-authenticator-aws:000000000000AIDxxxxx",
      "groups": [
        "system:masters"
      ],
      "extra": {
        "accessKeyId": [
  "AKIA......"
]
      }
    },
    "audiences": [
      "https://kubernetes.default.svc"
    ]
  }
}
```

## ⑤ Kubernetes Userに紐づいた権限を確認　＆ Allow or Deny
④で取得したuserに紐づいている、k8s の Role & RoleBindingから権限の確認してAPIリクエストに対して実行可能かどうかを判断します。


# 参考文献
- https://yomon.hatenablog.com/entry/2020/11/eks_system_masters
- https://katainaka0503.hatenablog.com/entry/2019/12/07/091737#Kubernetes%E3%81%AE%E8%AA%8D%E8%A8%BC%E8%AA%8D%E5%8F%AFAdmissionControl
- https://zenn.dev/take4s5i/articles/aws-eks-authentication
- https://44smkn.hatenadiary.com/entry/2021/02/06/185748
- https://docs.aws.amazon.com/eks/latest/userguide/add-user-role.html
