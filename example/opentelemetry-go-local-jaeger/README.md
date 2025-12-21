# OpenTelemetry Go Sample with Echo + GORM

## セットアップ

### 1. コンテナ起動

```bash
docker compose up -d
```

### 2. MySQL の起動確認

```bash
docker compose exec mysql mysqladmin ping -h localhost -u root -ppassword
```

### 3. マイグレーション実行

```bash
docker compose exec mysql mysql -u root -ppassword otel_sample < migrations/001_create_users.sql
```

または直接:

```bash
docker compose exec -T mysql mysql -u root -ppassword otel_sample <<EOF
CREATE TABLE IF NOT EXISTS users (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
EOF
```

### 4. アプリケーション起動

```bash
go run main.go
```

## 動作確認

### ユーザー作成

```bash
curl -X POST http://localhost:8080/users \
  -H "Content-Type: application/json" \
  -d '{"name":"test","email":"test@example.com"}'
```

### ユーザー一覧取得

```bash
curl http://localhost:8080/users
```

### ユーザー取得

```bash
curl http://localhost:8080/users/1
```

## トレース確認

Jaeger UI: http://localhost:16686

1. Service で `otel-sample` を選択
2. Find Traces をクリック
3. トレースをクリックして詳細を確認

## 停止

```bash
docker compose down
```

データも削除する場合:

```bash
docker compose down -v
```
