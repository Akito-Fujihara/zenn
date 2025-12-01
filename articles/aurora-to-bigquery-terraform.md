---
title: "Terraform で構築する Aurora → BigQuery データパイプライン"
emoji: "📊"
type: "tech"
topics: ["terraform", "aws", "gcp", "bigquery", "dms"]
published: true
---

# はじめに

RDB のデータを分析基盤で活用したいケースは多くあります。しかし、本番 DB に直接クエリを投げると負荷やセキュリティの問題が発生するため、分析用途に特化したデータ基盤へデータを連携することが一般的です。

本記事では、AWS Aurora（MySQL）から Google BigQuery へデータを連携するパイプラインを Terraform で構築する方法を解説します。

# アーキテクチャ

![](/images/aurora-bigquery-pipeline.drawio.png)

本構成では以下のフローでデータを連携します：

1. **AWS DMS Serverless** が Aurora（Reader Endpoint）からデータを抽出
2. **S3** に Parquet 形式でデータを保存
3. **Google Storage Transfer Service** が S3 から GCS へデータを転送
4. **BigQuery External Table** が GCS 上の Parquet ファイルを直接参照

各コンポーネントの役割：

| コンポーネント           | 役割                                           |
| ------------------------ | ---------------------------------------------- |
| DMS Serverless           | Aurora → S3 へのデータ抽出・変換（Parquet 化） |
| S3                       | 一時的なデータ保存場所（AWS 側）               |
| EventBridge Scheduler    | DMS の定期実行トリガー                         |
| Storage Transfer Service | S3 → GCS のクロスクラウドデータ転送            |
| GCS                      | 分析用データの保存場所（GCP 側）               |
| BigQuery External Table  | GCS 上のデータを直接クエリ                     |

# Aurora → BigQuery 連携方法の比較

Aurora から BigQuery へデータを連携する代表的な方法として、GCP ネイティブの **Datastream** があります。本構成と比較して説明します。

## Datastream + BigQuery との比較

| 項目               | 本構成（DMS + Storage Transfer）  | Datastream + BigQuery            |
| ------------------ | --------------------------------- | -------------------------------- |
| **Aurora 対応**    | ○（DMS が Aurora を直接サポート） | ○（Aurora MySQL 対応）           |
| **ネットワーク**   | インターネット経由（S3/GCS）      | VPN / Cloud Interconnect 必要    |
| **データ形式**     | Parquet（カラム型、圧縮効率高）   | BigQuery ネイティブ              |
| **CDC**            | ○（DMS CDC 対応）                 | ○（ネイティブ CDC）              |
| **マスキング**     | DMS でカラム除外 or BigQuery      | BigQuery のみ                    |
| **サーバーレス**   | ○                                 | ○                                |
| **リアルタイム性** | 中（定期バッチ）                  | 高（継続的レプリケーション）     |
| **中間ストレージ** | S3 / GCS（Parquet 保存）          | 不要（直接 BigQuery へ）         |

## 本構成を選んだ理由

1. **ネットワーク構成がシンプル**: VPN / Interconnect 不要でインターネット経由の転送が可能
2. **Parquet 形式での中間保存**: カラム型フォーマットで圧縮効率が高く、BigQuery 以外のツールからも参照可能
3. **セキュアなクロスクラウド認証**: OpenID Connect フェデレーションでアクセスキー不要
4. **疎結合**: 各コンポーネントが独立しており、障害の影響範囲が限定的
5. **AWS 側の制御**: DMS の柔軟なテーブルマッピングやカラム除外が利用可能

# Terraform 実装

本記事では主要なリソース定義を紹介します。Provider 設定やデータソースなど基本的な部分は省略しています。

## DMS（Database Migration Service）

DMS Serverless を使用して Aurora から S3 へデータを抽出します。

