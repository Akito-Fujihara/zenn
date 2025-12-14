---
title: "Clean Architecture + GORM で Usecase 層にトランザクションを実装する"
emoji: "🔄"
type: "tech"
topics: ["go", "gorm", "cleanarchitecture", "database"]
published: false
---

# はじめに

本記事では、GORM を使用して **Usecase 層でトランザクションを管理する実装パターン** を解説します。

# なぜ Usecase 層でトランザクションを管理するのか？

## 1. 複数操作の原子性確保

Repository 層で複数エンティティの操作を 1 つのメソッドにまとめると、責務が曖昧になり、再利用性やテスタビリティが低下します。

```go
// NG: Repository 層で複数エンティティの操作を1つのメソッドに混在させる
func (r *OrderRepo) CreateOrder(ctx context.Context, order *Order, userID int64, pointsToUse int32) error {
    return r.db.Transaction(func(tx *gorm.DB) error {
        // 注文を作成
        if err := tx.Create(order).Error; err != nil {
            return err
        }

        // ポイントを更新（別エンティティの操作が混在）
        if err := tx.Model(&Point{}).Where("user_id = ?", userID).
            Update("amount", gorm.Expr("amount - ?", pointsToUse)).Error; err != nil {
            return err
        }

        return nil
    })
}

// 問題点:
// 1. OrderRepo が Point エンティティを直接操作している（責務の逸脱）
// 2. ポイント操作のロジックが再利用できない
// 3. テストが困難（Order と Point の両方をセットアップ必要）
// 4. ビジネスロジックが Repository 層に漏れ出している
```

Usecase 層でトランザクションを管理することで、各 Repository は単一エンティティの責務に集中でき、複数エンティティにまたがる整合性制御を適切に分離できます。

## 2. ビジネスロジックとデータ整合性の一致

Clean Architecture では、ビジネスルールは Usecase 層に集約されます。トランザクションの範囲（どの操作を不可分にするか）はビジネス要件そのものであり、Usecase 層で定義することで、ビジネス上の整合性とデータの整合性を一致させられます。

## 3. 柔軟な制御

「行ロック（`FOR UPDATE`）が必要」「読み取りだけなのでトランザクション不要」といったユースケース固有の要件に対応できます。Repository 層で固定的に管理するのではなく、各 Usecase の要件に応じた最適なトランザクション戦略を実装できます。

# アーキテクチャ設計

トランザクション管理を Usecase 層と Infrastructure 層に分割します。

```
┌─────────────────────────────────────────────────────────┐
│                     Usecase 層                          │
│  - どの処理に整合性を保つか（トランザクション範囲）      │
│  - 処理の実行順序                                       │
│  - エラー時のロールバック判断                           │
└─────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────┐
│                    Domain 層                            │
│  - ITransactionRepository インターフェース              │
│  - Repository インターフェース（WithTx メソッド含む）   │
└─────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────┐
│                Infrastructure 層                        │
│  - トランザクション開始、コミット、ロールバック         │
│  - GORM の具体的な実装                                  │
└─────────────────────────────────────────────────────────┘
```

**依存関係の方向**: Usecase 層 → Domain 層（インターフェース） ← Infrastructure 層（実装）

これにより、Usecase 層は GORM に直接依存せず、インターフェースを通じてトランザクションを制御できます。

# GORM での実装

## Domain 層 - インターフェース定義

まず、Domain 層にトランザクション用のインターフェースを定義します。

```go
// domain/repository/repository.go
package repository

import (
    "context"

    "gorm.io/gorm"
)

// トランザクション管理用インターフェース
type ITransactionRepository interface {
    Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error
}

// 各 Repository インターフェースには WithTx メソッドを定義
type OrderRepository interface {
    Create(ctx context.Context, order *model.Order) (int64, error)
    WithTx(tx *gorm.DB) OrderRepository
}

type PointRepository interface {
    Subtract(ctx context.Context, userID int64, amount int32) error
    WithTx(tx *gorm.DB) PointRepository
}
```

:::message
**ポイント**: `WithTx` メソッドにより、同じインターフェースでトランザクション内外の両方に対応できます。
:::

:::message alert
**トレードオフについて**: 本記事では Domain 層のインターフェースに `gorm.DB` 型が現れています。Clean Architecture の厳密な解釈では、Domain 層は Infrastructure 層（GORM）に依存すべきではありません。

