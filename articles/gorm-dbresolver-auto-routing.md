---
title: "GORM で Reader/Writer を自動判定する仕組み"
emoji: "🔀"
type: "tech"
topics: ["go", "gorm", "database", "mysql"]
published: false
---

# はじめに

GORM で読み取りレプリカ（Reader）と書き込みソース（Writer）を分離したい場合、[DBResolver](https://github.com/go-gorm/dbresolver) プラグインを使っています。
DBResolver の優れた点は、**明示的に指定しなくても、クエリの種類に応じて自動的に Reader/Writer を切り替えてくれる**ことです。

# DBResolver の基本的な使い方

まず、基本的な使い方を見てみましょう。

```go
import (
    "gorm.io/driver/mysql"
    "gorm.io/gorm"
    "gorm.io/plugin/dbresolver"
)

db, err := gorm.Open(mysql.Open(writerDSN), &gorm.Config{})

// DBResolver を登録
db.Use(dbresolver.Register(dbresolver.Config{
    Sources:  []gorm.Dialector{mysql.Open(writerDSN)},  // Writer
    Replicas: []gorm.Dialector{mysql.Open(readerDSN)},  // Reader
    Policy:   dbresolver.RandomPolicy{},
}))

// 以降、自動的に振り分けられる
db.Find(&users)        // → Reader
db.Create(&user)       // → Writer
db.Raw("SELECT ...").Scan(&result)  // → Reader（自動判定）
```

このように、**設定するだけで特別な指定なしに自動振り分けが行われます**。では、どのような仕組みで実現されているのでしょうか？

# DBResolver の自動振り分けの仕組み

DBResolver は GORM の **Callback システム** を利用して、各操作の前に適切な DB への接続を切り替えています。

## Callback の登録

DBResolver は初期化時に以下の Callback を登録します：

```go
func (dr *DBResolver) registerCallbacks(db *gorm.DB) {
    dr.Callback().Create().Before("*").Register("gorm:db_resolver", dr.switchSource)
    dr.Callback().Query().Before("*").Register("gorm:db_resolver", dr.switchReplica)
    dr.Callback().Update().Before("*").Register("gorm:db_resolver", dr.switchSource)
    dr.Callback().Delete().Before("*").Register("gorm:db_resolver", dr.switchSource)
    dr.Callback().Row().Before("*").Register("gorm:db_resolver", dr.switchReplica)
    dr.Callback().Raw().Before("*").Register("gorm:db_resolver", dr.switchGuess)
}
```

各操作に対して以下のように振り分けられます：

| 操作   | 呼び出される関数 | 振り分け先             |
| ------ | ---------------- | ---------------------- |
| Create | `switchSource`   | Writer                 |
| Update | `switchSource`   | Writer                 |
| Delete | `switchSource`   | Writer                 |
| Query  | `switchReplica`  | Reader（条件による）   |
| Row    | `switchReplica`  | Reader（条件による）   |
| Raw    | `switchGuess`    | **SQL を解析して判定** |

## switchSource: 書き込み操作は Writer へ

Create/Update/Delete 操作は単純に Writer へ振り分けます：

```go
func (dr *DBResolver) switchSource(db *gorm.DB) {
    if !isTransaction(db.Statement.ConnPool) {
        db.Statement.ConnPool = dr.resolve(db.Statement, Write)
    }
}
```

**トランザクション中でなければ**、Writer への接続に切り替えます。

## switchReplica: クエリ操作の賢い振り分け

Query/Row 操作は少し複雑です：

```go
func (dr *DBResolver) switchReplica(db *gorm.DB) {
    if !isTransaction(db.Statement.ConnPool) {
        if rawSQL := db.Statement.SQL.String(); len(rawSQL) > 0 {
            // Raw SQL が存在する場合は switchGuess で判定
            dr.switchGuess(db)
        } else {
            _, locking := db.Statement.Clauses["FOR"]
            if _, ok := db.Statement.Settings.Load(writeName); ok || locking {
                // FOR UPDATE などのロック句がある場合は Writer
                db.Statement.ConnPool = dr.resolve(db.Statement, Write)
            } else {
                // それ以外は Reader
                db.Statement.ConnPool = dr.resolve(db.Statement, Read)
            }
        }
    }
}
```

振り分けロジック：

1. **Raw SQL がある場合** → `switchGuess` で判定
2. **`FOR UPDATE` などのロック句がある場合** → Writer
3. **明示的に `.Clauses(dbresolver.Write)` が指定されている場合** → Writer
4. **それ以外** → Reader

## switchGuess: Raw SQL の自動判定（重要）

**Raw SQL の場合、SQL 文の内容を解析して自動判定します**：

```go
func (dr *DBResolver) switchGuess(db *gorm.DB) {
    if !isTransaction(db.Statement.ConnPool) {
        if _, ok := db.Statement.Settings.Load(writeName); ok {
            // 明示的に Write 指定
            db.Statement.ConnPool = dr.resolve(db.Statement, Write)
        } else if _, ok := db.Statement.Settings.Load(readName); ok {
            // 明示的に Read 指定
            db.Statement.ConnPool = dr.resolve(db.Statement, Read)
        } else if rawSQL := strings.TrimSpace(db.Statement.SQL.String());
                  len(rawSQL) > 10 &&
                  strings.EqualFold(rawSQL[:6], "select") &&
                  !strings.EqualFold(rawSQL[len(rawSQL)-10:], "for update") {
            // SELECT 文で、末尾が FOR UPDATE でなければ Reader
            db.Statement.ConnPool = dr.resolve(db.Statement, Read)
        } else {
            // それ以外は Writer
            db.Statement.ConnPool = dr.resolve(db.Statement, Write)
        }
    }
}
```

判定ロジック：

1. **明示的に指定されている場合** → その指定に従う
2. **SQL が `SELECT` で始まり、末尾が `FOR UPDATE` でない場合** → Reader
3. **それ以外（INSERT, UPDATE, DELETE など）** → Writer

この仕組みにより、以下のような Raw SQL も自動で適切に振り分けられます：

```go
// Reader へ
db.Raw("SELECT * FROM users WHERE id = ?", 1).Scan(&user)

// Writer へ（FOR UPDATE が含まれる）
db.Raw("SELECT * FROM users WHERE id = ? FOR UPDATE", 1).Scan(&user)

// Writer へ（INSERT 文）
db.Exec("INSERT INTO users (name) VALUES (?)", "Alice")
```

## トランザクション中は振り分けをスキップ

全ての関数で `isTransaction` チェックが行われていることに注目してください：

```go
func isTransaction(connPool gorm.ConnPool) bool {
    _, ok := connPool.(gorm.TxCommitter)
    return ok
}
```

**トランザクション中は既に接続が確定しているため、振り分け処理をスキップします**。これにより、トランザクション内での一貫性が保たれます。

# 実際の使用例

実際のプロジェクトでの DBResolver の使い方を見てみましょう。

```go
package mysql

import (
    "fmt"
    "gorm.io/driver/mysql"
    "gorm.io/gorm"
    "gorm.io/plugin/dbresolver"
)

type Config struct {
    Database         string
    Host             string
    ReadonlyHost     string
    Port             int
    Username         string
    ReadonlyUsername string
    Password         string
    ReadonlyPassword string
}

func NewMysqlConn(config *Config) (*gorm.DB, error) {
    // Writer 用の DSN
    writerDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Asia%%2FTokyo",
        config.Username,
        config.Password,
        config.Host,
        config.Port,
        config.Database,
    )

    // Reader 用の DSN
    readerDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Asia%%2FTokyo",
        config.ReadonlyUsername,
        config.ReadonlyPassword,
        config.ReadonlyHost,
        config.Port,
        config.Database,
    )

    // GORM 接続を開く
    db, err := gorm.Open(mysql.Open(writerDSN), &gorm.Config{})
    if err != nil {
        return nil, fmt.Errorf("failed to connect to database: %v", err)
    }

    // DBResolver を登録
    if err := db.Use(dbresolver.Register(dbresolver.Config{
        Sources:  []gorm.Dialector{mysql.Open(writerDSN)},
        Replicas: []gorm.Dialector{mysql.Open(readerDSN)},
        Policy:   dbresolver.RandomPolicy{},
    })); err != nil {
        return nil, fmt.Errorf("failed to register dbresolver: %v", err)
    }

    return db, nil
}
```

この実装により、以降のコードでは以下のように自動振り分けが行われます：

```go
// Reader へ
db.Find(&users)
db.Where("active = ?", true).First(&user)
db.Raw("SELECT COUNT(*) FROM users").Scan(&count)

// Writer へ
db.Create(&user)
db.Model(&user).Update("name", "Bob")
db.Delete(&user)
db.Exec("UPDATE users SET status = ? WHERE id = ?", "active", 1)
```

# まとめ

GORM の DBResolver は以下の仕組みで自動的に Reader/Writer を振り分けています：

1. **Callback システム**を利用して各操作の前に振り分け処理を実行
2. **Create/Update/Delete** は常に Writer へ
3. **Query/Row** は基本的に Reader へ（`FOR UPDATE` などの例外あり）
4. **Raw SQL** は内容を解析して判定（`switchGuess` 関数）
   - `SELECT` 文で `FOR UPDATE` でなければ Reader
   - それ以外は Writer
5. **トランザクション中**は振り分けをスキップして既存の接続を使用

この仕組みにより、**開発者は明示的に Reader/Writer を指定する必要がなく**、GORM が自動的に最適な接続を選択してくれます。

# 参考資料

- [GORM DBResolver 公式ドキュメント](https://gorm.io/docs/dbresolver.html)
- [go-gorm/dbresolver GitHub リポジトリ](https://github.com/go-gorm/dbresolver)
- [callbacks.go - ソースコード](https://github.com/go-gorm/dbresolver/blob/master/callbacks.go)