```hcl:dms.tf
# DMS レプリケーション用のサブネットグループ
resource "aws_dms_replication_subnet_group" "main" {
  replication_subnet_group_description = "DMS subnet group for analytics pipeline"
  replication_subnet_group_id          = "sample-analytics-dms-subnet-group"
  subnet_ids                           = local.private_subnet_ids
}

# Aurora をソースとするエンドポイント
resource "aws_dms_endpoint" "aurora_source" {
  endpoint_id   = "sample-aurora-source"
  endpoint_type = "source"
  engine_name   = "aurora"
  database_name = "myapp"
  username      = local.db_secrets.MYSQL_USER
  password      = local.db_secrets.MYSQL_PASSWORD
  server_name   = data.aws_rds_cluster.aurora_cluster.reader_endpoint
  port          = 3306
  ssl_mode      = "none"
}

# S3 をターゲットとするエンドポイント（Parquet 形式）
resource "aws_dms_endpoint" "s3_target" {
  endpoint_id   = "sample-s3-target"
  endpoint_type = "target"
  engine_name   = "s3"

  s3_settings {
    service_access_role_arn          = aws_iam_role.dms_s3_target_role.arn
    bucket_name                      = aws_s3_bucket.analytics_data.id
    bucket_folder                    = ""
    compression_type                 = "GZIP"
    data_format                      = "parquet"
    encoding_type                    = "plain"
    dict_page_size_limit             = 1048576
    enable_statistics                = true
    include_op_for_full_load         = false
    parquet_timestamp_in_millisecond = true
    parquet_version                  = "parquet-2-0"
    row_group_length                 = 1024
    data_page_size                   = 1048576
    add_column_name                  = true
    timestamp_column_name            = "dms_timestamp"
    use_csv_no_sup_value             = false
    preserve_transactions            = false
    cdc_inserts_and_updates          = false
    cdc_inserts_only                 = false
  }
}

# DMS Serverless レプリケーション設定
resource "aws_dms_replication_config" "main" {
  replication_config_identifier = "sample-aurora-to-s3-serverless"
  replication_type              = "full-load"
  source_endpoint_arn           = aws_dms_endpoint.aurora_source.endpoint_arn
  target_endpoint_arn           = aws_dms_endpoint.s3_target.endpoint_arn
  start_replication             = false  # 手動または EventBridge から実行

  # Serverless コンピュート設定
  compute_config {
    replication_subnet_group_id = aws_dms_replication_subnet_group.main.id
    vpc_security_group_ids      = [local.dms_sg_id]
    min_capacity_units          = 1    # 最小 DCU（1 DCU = 2GB メモリ）
    max_capacity_units          = 2    # 最大 DCU
    multi_az                    = true # 高可用性のためマルチ AZ
  }

  # テーブルマッピング（全テーブルを対象）
  table_mappings = jsonencode({
    rules = [
      {
        rule-type = "selection"
        rule-id   = "1"
        rule-name = "all-tables"
        object-locator = {
          schema-name = "myapp"
          table-name  = "%"
        }
        rule-action = "include"
      }
    ]
  })
}
```

### Parquet 設定のポイント

| 設定項目                | 値            | 説明                                 |
| ----------------------- | ------------- | ------------------------------------ |
| `data_format`           | parquet       | Parquet 形式で出力                   |
| `compression_type`      | GZIP          | 圧縮してファイルサイズを削減         |
| `parquet_version`       | parquet-2-0   | Parquet 2.0 形式（より効率的）       |
| `timestamp_column_name` | dms_timestamp | DMS 実行時刻のタイムスタンプ列を追加 |

## S3 バケット

DMS の出力先となる S3 バケットを作成します。

```hcl:s3.tf
# DMS のデータ保存先 S3 バケット
resource "aws_s3_bucket" "analytics_data" {
  bucket = "sample-analytics-data-myapp"
}

# バージョニング設定
resource "aws_s3_bucket_versioning" "analytics_data" {
  bucket = aws_s3_bucket.analytics_data.id

  versioning_configuration {
    status = "Enabled"
  }
}

# 暗号化設定
resource "aws_s3_bucket_server_side_encryption_configuration" "analytics_data" {
  bucket = aws_s3_bucket.analytics_data.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# パブリックアクセスブロック
resource "aws_s3_bucket_public_access_block" "analytics_data" {
  bucket = aws_s3_bucket.analytics_data.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# ライフサイクルポリシー（7日後に自動削除）
resource "aws_s3_bucket_lifecycle_configuration" "analytics_data" {
  bucket = aws_s3_bucket.analytics_data.id

  rule {
    id     = "delete-old-data"
    status = "Enabled"

    filter {
      prefix = ""
    }

    expiration {
      days = 7
    }
  }
}
```

## EventBridge Scheduler

DMS を定期実行するスケジューラを設定します。

