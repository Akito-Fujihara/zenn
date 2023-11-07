---
title: "EKSで始めるArgo Workflows(Artifact Repository)"
emoji: "✨"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: ["kubernetes", "eks", "argoworkflows", "workflowarchive"]
published: true
---

# 自己紹介
都内SaaS 企業でSRE エンジニア ２年目のFujihara Akitoです。
週一程度簡単なmemo書を残したいと思っています。

# Workflow Archive
Argo Workflows で Workflow の実行結果を永続的に保存したい場合には Workflow Archive を使用することができます。
Postgres または Mysql を使用して Workflow の Status, 実行されたpod, 実行結果などが保存されます。
しかし、Workflow Archive には log 保存の機能は存在しないため Artifact Repository を利用する必要があります。
自分の記事ですが、[EKSで始めるArgo Workflows(Artifact Repository)](https://zenn.dev/fujihara_akito/articles/argo-workflow-eks-artifact-repository) で解説しています。

また以下のようなことが https://argoproj.github.io/argo-workflows/cli/argo_archive に書かれています
- クラウドサービスなどによるIAM認証には対応してないが、データベースプロキシを利用することで接続できる
- Archive を有効にして workflow controller (カスタムコントローラー) を起動するたびにmigrateされ、データベースに argo_workflows などのテーブルが作成される
- garbage collection function (実行済み workflow の実行済み pod などを削除する機能) の周期は ARCHIVED_WORKFLOW_GC_PERIOD(default: 24h) で設定することができる
- `archiveTTL` の設定で　Workflow が garbage collection function によって削除されるまでの期間を設定することができる (defaultは永遠だが、cluster update などで k8s resource 自体が削除された場合は消える)

# EKS & RDS(MySQL) で実装

## Argo Workflows を EKS　にデプロイ
https://zenn.dev/fujihara_akito/articles/argo-workflow-eks-1 を参考にデプロイ
追加で環境変数を設定する
```
$ export TF_VAR_mysql_password=[passwordを設定]
$ export TF_VAR_mysql_username=[usernameを設定]
```

## RDS の作成

https://github.com/Akito-Fujihara/integration-argoworkflows/blob/main/terraform/sg.tf
```
resource "aws_security_group" "rds" {
  name   = "${var.name}-rds"
  vpc_id = module.vpc.vpc_id

  ingress {
    from_port   = 3306
    to_port     = 3306
    protocol    = "tcp"
    security_groups = [module.eks.node_security_group_id]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
```

https://github.com/Akito-Fujihara/integration-argoworkflows/blob/main/terraform/rds.tf
```
resource "aws_db_subnet_group" "rds" {
  name        = "${var.name}-rds"
  description = "rds subnet group for ${var.name}"
  subnet_ids  = module.vpc.private_subnets
}

resource "aws_db_instance" "rds" {
  allocated_storage      = 10
  storage_type           = "gp2"
  engine                 = "mysql"
  engine_version         = "8.0.28"
  instance_class         = "db.t3.micro"
  identifier             = "${var.name}-db"
  username               = var.mysql_username
  password               = var.mysql_password
  vpc_security_group_ids = [aws_security_group.rds.id]
  db_subnet_group_name   = aws_db_subnet_group.rds.name
}
```
RDS が作成されていることが確認できると思います。

## 作成したEKS cluster に password & username の　Secret を作成
```
$ kubectl create secret generic argo-mysql-config --from-literal=password=[passwordを設定] --from-literal=username=[usernameを設定]
```

## Argo Workflows の helm values を修正

https://github.com/Akito-Fujihara/integration-argoworkflows/blob/main/kubernetes/argoworkflows.yaml.gotmpl
```
controller:
  persistence:
    archive: true
    nodeStatusOffLoad: true
    mysql:
      # Change here per cluster
      database: workflow_archive
      host: [作成したrds の hostname]
      port: 3306
      tableName: argo_workflows
      userNameSecret:
        name: argo-mysql-config
        key: username
      passwordSecret:
        name: argo-mysql-config
        key: password
```

## 検証してみる
作成された ALB のAレコードにアクセスして WorkflowTemplate を submit する
![](/images/argo-workflows/workflow-submit.png)

Archived Workflowsを確認
![](/images/argo-workflows/archived-workflows.png)
