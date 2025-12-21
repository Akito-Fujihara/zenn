---
title: "Go + OpenTelemetry でローカル開発環境にトレーシングを導入する"
emoji: "🔍"
type: "tech"
topics: ["go", "opentelemetry", "jaeger", "echo", "gorm"]
published: true
---

# はじめに

本記事では、Go アプリケーション（Echo + GORM）に OpenTelemetry を導入し、Jaeger でトレースを可視化する方法を解説します。

本記事のサンプルコードは以下のリポジトリで公開しています。
https://github.com/Akito-Fujihara/zenn/tree/main/example/opentelemetry-go-local-jaeger

## なぜローカル開発でトレーシングが必要か

- **処理の流れを可視化**: HTTP リクエストからデータベースクエリまでの一連の処理を追跡
- **パフォーマンスボトルネックの特定**: どの処理に時間がかかっているかを視覚的に把握
- **デバッグの効率化**: 複雑な処理フローを理解しやすくなる

# Jaeger のセットアップ

まず、トレースを可視化するための Jaeger をセットアップします。

```yaml:docker-compose.yaml
services:
  jaeger:
    image: jaegertracing/all-in-one:1.54
    ports:
      - "16686:16686"  # Jaeger UI
      - "4317:4317"    # OTLP gRPC
    environment:
      - COLLECTOR_OTLP_ENABLED=true

  mysql:
    image: mysql:8.0
    ports:
      - "3306:3306"
    environment:
      MYSQL_ROOT_PASSWORD: password
      MYSQL_DATABASE: otel_sample
    volumes:
      - mysql_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 5s
      timeout: 5s
      retries: 10

volumes:
  mysql_data:
```

```bash
docker compose up -d
```

Jaeger UI には `http://localhost:16686` でアクセスできます。

# TracerProvider のセットアップ

OpenTelemetry の中心となる TracerProvider を設定します。

```go:tracer/tracer.go
package tracer

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

func InitTracer(serviceName, endpoint string) (func(), error) {
	ctx := context.Background()

	// OTLP gRPC Exporter を作成
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	// リソースの定義（サービス名など）
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// TracerProvider の設定
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	// グローバルに設定
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// クリーンアップ関数を返す
	return func() {
		_ = tp.Shutdown(ctx)
	}, nil
}
```

**ポイント:**
- `otlptracegrpc.New`: Jaeger の OTLP エンドポイントにトレースを送信
- `sdktrace.WithBatcher`: トレースを効率的にバッチ送信
- `propagation.TraceContext{}`: W3C Trace Context 形式でトレースを伝播

# Echo のトレーシング

Echo フレームワークに OpenTelemetry ミドルウェアを追加します。

```go:main.go
package main

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"

	"otel-sample/database"
	"otel-sample/tracer"
)

func main() {
	// TracerProvider を初期化
	cleanup, err := tracer.InitTracer("otel-sample", "localhost:4317")
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	// データベース接続
	dsn := "root:password@tcp(localhost:3306)/otel_sample?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := database.NewDB(dsn)
	if err != nil {
		log.Fatal(err)
	}

	e := echo.New()

	// ミドルウェア
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(otelecho.Middleware("otel-sample"))

	// ルート
	e.GET("/users", func(c echo.Context) error {
		var users []database.User
		if err := db.WithContext(c.Request().Context()).Find(&users).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, users)
	})

	e.GET("/users/:id", func(c echo.Context) error {
		var user database.User
		if err := db.WithContext(c.Request().Context()).First(&user, c.Param("id")).Error; err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
		}
		return c.JSON(http.StatusOK, user)
	})

	e.POST("/users", func(c echo.Context) error {
		var user database.User
		if err := c.Bind(&user); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		if err := db.WithContext(c.Request().Context()).Create(&user).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusCreated, user)
	})

	e.Logger.Fatal(e.Start(":8080"))
}
```

**ポイント:**
- `db.WithContext(c.Request().Context())`: HTTP リクエストのコンテキストを GORM に渡すことで、トレースが連携される

# GORM のトレーシング

GORM にトレーシングプラグインを追加します。

```go:database/database.go
package database

import (
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"
)

type User struct {
	ID    int64  `json:"id" gorm:"primaryKey"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func NewDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	// OpenTelemetry トレーシングプラグインを登録
	if err := db.Use(tracing.NewPlugin(tracing.WithoutMetrics())); err != nil {
		return nil, fmt.Errorf("failed to use tracing: %w", err)
	}

	return db, nil
}
```

これにより、SQL クエリが自動的にスパンとして記録されます。

# 補足: 追加のインストルメンテーション

## HTTP クライアント

外部 API への HTTP リクエストをトレースする場合：

```go
import (
	"net/http"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func NewHTTPClient() *http.Client {
	return &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
}
```

## Redis

Redis 操作をトレースする場合：

```go
import (
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

func NewRedisClient(addr string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	// トレーシングを有効化
	if err := redisotel.InstrumentTracing(client); err != nil {
		return nil, err
	}

	return client, nil
}
```

## AWS SDK

AWS サービス呼び出しをトレースする場合：

```go
import (
	"context"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
)

func NewS3Client() (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, err
	}

	// OpenTelemetry ミドルウェアを追加
	otelaws.AppendMiddlewares(&cfg.APIOptions)

	return s3.NewFromConfig(cfg), nil
}
```

# 動作確認

1. アプリケーションを起動
2. API にリクエストを送信
3. Jaeger UI (`http://localhost:16686`) でトレースを確認

![Jaeger UI でのトレース表示イメージ](/images/jaeger-trace-example.png)

トレースでは以下が確認できます：
- **HTTP リクエスト**: メソッド、パス、ステータスコード、レイテンシ
- **DB クエリ**: 実行された SQL、実行時間
- **外部 API 呼び出し**: リクエスト先、レスポンス時間

# まとめ

OpenTelemetry を Go アプリケーションに導入することで：

1. **TracerProvider** で OTLP gRPC Exporter を設定
2. **Echo** は `otelecho.Middleware` で HTTP リクエストをトレース
3. **GORM** は `tracing.NewPlugin` で SQL クエリをトレース
4. **HTTP/Redis/AWS** も専用のインストルメンテーションライブラリで対応

ローカル開発環境での導入は Jaeger All-in-One を使えば簡単に始められます。

# 参考資料

- [OpenTelemetry Go SDK Documentation](https://opentelemetry.io/docs/languages/go/)
- [Jaeger Documentation](https://www.jaegertracing.io/docs/)
- [otelecho - Echo Instrumentation](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho)
- [GORM OpenTelemetry Plugin](https://github.com/go-gorm/opentelemetry)