```hcl:eventbridge.tf
# 3時間ごとに DMS タスクを実行
resource "aws_scheduler_schedule" "dms_task_schedule" {
  name        = "sample-dms-task-schedule"
  description = "Run DMS task every 3 hours"

  # 0, 3, 6, 9, 12, 15, 18, 21時に実行
  schedule_expression          = "cron(0 0-21/3 * * ? *)"
  schedule_expression_timezone = "Asia/Tokyo"

  flexible_time_window {
    mode = "OFF"
  }

  # DMS Serverless レプリケーションを実行
  target {
    arn      = "arn:aws:scheduler:::aws-sdk:databasemigration:startReplication"
    role_arn = aws_iam_role.eventbridge_dms_role.arn

    input = jsonencode({
      ReplicationConfigArn = aws_dms_replication_config.main.arn
      StartReplicationType = "reload-target"  # フルロードを再実行
    })

    retry_policy {
      maximum_event_age_in_seconds = 3600  # 1時間
      maximum_retry_attempts       = 2
    }
  }

  state = "ENABLED"

  depends_on = [
    aws_iam_role_policy_attachment.eventbridge_dms
  ]
}
```

## IAM ロール・ポリシー（AWS）

DMS、EventBridge、GCP Storage Transfer Service 用の IAM を設定します。

```hcl:iam.tf
# ========================================
# DMS 用 IAM ロール
# ========================================

# DMS が VPC リソースを管理するためのロール（固定名称必須）
resource "aws_iam_role" "dms_access_role" {
  name = "dms-vpc-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "dms.amazonaws.com"
        }
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "dms_vpc_management" {
  role       = aws_iam_role.dms_access_role.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonDMSVPCManagementRole"
}

# DMS が CloudWatch ログを書き込むためのロール
resource "aws_iam_role" "dms_cloudwatch_logs_role" {
  name = "dms-cloudwatch-logs-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "dms.amazonaws.com"
        }
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "dms_cloudwatch_logs" {
  role       = aws_iam_role.dms_cloudwatch_logs_role.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonDMSCloudWatchLogsRole"
}

# DMS が S3 バケットへ書き込むためのロール
resource "aws_iam_role" "dms_s3_target_role" {
  name = "sample-dms-s3-target-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "dms.amazonaws.com"
        }
      }
    ]
  })
}

resource "aws_iam_policy" "dms_s3_target_policy" {
  name = "sample-dms-s3-target-policy"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:ListBucket",
          "s3:GetBucketLocation"
        ]
        Resource = aws_s3_bucket.analytics_data.arn
      },
      {
        Effect = "Allow"
        Action = [
          "s3:PutObject",
          "s3:GetObject",
          "s3:DeleteObject",
          "s3:PutObjectAcl"
        ]
        Resource = "${aws_s3_bucket.analytics_data.arn}/*"
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "dms_s3_target" {
  role       = aws_iam_role.dms_s3_target_role.name
  policy_arn = aws_iam_policy.dms_s3_target_policy.arn
}

# ========================================
# EventBridge 用 IAM ロール
# ========================================

resource "aws_iam_role" "eventbridge_dms_role" {
  name = "sample-eventbridge-dms-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = [
            "scheduler.amazonaws.com",
            "events.amazonaws.com"
          ]
        }
        Action = "sts:AssumeRole"
      }
    ]
  })
}

resource "aws_iam_policy" "eventbridge_dms_policy" {
  name        = "sample-eventbridge-dms-policy"
  description = "Policy for EventBridge to start DMS Serverless replications"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "dms:StartReplication",
          "dms:DescribeReplications",
          "dms:StopReplication"
        ]
        Resource = aws_dms_replication_config.main.arn
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "eventbridge_dms" {
  role       = aws_iam_role.eventbridge_dms_role.name
  policy_arn = aws_iam_policy.eventbridge_dms_policy.arn
}

# ========================================
# GCP Storage Transfer Service 用 IAM ロール
# ========================================

# GCP STS が S3 を読み取るためのポリシー
resource "aws_iam_policy" "s3_read_for_gcp_sts" {
  name        = "sample-S3ReadAccessForGCP-STS"
  description = "Allows read-only access to analytics S3 bucket for GCP STS."

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:GetBucketLocation",
          "s3:ListBucket"
        ]
        Resource = aws_s3_bucket.analytics_data.arn
      },
      {
        Effect   = "Allow"
        Action   = "s3:GetObject"
        Resource = "${aws_s3_bucket.analytics_data.arn}/*"
      },
    ]
  })
}

# GCP STS が引き受ける IAM ロール（Web Identity フェデレーション）
resource "aws_iam_role" "gcp_sts_role" {
  name = "sample-GCP-StorageTransferRole"

  # GCP STS サービスアカウントからの AssumeRole を許可
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          "Federated" = "accounts.google.com"
        }
        Action = "sts:AssumeRoleWithWebIdentity"
        Condition = {
          "StringEquals" = {
            "accounts.google.com:sub" = data.google_storage_transfer_project_service_account.default.subject_id
          }
        }
      },
    ]
  })
}

resource "aws_iam_role_policy_attachment" "attach_s3_read_policy" {
  role       = aws_iam_role.gcp_sts_role.name
  policy_arn = aws_iam_policy.s3_read_for_gcp_sts.arn
}
```

