---
title: "terraform ハンズオン EKS 認証認可②(irsa)"
emoji: "✨"
type: "tech" # tech: 技術記事 / idea: アイデア

topics: ["kubernetes", "irsa"]
published: false
---

# 自己紹介
都内SaaS 企業でSRE エンジニア ２年目のFujihara Akitoです。
週一程度簡単なmemo書を残したいと思っています。

# EKSの認証認可
EKSの認証認可の方法には大きく2種類あります。
- aws-auth
- IAM Roles for Service Accounts (IRSA)
今回の記事ではまずirsaについて説明します。

# EKSのirsaによる認証認可
irsaとはIAM Role for Service Accountの略で文字通りIAM Roleのk8sの ServiceAccountに紐付ける機能です。
そしてIAM Roleが紐づけられたService Accountをk8sのリソースに結びつけることによってAWSリソースにアクセス可能になります。

実際にアクセスする時の概略図は以下

![](/images/k8s-irsa.drawio.png)

## (準備)EKS clusterを作成
1. awscli & terraform の install
2. [IAM Userの作成](https://docs.aws.amazon.com/ja_jp/IAM/latest/UserGuide/id_users_create.html) & [awscliの設定](https://docs.aws.amazon.com/ja_jp/cli/latest/userguide/cli-configure-role.html)
3. 環境変数にuserarn & usernameを設定
```
export TF_ENV_userarn=arn:aws:iam::××××××××××××:user/hogehoge
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
6. S3のtxt fileのupload
```
❯ aws s3 cp node-role-access.txt s3://node-role-access-bucket
❯ aws s3 cp irsa-access.txt s3://irsa-access-bucket
```

## EKS NodeのInstance Profileの認可

準備編で作成したEKS NodeのInstance ProfileにはS3([node-role-access-bucket](https://github.com/Akito-Fujihara/aws-auth-and-irsa/blob/4d07dfd51fbae7f753955180b06dd60101b22775/s3.tf#L1-L3))にアクセスするためのIAM policy([node_role_access](https://github.com/Akito-Fujihara/aws-auth-and-irsa/blob/4d07dfd51fbae7f753955180b06dd60101b22775/iam.tf#L25-L28))が付与されています。

### ①IAM role credentialの取得
PodがEKS Nodeの起動時にアタッチするAWS IAM instance profileからIAM credentailを取得します

```
❯ k apply -f node_role_pod.yaml
pod/node-role-pod created

❯ k exec -it node-role-pod -- bash

bash-4.2# TOKEN=`curl -X PUT "http://169.254.169.254/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 21600"`
  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
                                 Dload  Upload   Total   Spent    Left  Speed
100    56  100    56    0     0  10481      0 --:--:-- --:--:-- --:--:-- 11200

bash-4.2# ROLE_NAME=`curl -H "X-aws-ec2-metadata-token: $Tta/iam/security-credentials/`

bash-4.2# curl -s -H "X-aws-ec2-metadata-token: $TOKEN" http://169.254.169.254/latest/meta-data/iam/security-credentials/${ROLE_NAME}/
{
  "Code" : "Success",
  "LastUpdated" : "2023-09-03T13:15:09Z",
  "Type" : "AWS-HMAC",
  "AccessKeyId" : "HOGEHOGE",
  "SecretAccessKey" : "HOGEHOGE",
  "Token" : "HOGEHOGE",
  "Expiration" : "2023-09-03T19:24:56Z"
}

```

### ②Get file

実際にS3にアクセスしてみます

```
❯ k exec -it node-role-pod -- bash

bash-4.2# aws s3 cp s3://node-role-access-bucket ./ --recursive
download: s3://node-role-access-bucket/node-role-access.txt to ./node-role-access.txt

bash-4.2# cat ./node-role-access.txt
node-roleによって取得できるtxt file

bash-4.2# aws s3 cp s3://irsa-access-bucket ./ --recursive
fatal error: An error occurred (AccessDenied) when calling the ListObjectsV2 operation: Access Denied
```
nodeのroleを使用しているためirsa-access-bucketにはアクセスできません

## irsaの認証認可
準備編で作成した[irsa](https://github.com/Akito-Fujihara/aws-auth-and-irsa/blob/4d07dfd51fbae7f753955180b06dd60101b22775/eks.tf#L75-L93)にはS3([irsa-access-bucket](https://github.com/Akito-Fujihara/aws-auth-and-irsa/blob/4d07dfd51fbae7f753955180b06dd60101b22775/s3.tf#L5-L7))にアクセスするためのIAM Policy([irsa_access](https://github.com/Akito-Fujihara/aws-auth-and-irsa/blob/4d07dfd51fbae7f753955180b06dd60101b22775/iam.tf#L54-L57))が付与されています。

```
❯ k apply -f irsa.yaml
serviceaccount/s3-irsa-access created
rolebinding.rbac.authorization.k8s.io/s3-irsa-access-rb created

❯ k apply -f irsa_pod.yaml
pod/irsa-access-pod created
```

### ①OIDC JWT tokenの取得
kubectl apply -fコマンドを使ってPodを起動する時に k8s api-serverにあるAmazon EKS Pod Identity webhookがService AccountとそのService AccountのAnnotationにAWS IAM Role ARNがあるか確認しています。
Service AccountのAnnotationにAWS IAM Role ARNがある場合にOIDC経由で取得したJWT トークンを`/var/run/secrets/eks.amazonaws.com/serviceaccount/token`に保存しています。

```
❯ k get pod irsa-access-pod -o yaml
・・・
spec:
  containers:
  - command:
    - sleep
    - "600"
    env:
    - name: AWS_STS_REGIONAL_ENDPOINTS
      value: regional
    - name: AWS_DEFAULT_REGION
      value: ap-northeast-1
    - name: AWS_REGION
      value: ap-northeast-1
    - name: AWS_ROLE_ARN
      value: arn:aws:iam::××××××××××××:role/s3-irsa-access # < IAM role ARNがインジェクトされている
    - name: AWS_WEB_IDENTITY_TOKEN_FILE
      value: /var/run/secrets/eks.amazonaws.com/serviceaccount/token # < IAM roleをAssumeするためのJWT token
    image: amazon/aws-cli
・・・

❯ k exec -it irsa-access-pod -- bash

bash-4.2# cat /var/run/secrets/eks.amazonaws.com/serviceaccount/token
HOGEHOGE
```

### ②IAM role credentialの取得
PodがAWS_WEB_IDENTITY_TOKEN_FILEに保存されたトークンを使ってsts:assume-role-with-web-identityコマンドを実行し、AWS IAM roleをAssumeする。

IAM role credentialの取得の再現
```
❯ k exec -it irsa-access-pod -- bash

bash-4.2# OIDC_TOKEN=`cat /var/run/secrets/eks.amazonaws.com/serviceaccount/token`

bash-4.2# AWS_ROLE_ARN=arn:aws:iam::××××××××××××:role/s3-irsa-access

bash-4.2# aws sts assume-role-with-web-identity \
>               --role-arn ${AWS_ROLE_ARN} \
>               --web-identity-token ${OIDC_TOKEN} \
>               --role-session-name "oidc" \
>               --duration-seconds 900 \
>               --query "Credentials" \
>               --output "json"
{
    "AccessKeyId": "HOGEHOGE",
    "SecretAccessKey": "HOGEHOGE",
    "SessionToken": "HOGEHOGE",
    "Expiration": "2023-09-03T15:01:58+00:00"
}
```

AssumeしたRoleの確認
```
bash-4.2# aws sts get-caller-identity # < assumeしたroleを確認
{
    "UserId": "AROA3FFDE7MHRFF4AREEW:botocore-session-1693751200",
    "Account": "××××××××××××",
    "Arn": "arn:aws:sts::××××××××××××:assumed-role/s3-irsa-access/botocore-session-1693751200"
}
```

### ③Get file
```
❯ k exec -it irsa-access-pod -- bash

bash-4.2# aws s3 cp s3://node-role-access-bucket ./ --recursive
fatal error: An error occurred (AccessDenied) when calling the ListObjectsV2 operation: Access Denied

bash-4.2# aws s3 cp s3://irsa-access-bucket ./ --recursive
download: s3://irsa-access-bucket/irsa-access.txt to ./irsa-access.txt

bash-4.2# cat ./irsa-access.txt
irsaによって取得できるtxt file
```
irsaを使用しているためnode roleで取得できるfileは取得できない