完全に分離する場合は、独自の `Transaction` インターフェースを定義する方法もありますが、実装の複雑さが増します。本記事では実用性を優先し、GORM への依存を許容しています。プロジェクトの規模やチームの方針に応じて判断してください。
:::

## Infrastructure 層 - 実装

次に、Infrastructure 層でこれらのインターフェースを実装します。

### TransactionRepository の実装

```go
// infra/mysql/repository/transaction.go
package repository

import (
    "context"

    domainRepo "your-app/domain/repository"
    "gorm.io/gorm"
)

type TransactionRepository struct {
    db *gorm.DB
}

func NewTransactionRepository(db *gorm.DB) domainRepo.ITransactionRepository {
    return &TransactionRepository{db: db}
}

func (r *TransactionRepository) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
    return r.db.WithContext(ctx).Transaction(fn)
}
```

GORM の `Transaction` メソッドは、渡された関数が `nil` を返せばコミット、`error` を返せばロールバックを自動で行います。

### WithTx パターンの実装

各 Repository に `WithTx` メソッドを実装します。

```go
// infra/mysql/repository/order.go
package repository

import (
    "context"

    domainRepo "your-app/domain/repository"
    "your-app/domain/model"
    "gorm.io/gorm"
)

type OrderRepository struct {
    db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) domainRepo.OrderRepository {
    return &OrderRepository{db: db}
}

// WithTx: トランザクション内で使用する Repository を返す
func (r *OrderRepository) WithTx(tx *gorm.DB) domainRepo.OrderRepository {
    return &OrderRepository{db: tx}
}

func (r *OrderRepository) Create(ctx context.Context, order *model.Order) (int64, error) {
    result := r.db.WithContext(ctx).Create(order)
    return order.ID, result.Error
}

// ... 他のメソッド
```

## Usecase 層 - トランザクションの利用

Usecase 層でトランザクションを使用します。

```go
// usecase/order.go
package usecase

import (
    "context"
    "fmt"

    "your-app/domain/repository"
    "gorm.io/gorm"
)

type OrderUsecase struct {
    txRepo    repository.ITransactionRepository
    orderRepo repository.OrderRepository
    pointRepo repository.PointRepository
}

func NewOrderUsecase(
    txRepo repository.ITransactionRepository,
    orderRepo repository.OrderRepository,
    pointRepo repository.PointRepository,
) *OrderUsecase {
    return &OrderUsecase{
        txRepo:    txRepo,
        orderRepo: orderRepo,
        pointRepo: pointRepo,
    }
}
```

# 実際のユースケース例

NG 例で示した「注文作成 + ポイント消費」を、Usecase 層でトランザクション管理する形に改善します。

```go
func (u *OrderUsecase) CreateOrder(ctx context.Context, order *model.Order, pointsToUse int32) error {
    return u.txRepo.Transaction(ctx, func(tx *gorm.DB) error {
        // 1. 注文を作成
        _, err := u.orderRepo.WithTx(tx).Create(ctx, order)
        if err != nil {
            return fmt.Errorf("failed to create order: %w", err)
        }

        // 2. ポイントを消費
        if pointsToUse > 0 {
            err = u.pointRepo.WithTx(tx).Subtract(ctx, order.UserID, pointsToUse)
            if err != nil {
                return fmt.Errorf("failed to subtract points: %w", err)
            }
        }

        return nil
    })
}
```

この実装では：

- **各 Repository は単一エンティティの責務に集中** - OrderRepository は Order のみ、PointRepository は Point のみを操作
- **トランザクションの範囲は Usecase 層で制御** - どの操作を不可分にするかはビジネス要件に基づいて決定
- **再利用性が高い** - 各 Repository のメソッドは他の Usecase からも利用可能
- **テストが容易** - 各 Repository を個別にモック可能

# まとめ

Clean Architecture + GORM でのトランザクション管理のポイント：

1. **Domain 層**に `ITransactionRepository` インターフェースを定義
2. 各 Repository インターフェースに **`WithTx` メソッド**を追加
3. **Infrastructure 層**で GORM を使った実装を提供
4. **Usecase 層**でトランザクションの範囲と実行順序を制御

この設計により、ビジネスロジック層でデータの整合性を制御しつつ、Infrastructure 層の実装詳細を隠蔽できます。

# 参考資料

- [Go でクリーンアーキテクチャにおけるトランザクション処理](https://zenn.dev/cloud_ace/articles/transaction-architecture)
- [GORM Transaction ドキュメント](https://gorm.io/docs/transactions.html)