## GCS バケット・Storage Transfer Job

S3 から GCS へのデータ転送を設定します。

```hcl:gcs.tf
# GCP Storage Transfer Service のサービスアカウントを取得
data "google_storage_transfer_project_service_account" "default" {
  project = var.gcp_project_id
}

# 転送先の GCS バケット
resource "google_storage_bucket" "analytics_data" {
  name          = "sample-analytics-data-myapp"
  location      = var.gcp_region
  storage_class = "STANDARD"

  versioning {
    enabled = true
  }

  # 7日後にオブジェクトを削除
  lifecycle_rule {
    condition {
      age = 7
    }
    action {
      type = "Delete"
    }
  }

  public_access_prevention = "enforced"
}

# Storage Transfer Service にバケットへの権限を付与
resource "google_storage_bucket_iam_member" "storage_transfer_service_bucket" {
  bucket = google_storage_bucket.analytics_data.name
  role   = "roles/storage.admin"
  member = "serviceAccount:${data.google_storage_transfer_project_service_account.default.email}"
}

# S3 → GCS 転送ジョブ
resource "google_storage_transfer_job" "s3_to_gcs_sync" {
  project     = var.gcp_project_id
  description = "Sync from AWS S3 to GCS every 3 hours"
  status      = "ENABLED"

  transfer_spec {
    # 転送元: AWS S3
    aws_s3_data_source {
      bucket_name = aws_s3_bucket.analytics_data.bucket
      role_arn    = aws_iam_role.gcp_sts_role.arn
    }

    # 転送先: GCS
    gcs_data_sink {
      bucket_name = google_storage_bucket.analytics_data.name
    }

    transfer_options {
      delete_objects_from_source_after_transfer  = false
      overwrite_objects_already_existing_in_sink = true
    }
  }

  schedule {
    schedule_start_date {
      year  = 2025
      month = 1
      day   = 1
    }
    start_time_of_day {
      hours   = 1
      minutes = 0
      seconds = 0
      nanos   = 0
    }
    # 3時間ごとに繰り返し（10800秒）
    repeat_interval = "10800s"
  }
}
```

## BigQuery データセット・External Table

GCS 上の Parquet ファイルを参照する External Table を作成します。

