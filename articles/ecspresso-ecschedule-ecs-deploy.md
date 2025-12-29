---
title: "ecspresso と ecschedule で ECS デプロイフローを作ってみる"
emoji: "🚀"
type: "tech"
topics: ["aws", "ecs", "ecspresso", "ecschedule", "githubactions"]
published: true
---

# はじめに

ECS へのデプロイフローを紹介します。

AWS CLI で長いコマンドを叩いたり、Terraform でタスク定義を管理したり、いろいろな方法がありますが、今回は **ecspresso** と **ecschedule** を使ったデプロイフローを紹介します。

## ecspresso とは

**[ecspresso](https://github.com/kayac/ecspresso)** は、ECS サービスとタスク定義を JSON/YAML/Jsonnet ファイルで管理し、デプロイを行うツールです。

**主な特徴:**

- `diff`: ローカルの定義ファイルと ECS 上の実行中リソースを比較し、差分を表示
- `deploy`: ローリングデプロイやブルーグリーンデプロイに対応
- `run`: マイグレーションなどのワンタイムタスクを実行
- `rollback`: 問題発生時に前のタスク定義リビジョンへ戻す
- `verify`: デプロイ前に IAM ロールやコンテナイメージの存在を検証
- `init`: 既存の ECS サービスから設定ファイルを自動生成（移行が楽）

**テンプレート機能**で環境変数や Terraform の state から値を動的に埋め込めるのも便利です。

## ecschedule とは

**[ecschedule](https://github.com/Songmu/ecschedule)** は、ECS Scheduled Tasks（EventBridge ルール）を YAML/JSON/Jsonnet で管理するツールです。

**主な特徴:**

- `diff`: 設定ファイルと EventBridge 上のルールを比較し、差分を表示
- `apply`: スケジュールルールを作成/更新
- `run`: スケジュールを無視して任意のタイミングでタスクを実行
- `dump`: 既存のルール設定を YAML 形式で出力（移行が楽）

ecspresso と同様に **tfstate プラグイン** で Terraform の出力を参照でき、両ツールを組み合わせて使いやすくなっています。

## なぜこれらのツールを使うのか

どちらも **diff で差分確認 → deploy/apply で反映** というシンプルなフローが特徴です。

| 課題                                   | ecspresso/ecschedule で解決      |
| -------------------------------------- | -------------------------------- |
| AWS CLI のコマンドが長い               | 設定ファイルに書いておける       |
| 変更の影響がわからない                 | `diff` で事前に確認できる        |
| Terraform でタスク定義を管理すると煩雑 | ECS 特化で扱いやすい             |
| Terraform の出力を使いたい             | tfstate プラグインで直接参照可能 |
| 既存サービスの移行が大変               | `init`/`dump` で設定を自動生成   |

# ecspresso でサービスをデプロイする

## 設定ファイルの構成

ecspresso では主に 3 つのファイルを用意します。

```
.
├── ecspresso.yml         # ecspresso の設定
├── ecs-task-def.json     # タスク定義
└── ecs-service-def.json  # サービス定義
```

### ecspresso.yml

クラスタ名、サービス名、参照するファイルを指定します。

```yaml:ecspresso.yml
region: ap-northeast-1
cluster: my-app-cluster
service: my-app-service
service_definition: ecs-service-def.json
task_definition: ecs-task-def.json
timeout: "10m0s"

plugins:
  - name: tfstate
    config:
      url: s3://my-project-tfstate/terraform.tfstate
```

### ecs-task-def.json

コンテナの設定を記述します。`{{ must_env }}` や `{{ tfstate }}` でテンプレート機能が使えます。

```json:ecs-task-def.json
{
  "family": "my-app-task",
  "networkMode": "awsvpc",
  "requiresCompatibilities": ["FARGATE"],
  "cpu": "256",
  "memory": "512",
  "executionRoleArn": "{{ tfstate `aws_iam_role.ecs_task_execution.arn` }}",
  "taskRoleArn": "{{ tfstate `aws_iam_role.ecs_task.arn` }}",
  "containerDefinitions": [
    {
      "name": "my_app",
      "image": "{{ tfstate `aws_ecr_repository.app.repository_url` }}:{{ must_env `IMAGE_TAG` }}",
      "essential": true,
      "portMappings": [
        {
          "containerPort": 8080,
          "protocol": "tcp"
        }
      ],
      "environment": [
        {
          "name": "ENV",
          "value": "production"
        }
      ],
      "secrets": [
        {
          "name": "DB_PASSWORD",
          "valueFrom": "{{ tfstate `aws_secretsmanager_secret.db.arn` }}:password::"
        }
      ],
      "logConfiguration": {
        "logDriver": "awslogs",
        "options": {
          "awslogs-group": "{{ tfstate `aws_cloudwatch_log_group.app.name` }}",
          "awslogs-region": "ap-northeast-1",
          "awslogs-stream-prefix": "app"
        }
      }
    }
  ]
}
```

**ポイント:**

- `{{ tfstate "..." }}`: Terraform の state から値を直接参照
- `{{ must_env "IMAGE_TAG" }}`: 環境変数から値を取得（未設定だとエラー）
- イメージ URL は `tfstate` で ECR リポジトリ URL を取得し、`must_env` でタグを指定する組み合わせが便利

### ecs-service-def.json

サービスの設定（ロードバランサー、ネットワーク等）を記述します。

```json:ecs-service-def.json
{
  "launchType": "FARGATE",
  "platformVersion": "LATEST",
  "networkConfiguration": {
    "awsvpcConfiguration": {
      "subnets": [
        "{{ tfstate `aws_subnet.private[0].id` }}",
        "{{ tfstate `aws_subnet.private[1].id` }}"
      ],
      "securityGroups": [
        "{{ tfstate `aws_security_group.ecs.id` }}"
      ],
      "assignPublicIp": "DISABLED"
    }
  },
  "loadBalancers": [
    {
      "containerName": "my_app",
      "containerPort": 8080,
      "targetGroupArn": "{{ tfstate `aws_lb_target_group.app.arn` }}"
    }
  ],
  "deploymentConfiguration": {
    "maximumPercent": 200,
    "minimumHealthyPercent": 100
  }
}
```

## デプロイの流れ

```bash
# 1. 差分を確認
ecspresso diff --config ecspresso.yml

# 2. デプロイ実行
ecspresso deploy --config ecspresso.yml
```

`diff` の出力例：

```diff
--- old task definition
+++ new task definition
@@ -10,7 +10,7 @@
   "containerDefinitions": [
     {
       "name": "my_app",
-      "image": "123456789.dkr.ecr.ap-northeast-1.amazonaws.com/my-app:v1.0.0",
+      "image": "123456789.dkr.ecr.ap-northeast-1.amazonaws.com/my-app:v1.1.0",
```

変更内容が一目でわかるので、レビューもしやすくなります。

## マイグレーションの実行

デプロイ前にマイグレーションを実行したい場合は `ecspresso run` を使います。

```bash
# 別のタスク定義でワンタイム実行
ecspresso run \
  --config ecspresso.yml \
  --task-def ecs-task-def-migrate.json
```

# ecschedule でスケジュールタスクを管理する

定期実行するバッチ処理は ecschedule で管理します。

## 設定ファイル

```yaml:ecschedule.yaml
region: ap-northeast-1
cluster: my-app-cluster

# 共通設定を YAML アンカーで定義
common: &common
  taskDefinition: my-app-task
  launch_type: FARGATE
  platform_version: LATEST
  network_configuration:
    aws_vpc_configuration:
      subnets:
        - {{ tfstate `aws_subnet.private[0].id` }}
        - {{ tfstate `aws_subnet.private[1].id` }}
      security_groups:
        - {{ tfstate `aws_security_group.ecs.id` }}
      assign_public_ip: DISABLED

rules:
  # 毎時 0 分に実行するバッチ
  - name: hourly-sync-batch
    description: 外部システムとのデータ同期
    scheduleExpression: cron(0 * * * ? *)
    disabled: false
    <<: *common
    containerOverrides:
      - name: my_app
        command: ["./batch", "sync"]

  # 毎日 9:00 (JST) に実行するバッチ
  - name: daily-report-batch
    description: 日次レポートの生成
    scheduleExpression: cron(0 0 * * ? *)  # UTC 0:00 = JST 9:00
    disabled: false
    <<: *common
    containerOverrides:
      - name: my_app
        command: ["./batch", "report", "--type", "daily"]

  # 15 分ごとに実行するバッチ
  - name: reminder-batch
    description: リマインダー通知の送信
    scheduleExpression: cron(0/15 * * * ? *)
    disabled: false
    <<: *common
    containerOverrides:
      - name: my_app
        command: ["./batch", "reminder"]

plugins:
  - name: tfstate
    config:
      url: s3://my-project-tfstate/terraform.tfstate
```

**ポイント:**

- YAML アンカー（`&common`）で共通設定を再利用
- `containerOverrides` でバッチごとにコマンドを上書き
- `scheduleExpression` は UTC で指定（JST との時差に注意）

## 適用の流れ

```bash
# 1. 差分を確認
ecschedule diff --conf ecschedule.yaml --all

# 2. 適用
ecschedule apply --conf ecschedule.yaml --all
```

# GitHub Actions で自動化する

PR ベースで diff を確認し、ラベルでデプロイを実行するワークフローを作ります。

## Diff ワークフロー（PR 時に自動実行）

````yaml:.github/workflows/ecspresso-diff.yml
name: ECS Diff

on:
  pull_request:
    branches:
      - main
    paths:
      - "infra/**"

permissions:
  id-token: write
  contents: read
  pull-requests: write

jobs:
  ecspresso-diff:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup ecspresso
        uses: kayac/ecspresso@v2
        with:
          version: v2.5.0

      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: ${{ secrets.AWS_ROLE_ARN }}
          aws-region: ap-northeast-1

      - name: Run ecspresso diff
        id: diff
        run: |
          OUTPUT=$(ecspresso diff --config infra/ecspresso.yml 2>&1) || true
          echo "result<<EOF" >> $GITHUB_OUTPUT
          echo "$OUTPUT" >> $GITHUB_OUTPUT
          echo "EOF" >> $GITHUB_OUTPUT

      - name: Post diff to PR
        uses: marocchino/sticky-pull-request-comment@v2
        with:
          header: ecspresso-diff
          message: |
            ## ECS Diff Result

            ```
            ${{ steps.diff.outputs.result }}
            ```
````

## Deploy ワークフロー（ラベルでトリガー）

````yaml:.github/workflows/ecspresso-deploy.yml
name: ECS Deploy

on:
  pull_request:
    types: [labeled]

permissions:
  id-token: write
  contents: read
  pull-requests: write

jobs:
  ecspresso-deploy:
    runs-on: ubuntu-latest
    if: github.event.label.name == 'deploy'
    steps:
      - uses: actions/checkout@v4

      - name: Setup ecspresso
        uses: kayac/ecspresso@v2
        with:
          version: v2.5.0

      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: ${{ secrets.AWS_ROLE_ARN }}
          aws-region: ap-northeast-1

      - name: Run ecspresso deploy
        id: deploy
        run: |
          OUTPUT=$(ecspresso deploy --config infra/ecspresso.yml 2>&1)
          echo "result<<EOF" >> $GITHUB_OUTPUT
          echo "$OUTPUT" >> $GITHUB_OUTPUT
          echo "EOF" >> $GITHUB_OUTPUT

      - name: Post result to PR
        uses: marocchino/sticky-pull-request-comment@v2
        with:
          header: ecspresso-deploy
          message: |
            ## ECS Deploy Result

            ```
            ${{ steps.deploy.outputs.result }}
            ```
````

## 全体の流れ

```
1. PR を作成
   ↓
2. infra/ の変更を検知して ecspresso diff が自動実行
   ↓
3. PR コメントに差分が表示される（レビュー）
   ↓
4. 問題なければ "deploy" ラベルを付与
   ↓
5. ecspresso deploy が実行される
   ↓
6. 結果が PR コメントに投稿される
```

# やると良いこと

## ecspresso run でワンタイムタスクを GitHub Actions から実行

マイグレーションやデータ修正など、1 回だけ実行したいタスクは `ecspresso run` で実行できます。

GitHub Actions の `workflow_dispatch` と組み合わせれば、UI から手動実行でき、実行履歴も残るので運用しやすくなります。

## ecspresso rollback で素早く切り戻し

デプロイ後に問題が発生した場合、`ecspresso rollback` で前のタスク定義リビジョンに即座に戻せます。

GitHub Actions でヘルスチェックと組み合わせれば、問題検知時に自動ロールバックする運用も可能です。

## VPC Endpoint でスケジュールタスクのコストを削減

ecschedule で大量のバッチを実行する場合、タスク起動のたびに NAT Gateway を経由すると通信コストがかさみます。

ECR、CloudWatch Logs、Secrets Manager などの VPC Endpoint を作成しておくと、NAT Gateway を経由せずに AWS サービスにアクセスできます。

**コスト削減の具体例:**

15 分ごとに実行するバッチが 5 つある場合：

- 1 日あたり: 5 バッチ × 96 回 = 480 回のタスク起動
- 1 回のタスク起動で ECR からイメージ取得（約 100MB）+ ログ送信

NAT Gateway のデータ処理料金は **$0.062/GB**（東京リージョン）なので、

- 月間: 480 回 × 30 日 × 100MB = 約 1.4TB → **約 $87/月**

VPC Endpoint（Interface 型）は **$0.014/時間** × 2 AZ = **約 $20/月**

バッチの数や実行頻度が多いほど、VPC Endpoint の方がコスト効率が良くなります。

**参考:**

- [Amazon VPC の料金（NAT Gateway）](https://aws.amazon.com/jp/vpc/pricing/)
- [AWS PrivateLink の料金（VPC Endpoint）](https://aws.amazon.com/jp/privatelink/pricing/)
- [Amazon ECS インターフェイス VPC エンドポイント](https://docs.aws.amazon.com/ja_jp/AmazonECS/latest/developerguide/vpc-endpoints.html)

# まとめ

ecspresso と ecschedule を使うと：

- **diff → deploy/apply** のシンプルなフローでデプロイできる
- **変更内容を事前に確認** できるので安心
- **Terraform の出力を直接参照** できるので二重管理が不要
- **GitHub Actions と組み合わせ** て PR ベースの CI/CD を構築できる

ECS へのデプロイを検討している方は、ぜひ試してみてください。

# 参考資料

- [ecspresso - GitHub](https://github.com/kayac/ecspresso)
- [ecschedule - GitHub](https://github.com/Songmu/ecschedule)
