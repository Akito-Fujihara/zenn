---
title: "EKSで始めるArgo Workflows(Artifact Repository)"
emoji: "✨"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: ["kubernetes", "eks", "argoworkflows", "artifactrepository"]
published: true
---

# 自己紹介
都内SaaS 企業でSRE エンジニア ２年目のFujihara Akitoです。
週一程度簡単なmemo書を残したいと思っています。

# Argo Workflows の　Artifact Repository
Artifact Repository とは CI/CD ワークフローで生成される Artifact(ログ, パッケージ)などを外部に保管するRepositoryのことです。
Argo Workflow にもジョブ実行によって生成されるログや共有したいファイルを保管するためにArtifact Repository を設定することができます。
https://argoproj.github.io/argo-workflows/configure-artifact-repository/

# EKS & S３ で実践
## Argo Workflows を EKS　にデプロイ
https://zenn.dev/fujihara_akito/articles/argo-workflow-eks-1 を参考にデプロイ

## S3の作成
Artifact を今回はS3に保管します。
```
resource "aws_s3_bucket" "artifact_repo_1" {
  bucket = "argowf-artifact-repo-1"
}

resource "aws_s3_bucket" "artifact_repo_2" {
  bucket = "argowf-artifact-repo-2"
}
```

## irsa role の作成
Argo Workflows の Artifact Repository を S3 で行うためには `kind: Workflow` などのジョブ実行リソースだけではなく `argo-workflows-server & argo-workflows-workflow-controller` にもS3に対する権限を付与してあげる必要があります。
```
module "argowf_s3_access" {
  source = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "5.30.0"

  role_name = "argowf-s3-access"
  allow_self_assume_role = false

  oidc_providers = {
    main = {
      provider_arn = module.eks.oidc_provider_arn
      namespace_service_accounts = [
        "argo-workflows:argo-workflows-server",
        "argo-workflows:argo-workflows-workflow-controller",
      ]
    }
  }
  role_policy_arns = {
    s3_1 = aws_iam_policy.artifact_repo_1_access.arn,
    s3_2 = aws_iam_policy.artifact_repo_2_access.arn,
  }
}

module "artifact_repo_1_s3_access" {
  source = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "5.30.0"

  role_name = "s3-1-access"
  allow_self_assume_role = false

  oidc_providers = {
    main = {
      provider_arn = module.eks.oidc_provider_arn
      namespace_service_accounts = [
        "argo-workflows:artifact-repo-1-access",
      ]
    }
  }
  role_policy_arns = {
    s3_1 = aws_iam_policy.artifact_repo_1_access.arn,
  }
}

module "artifact_repo_2_s3_access" {
  source = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "5.30.0"

  role_name = "s3-2-access"
  allow_self_assume_role = false

  oidc_providers = {
    main = {
      provider_arn = module.eks.oidc_provider_arn
      namespace_service_accounts = [
        "argo-workflows:artifact-repo-2-access",
      ]
    }
  }
  role_policy_arns = {
    s3_1 = aws_iam_policy.artifact_repo_2_access.arn,
  }
}
```

## ServiceAccount に IAM role　を annotation
helm の value file
```
controller:
・・・・
  serviceAccount:
    annotations:
      eks.amazonaws.com/role-arn: {{ requiredEnv "ARGOWF_IRSA_ROLE_ARN" | quote}}

・・・・

server:
・・・・
  serviceAccount:
    annotations:
      eks.amazonaws.com/role-arn: {{ requiredEnv "ARGOWF_IRSA_ROLE_ARN" | quote}}
・・・・
```

`kind: Workflow & kind: WorkflowTemplate` 用の ServiceAccount を作成 & annotation をしておく
- https://github.com/Akito-Fujihara/integration-argoworkflows/blob/main/kubernetes/workflows/templates/serviceaccount.yaml
- https://github.com/Akito-Fujihara/integration-argoworkflows/blob/main/kubernetes/workflows/templates/role-binding.yaml
- https://github.com/Akito-Fujihara/integration-argoworkflows/blob/main/kubernetes/workflows/templates/role.yaml

## Artifact Repository の設定を必要な `kind: ConfigMap` を作成
helm の value file
```
useStaticCredentials: false
artifactRepositoryRef:
  artifact-repositories:
    s3-1-access:
      archiveLogs: true
      s3:
        endpoint: s3.amazonaws.com
        bucket: "argowf-artifact-repo-1"
        insecure: false
        useSDKCreds: true
        encryptionOptions:
          enableEncryption: false
        keyFormat: "{{ `{{workflow.creationTimestamp.Y}}\
          /{{workflow.creationTimestamp.m}}\
          /{{workflow.creationTimestamp.d}}\
          /{{workflow.namespace}}\
          /{{workflow.name}}\
          /{{pod.name}}` }}"
    s3-2-access:
      archiveLogs: true
      s3:
        endpoint: s3.amazonaws.com
        bucket: "argowf-artifact-repo-2"
        insecure: false
        useSDKCreds: true
        encryptionOptions:
          enableEncryption: false
        keyFormat: "{{ `{{workflow.creationTimestamp.Y}}\
          /{{workflow.creationTimestamp.m}}\
          /{{workflow.creationTimestamp.d}}\
          /{{workflow.namespace}}\
          /{{workflow.name}}\
          /{{pod.name}}` }}"
```

## 実行するWorkflowにServiceAccount と artifactRepositoryRef　の設定
```
# S3(argowf-artifact-repo-1) を設定
apiVersion: argoproj.io/v1alpha1
kind: WorkflowTemplate
metadata:
  name: s3-1-artifact-wftemplate
spec:
  serviceAccountName: artifact-repo-1-access
  artifactRepositoryRef:
    configMap: artifact-repositories
    key: s3-1-access

・・・・

# S3(argowf-artifact-repo-2) を設定
apiVersion: argoproj.io/v1alpha1
kind: WorkflowTemplate
metadata:
  name: s3-2-artifact-wftemplate
spec:
  serviceAccountName: artifact-repo-2-access
  artifactRepositoryRef:
    configMap: artifact-repositories
    key: s3-2-access

・・・・
```

WorkflowTemplate を実行すると S3 に log が保管されていることが確認できると思います。
