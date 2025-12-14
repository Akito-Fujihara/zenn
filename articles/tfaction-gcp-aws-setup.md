---
title: "tfaction で AWS & GCP のマルチクラウド Terraform CI/CD を構築する"
emoji: "🚀"
type: "tech"
topics: ["terraform", "githubactions", "aws", "gcp", "cicd"]
published: true
---

# tfaction とは

[tfaction](https://github.com/suzuki-shunsuke/tfaction) は、Monorepo 向けの Terraform ワークフローを GitHub Actions で構築するためのフレームワークです。

## 主な特徴

- **動的ビルドマトリックス**: 変更のあった作業ディレクトリのみで CI を実行
- **安全な apply**: Plan ファイルを使用した安全な apply 実行
- **マルチクラウド対応**: AWS、GCP の OIDC 認証に対応
- **自動修正**: `.terraform.lock.hcl` の自動更新・コミット

## 提供される主要な Actions

| Action         | 説明                                      |
| -------------- | ----------------------------------------- |
| `list-targets` | 変更のあった作業ディレクトリを検出        |
| `setup`        | terraform init などの準備処理             |
| `plan`         | terraform plan の実行と結果の PR コメント |
| `apply`        | terraform apply の実行                    |

# 全体構成

本記事で構築する構成は以下の通りです：

```
terraform-repo/
├── .github/
│   └── workflows/
│       ├── plan.yaml          # PR 時に terraform plan を実行
│       └── apply.yaml         # apply ラベル付与時に terraform apply を実行
├── aqua.yaml                  # CLI ツールのバージョン管理
├── tfaction-root.yaml         # tfaction のルート設定
├── oidc-role/                 # AWS + GCP の OIDC を一括管理
│   ├── aws.tf
│   ├── gcp.tf
│   ├── provider.tf
│   └── tfaction.yaml
├── staging/
│   └── some-service/
│       ├── main.tf
│       ├── provider.tf
│       └── tfaction.yaml
└── production/
    └── some-service/
        ├── main.tf
        ├── provider.tf
        └── tfaction.yaml
```

# 事前準備

## 1. GitHub App の作成

tfaction では GitHub App を使用して、PR へのコメントやラベル付けを行います。

1. GitHub の Settings → Developer settings → GitHub Apps から新しい App を作成
2. 以下の権限を付与：
   - Contents: Read and write
   - Pull requests: Read and write
   - Issues: Read and write
3. App ID と Private Key を取得し、リポジトリの Secrets に登録：
   - `GH_APP_ID`
   - `GH_APP_PRIVATE_KEY`

## 2. Terraform State 用の S3 バケット作成

Terraform State 管理用に S3 バケットを作成します（手動または別の Terraform で管理）。

```hcl
resource "aws_s3_bucket" "tfstate" {
  bucket = "your-terraform-tfstate"
}

resource "aws_s3_bucket_versioning" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id
  versioning_configuration {
    status = "Enabled"
  }
}
```

# OIDC 認証の設定（AWS + GCP）

GitHub Actions から AWS・GCP リソースにアクセスするために、OIDC 認証を設定します。
1 つのディレクトリで両方のクラウドの認証設定を管理します。

## oidc-role/provider.tf

```hcl
provider "aws" {
  region = "ap-northeast-1"
  default_tags {
    tags = {
      ManagedBy = "terraform"
    }
  }
}

provider "google" {
  project = "your-gcp-project-id"
  region  = "asia-northeast1"
  default_labels = {
    managed_by = "terraform"
  }
}

terraform {
  required_version = "~> 1.11.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
  }
  backend "s3" {
    bucket       = "your-terraform-tfstate"
    key          = "oidc-role/terraform.tfstate"
    region       = "ap-northeast-1"
    use_lockfile = true
  }
}
```

## oidc-role/aws.tf

```hcl
# GitHub Actions OIDC Provider の証明書を取得
data "tls_certificate" "github_actions" {
  url = "https://token.actions.githubusercontent.com/.well-known/openid-configuration"
}

# OIDC Provider の作成
resource "aws_iam_openid_connect_provider" "github_actions" {
  url             = "https://token.actions.githubusercontent.com"
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = [data.tls_certificate.github_actions.certificates[0].sha1_fingerprint]
}

# terraform plan 用の Role（読み取り権限）
module "iam_github_read_oidc_role" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-github-oidc-role"
  version = "5.55.0"

  name = "github-oidc-read-role"

  subjects = [
    "your-org/your-terraform-repo:*",
  ]

  policies = {
    ReadOnly     = "arn:aws:iam::aws:policy/ReadOnlyAccess"
    S3FullAccess = "arn:aws:iam::aws:policy/AmazonS3FullAccess"
  }
}

# terraform apply 用の Role（管理者権限）
module "iam_github_admin_oidc_role" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-github-oidc-role"
  version = "5.55.0"

  name = "github-oidc-admin-role"

  subjects = [
    "your-org/your-terraform-repo:*",
  ]

  policies = {
    Admin        = "arn:aws:iam::aws:policy/AdministratorAccess"
    S3FullAccess = "arn:aws:iam::aws:policy/AmazonS3FullAccess"
  }
}
```

## oidc-role/gcp.tf

```hcl
# プロジェクト情報の取得
data "google_project" "project" {}

# Workload Identity Pool の作成
resource "google_iam_workload_identity_pool" "github" {
  workload_identity_pool_id = "github-actions"
  display_name              = "GitHub Actions"
}

# Workload Identity Provider の作成
resource "google_iam_workload_identity_pool_provider" "github" {
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-provider"
  display_name                       = "GitHub Provider"

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.actor"      = "assertion.actor"
    "attribute.repository" = "assertion.repository"
  }

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }

  attribute_condition = "assertion.repository == 'your-org/your-terraform-repo'"
}

# terraform plan 用の Service Account
resource "google_service_account" "terraform_plan" {
  account_id   = "terraform-plan"
  display_name = "Terraform Plan Service Account"
}

# terraform apply 用の Service Account
resource "google_service_account" "terraform_apply" {
  account_id   = "terraform-apply"
  display_name = "Terraform Apply Service Account"
}

# Workload Identity と Service Account の紐付け（plan 用）
resource "google_service_account_iam_member" "workload_identity_plan" {
  service_account_id = google_service_account.terraform_plan.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.repository/your-org/your-terraform-repo"
}

# Workload Identity と Service Account の紐付け（apply 用）
resource "google_service_account_iam_member" "workload_identity_apply" {
  service_account_id = google_service_account.terraform_apply.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.repository/your-org/your-terraform-repo"
}

# Plan 用 Service Account への権限付与
resource "google_project_iam_member" "terraform_plan_viewer" {
  project = data.google_project.project.project_id
  role    = "roles/viewer"
  member  = "serviceAccount:${google_service_account.terraform_plan.email}"
}

# Apply 用 Service Account への権限付与
resource "google_project_iam_member" "terraform_apply_editor" {
  project = data.google_project.project.project_id
  role    = "roles/editor"
  member  = "serviceAccount:${google_service_account.terraform_apply.email}"
}
```

## oidc-role/tfaction.yaml

各作業ディレクトリには `tfaction.yaml` を配置します。空のオブジェクトでも OK です：

```yaml
{}
```

# tfaction-root.yaml の設定

リポジトリのルートに `tfaction-root.yaml` を配置し、各作業ディレクトリの設定を記述します。

```yaml
plan_workflow_name: terraform-plan
draft_pr: true

target_groups:
  # OIDC 認証設定（AWS + GCP を同時に使用）
  - working_directory: oidc-role
    target: oidc-role
    aws_region: ap-northeast-1
    terraform_plan_config:
      aws_assume_role_arn: arn:aws:iam::123456789012:role/github-oidc-read-role
      gcp_service_account: terraform-plan@your-project-id.iam.gserviceaccount.com
      gcp_workload_identity_provider: projects/123456789012/locations/global/workloadIdentityPools/github-actions/providers/github-provider
    terraform_apply_config:
      aws_assume_role_arn: arn:aws:iam::123456789012:role/github-oidc-admin-role
      gcp_service_account: terraform-apply@your-project-id.iam.gserviceaccount.com
      gcp_workload_identity_provider: projects/123456789012/locations/global/workloadIdentityPools/github-actions/providers/github-provider

  # staging 環境
  - working_directory: staging/some-service
    target: staging/some-service
    aws_region: ap-northeast-1
    terraform_plan_config:
      aws_assume_role_arn: arn:aws:iam::123456789012:role/github-oidc-read-role
      gcp_service_account: terraform-plan@your-project-id.iam.gserviceaccount.com
      gcp_workload_identity_provider: projects/123456789012/locations/global/workloadIdentityPools/github-actions/providers/github-provider
    terraform_apply_config:
      aws_assume_role_arn: arn:aws:iam::123456789012:role/github-oidc-admin-role
      gcp_service_account: terraform-apply@your-project-id.iam.gserviceaccount.com
      gcp_workload_identity_provider: projects/123456789012/locations/global/workloadIdentityPools/github-actions/providers/github-provider

  # production 環境
  - working_directory: production/some-service
    target: production/some-service
    aws_region: ap-northeast-1
    terraform_plan_config:
      aws_assume_role_arn: arn:aws:iam::123456789012:role/github-oidc-read-role
      gcp_service_account: terraform-plan@your-project-id.iam.gserviceaccount.com
      gcp_workload_identity_provider: projects/123456789012/locations/global/workloadIdentityPools/github-actions/providers/github-provider
    terraform_apply_config:
      aws_assume_role_arn: arn:aws:iam::123456789012:role/github-oidc-admin-role
      gcp_service_account: terraform-apply@your-project-id.iam.gserviceaccount.com
      gcp_workload_identity_provider: projects/123456789012/locations/global/workloadIdentityPools/github-actions/providers/github-provider
```

## 設定項目の説明

| 項目                             | 説明                                                                     |
| -------------------------------- | ------------------------------------------------------------------------ |
| `working_directory`              | Terraform ファイルが配置されているディレクトリ                           |
| `target`                         | tfaction が識別に使用するターゲット名（通常は working_directory と同じ） |
| `aws_region`                     | AWS リソースを操作する際のリージョン                                     |
| `terraform_plan_config`          | terraform plan 時の認証設定                                              |
| `terraform_apply_config`         | terraform apply 時の認証設定                                             |
| `aws_assume_role_arn`            | AWS OIDC で Assume する IAM Role の ARN                                  |
| `gcp_service_account`            | GCP で使用する Service Account のメールアドレス                          |
| `gcp_workload_identity_provider` | GCP Workload Identity Provider の完全な名前                              |

# aqua.yaml の設定

tfaction は [aqua](https://aquaproj.github.io/) を使用して CLI ツールのバージョンを管理します。

```yaml
---
# yaml-language-server: $schema=https://raw.githubusercontent.com/aquaproj/aqua/main/json-schema/aqua-yaml.json
registries:
  - type: standard
    ref: v4.355.0

packages:
  # Terraform 本体
  - name: hashicorp/terraform@v1.11.4
  # Terraform リンター
  - name: terraform-linters/tflint@v0.56.0
  # GitHub コメント投稿ツール
  - name: suzuki-shunsuke/github-comment@v6.3.2
  # GitHub へのファイルプッシュツール
  - name: int128/ghcp@v1.13.5
```

# GitHub Workflows の設定

## .github/workflows/plan.yaml

PR 作成・更新時に terraform plan を実行するワークフローです。

```yaml
name: terraform-plan

on:
  pull_request:
    branches:
      - main

permissions:
  id-token: write # OIDC 認証に必要
  contents: write # コミットのプッシュに必要
  pull-requests: write # PR コメントに必要
  issues: write # ラベル作成に必要

jobs:
  # 変更のあった作業ディレクトリを検出
  setup:
    runs-on: ubuntu-latest
    outputs:
      targets: ${{ steps.list-targets.outputs.targets }}
    steps:
      - uses: actions/checkout@v4

      - uses: aquaproj/aqua-installer@v3.1.2
        with:
          aqua_version: v2.50.0

      - uses: suzuki-shunsuke/tfaction/list-targets@v1.16.1
        id: list-targets

  # 検出された各ディレクトリで terraform plan を実行
  plan:
    name: "terraform plan (${{ matrix.target.target }})"
    runs-on: ${{ matrix.target.runs_on }}
    needs: setup

    # 変更のある作業ディレクトリが存在する場合のみ実行
    if: join(fromJSON(needs.setup.outputs.targets), '') != ''

    strategy:
      fail-fast: false
      matrix:
        target: ${{ fromJSON(needs.setup.outputs.targets) }}

    env:
      TFACTION_TARGET: ${{ matrix.target.target }}
      TFACTION_JOB_TYPE: terraform

    steps:
      - uses: actions/checkout@v4

      - uses: aquaproj/aqua-installer@v3.1.2
        with:
          aqua_version: v2.50.0

      # GitHub App トークンの生成
      - id: github_app_token
        uses: tibdex/github-app-token@v2
        with:
          app_id: ${{ secrets.GH_APP_ID }}
          private_key: ${{ secrets.GH_APP_PRIVATE_KEY }}

      # terraform init などの準備処理
      - uses: suzuki-shunsuke/tfaction/setup@v1.16.1
        with:
          github_app_token: ${{ steps.github_app_token.outputs.token }}

      # terraform plan の実行
      - uses: suzuki-shunsuke/tfaction/plan@v1.16.1
        with:
          github_app_token: ${{ steps.github_app_token.outputs.token }}
```

## .github/workflows/apply.yaml

`apply` ラベルが付与された際に terraform apply を実行するワークフローです。

```yaml
name: terraform-apply

on:
  pull_request:
    types: [labeled]

permissions:
  id-token: write
  contents: read
  pull-requests: write
  actions: read

jobs:
  # 変更のあった作業ディレクトリを検出
  setup:
    runs-on: ubuntu-latest
    # apply ラベルが付与された場合のみ実行
    if: github.event.label.name == 'apply'
    outputs:
      targets: ${{ steps.list-targets.outputs.targets }}
    steps:
      - uses: actions/checkout@v4

      - uses: aquaproj/aqua-installer@v3.1.2
        with:
          aqua_version: v2.50.0

      - uses: suzuki-shunsuke/tfaction/list-targets@v1.16.1
        id: list-targets

  # terraform apply の実行
  apply:
    name: "terraform apply (${{ matrix.target.target }})"
    runs-on: ${{ matrix.target.runs_on }}
    needs: setup

    if: join(fromJSON(needs.setup.outputs.targets), '') != ''

    strategy:
      fail-fast: false
      matrix:
        target: ${{ fromJSON(needs.setup.outputs.targets) }}

    env:
      TFACTION_IS_APPLY: "true"
      TFACTION_TARGET: ${{ matrix.target.target }}
      TFACTION_JOB_TYPE: terraform

    steps:
      - uses: actions/checkout@v4

      - uses: aquaproj/aqua-installer@v3.1.2
        with:
          aqua_version: v2.50.0

      - id: github_app_token
        uses: tibdex/github-app-token@v2
        with:
          app_id: ${{ secrets.GH_APP_ID }}
          private_key: ${{ secrets.GH_APP_PRIVATE_KEY }}

      # terraform init などの準備処理
      - uses: suzuki-shunsuke/tfaction/setup@v1.16.1
        with:
          github_app_token: ${{ steps.github_app_token.outputs.token }}

      # terraform apply の実行
      - uses: suzuki-shunsuke/tfaction/apply@v1.16.1
        with:
          github_app_token: ${{ steps.github_app_token.outputs.token }}
```

# まとめ

本記事では、tfaction を使用した AWS & GCP マルチクラウド対応の Terraform CI/CD パイプライン構築方法を解説しました。

## tfaction を使うメリット

1. **セキュリティ向上**: ローカルからの apply が不要になり、OIDC 認証により長期の認証情報が不要
2. **効率的な CI**: Monorepo でも変更のあったディレクトリのみで CI が実行される
3. **マルチクラウド対応**: AWS と GCP の認証を統一的に管理
4. **運用負荷軽減**: `.terraform.lock.hcl` の自動更新など、手作業を削減

## 注意点

- OIDC Role は最初に手動または別の方法で作成する必要があります
- GitHub App の設定が必要です
- 各作業ディレクトリに `tfaction.yaml`（空でも可）が必要です

tfaction を導入することで、安全で効率的な Terraform ワークフローを構築できます。ぜひお試しください。

# 参考資料

- [tfaction 公式ドキュメント](https://suzuki-shunsuke.github.io/tfaction/docs/)
- [tfaction GitHub リポジトリ](https://github.com/suzuki-shunsuke/tfaction)
- [aqua 公式ドキュメント](https://aquaproj.github.io/)
- [AWS IAM OIDC Provider](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_create_oidc.html)
- [GCP Workload Identity Federation](https://cloud.google.com/iam/docs/workload-identity-federation)