```hcl:bigquery.tf
# BigQuery データセット
resource "google_bigquery_dataset" "analytics_data" {
  dataset_id    = "sample_aurora_sync"
  friendly_name = "Analytics Data from Aurora"
  description   = "Analytics data imported from Aurora database via DMS and GCS"
  location      = var.gcp_region
}

# 対象テーブルの定義
locals {
  tables = [
    "users",
    "orders",
    "products",
    "categories",
    # 必要なテーブルを追加
  ]
}

# 各テーブルの External Table を作成
resource "google_bigquery_table" "external_tables" {
  for_each = toset(local.tables)

  dataset_id  = google_bigquery_dataset.analytics_data.dataset_id
  table_id    = each.key
  description = "External table for ${each.key} data from GCS"

  external_data_configuration {
    autodetect    = true
    source_format = "PARQUET"
    source_uris = [
      "gs://${google_storage_bucket.analytics_data.name}/myapp/${each.key}/LOAD00000001.parquet"
    ]

    parquet_options {
      enable_list_inference = true
      enum_as_string        = true
    }
  }
}

# BigQuery 用のサービスアカウント
resource "google_service_account" "bigquery_service_account" {
  account_id   = "bigquery-analytics-sa"
  display_name = "BigQuery Analytics Service Account"
  description  = "Service account for BigQuery analytics data access"
}

# データセットへのアクセス権限
resource "google_bigquery_dataset_iam_member" "bigquery_dataset_access" {
  dataset_id = google_bigquery_dataset.analytics_data.dataset_id
  role       = "roles/bigquery.dataEditor"
  member     = "serviceAccount:${google_service_account.bigquery_service_account.email}"
}

# GCS バケットからの読み取り権限
resource "google_storage_bucket_iam_member" "bigquery_gcs_access" {
  bucket = google_storage_bucket.analytics_data.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.bigquery_service_account.email}"
}

# BigQuery Job User 権限（クエリ実行用）
resource "google_project_iam_member" "bigquery_job_user" {
  project = var.gcp_project_id
  role    = "roles/bigquery.jobUser"
  member  = "serviceAccount:${google_service_account.bigquery_service_account.email}"
}

# BigQuery Data Viewer 権限（データ読み取り用）
resource "google_project_iam_member" "bigquery_data_viewer" {
  project = var.gcp_project_id
  role    = "roles/bigquery.dataViewer"
  member  = "serviceAccount:${google_service_account.bigquery_service_account.email}"
}
```

# 運用上の注意点

## Full-load のデータ量

本構成では `full-load` を使用しており、毎回テーブル全体のデータを転送します。テーブルサイズが大きい場合、転送時間とコストが増加するため注意が必要です。

## スケジュールのタイミング問題

本サンプルでは EventBridge Scheduler（JST 基準）と Storage Transfer Job（UTC 基準）が独立して動作しています。そのため、DMS の完了前に Storage Transfer が走ると、S3 の最新データが GCS に反映されない可能性があります。

**対策：**

1. **スケジュールをずらす**: DMS 完了後に十分な余裕を持って Storage Transfer を開始する（例: DMS 開始から 1 時間後）
2. **イベント駆動にする**: DMS 完了を EventBridge で検知し、Lambda 経由で Storage Transfer を開始する設計に変更する

# Full-load と CDC（差分同期）について

DMS のレプリケーション方式には Full-load と CDC（Change Data Capture）があります。

## CDC のメリット

CDC は本番運用で推奨される方式です：

- **効率的な転送**: 変更されたデータのみを転送するため、転送量が大幅に削減
- **高いリアルタイム性**: 継続的にデータを同期し、ニアリアルタイムでの分析が可能
- **Aurora への負荷軽減**: 差分のみ取得するため、ソース DB への負荷が低い

## 本サンプルでは Full-load を採用

本サンプルでは簡略化のため `full-load` を設定しています。

| 項目            | Full-load      | CDC                  |
| --------------- | -------------- | -------------------- |
| 転送データ      | 毎回全量       | 変更分のみ           |
| リアルタイム性  | 低（定期実行） | 高（継続的）         |
| 設定の複雑さ    | シンプル       | 複雑                 |
| Aurora 側の要件 | なし           | バイナリログの有効化 |

実運用では CDC の採用を検討してください。CDC を使用する場合は `replication_type` を `full-load-and-cdc` または `cdc` に変更し、Aurora 側でバイナリログを有効化する必要があります。

# まとめ

本記事では、Terraform を使用して Aurora から BigQuery へのデータパイプラインを構築する方法を解説しました。

主なポイント：

- DMS Serverless でサーバーレスなデータ抽出
- Parquet 形式で効率的なデータ保存
- OpenID Connect フェデレーションでセキュアなクロスクラウド連携
- External Table でデータロード不要の分析基盤

この構成により、運用負荷を抑えながら Aurora のデータを BigQuery で分析できる基盤を構築できます。

# 参考リンク

- [AWS DMS Serverless](https://docs.aws.amazon.com/dms/latest/userguide/CHAP_Serverless.html)
- [Google Storage Transfer Service](https://cloud.google.com/storage-transfer/docs/overview)
- [BigQuery External Tables](https://cloud.google.com/bigquery/docs/external-tables)
- [AWS to GCP Workload Identity Federation](https://cloud.google.com/iam/docs/workload-identity-federation-with-other-clouds)
