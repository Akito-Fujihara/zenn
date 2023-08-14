---
title: "Copy-on-Write とは"
emoji: "✨"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: ["linux", "copyonwrite", "process"]
published: true
---

# 自己紹介
都内SaaS 企業でSRE エンジニア ２年目のFujihara Akitoです。
週一程度簡単なmemo書を残したいと思っています。

# Copy-on-Write(CoW)　とは
Copy-on-Write(CoW)とは[fork()](https://zenn.dev/fujihara_akito/articles/linux-fork-execve)発行時に親プロセスのメモリを子プロセスにそのままコピーするのではなく、ページテーブルのみをコピーすることで親プロセスと子プロセスが同じ物理メモリを共有する方法です。
Copy-on-Writeを理解するために仮想記憶(仮想メモリ)についてのおさらいをしていきます。

### 仮想記憶(仮想メモリ)
仮想記憶(仮想メモリ)とは補助記憶領域(HDD, SSDなど)の一部を仮想的に主記憶(Memory)のように扱うための技術です。

実際の処理フロー(今回はページテーブル方式)が概略図は以下です。

![](/images/k8s-custom-resource-definitions/virtual-memory.drawio.png)

処理フローの例
1. プログラムがData 3にアクセスしようとする
2. Data3 は フラグ0(補助記憶にDataがある状態) であることを確認
3. 使用しないData5をページアウト(主記憶 > 補助記憶)にして主記憶の空き容量を作る
4. Data3をページイン(補助記憶 > 主記憶)にする
5. Data3を読み取って処理することができる

### Copy-on-Writeの流れ
親プロセスと子プロセスはともに共有された物理アドレスのメモリにアクセスできます。
ただ、どちらかがデータに書き込みを行う際にはそれぞれのプロセス毎に物理メモリの領域を持つことになります

処理フローの概略図です。

![](/images/k8s-custom-resource-definitions/copy-on-write.drawio.png)

1. どちらかのプロセスが書き込みを行おうとする
2. ページフォールト(プログラム実行時に必要なページが存在しない時に発生)
3. 書き込もうとしたページをコピーする
4. ページテーブルを変更する

そのためCopy-on-Writeのおかげで
- fork()の高速化
- メモリの使用量を減らせる
    - 実際にプロセスが生成されてから全てのメモリをwirteにすることはほとんどない

などのメリットが挙げられる
