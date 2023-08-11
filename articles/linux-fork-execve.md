---
title: "Linux プロセス fork() & execve()"
emoji: "✨"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: ["linux", "fork", "execve", "process"]
published: true
---

# 自己紹介
都内SaaS 企業でSRE エンジニア ２年目のFujihara Akitoです。
週一程度簡単なmemoを残したいと思っています。

# Linuxのプロセスのfork() & execve() とは？

プロセスとは実行中のプログラムのことです。OSによりメモリ領域などが割り当てられて、CPUとデータのやりとりを行い演算処理をしています。

プロセスの生成には主に二つの目的が存在します。
1. 同じプログラムの処理を複数のプロセスに分けて処理を実行したい場合
2. 別のプログラムを生成したい場合

Linuxのプロセスの生成ではfork()関数とexecve()関数を利用します。

### fork()
fork()は親プロセスをコピーして子プロセスを生成します。
親プロセスからfork()が呼ばれると子プロセス用のメモリ領域を確保して親プロセスのメモリ領域にあるデータをそのままコピー(Copy-on-Writeという機能により非常に軽量な処理)してきます。
その際に以下の情報を親プロセスが引き継ぎます
- 実行状態
- メモリ領域のデータ
- ユーザーID & グループID
- 環境変数
- 作業ディレクトリ
- umask値
- シグナルマスク & シグナルハンドラ
- ファイル記述子
- ディレクトリストリーム

fork()により、同じプログラムに対してプロセスを分けて実行することができます。

また、fork()によりLinuxカーネル内でプロセスの生成が行われてfork()から戻ってきた時には
- 親プロセスの場合は子プロセスのID
- 子プロセスの場合は0
が返します。

### execve()
fork()によって生成された子プロセスはexecve()を発行します。
それによって子プロセスは別のプログラムに置き換えられます。
execve()は呼び出されてから
- 実行ファイルからプログラムを取り込み、メモリ領域を上書きするために必要な情報を取り出す
- 呼び出したプロセスのメモリ領域を上書き
- 新しいプロセスのエントリポイントがら実行を開始

その際に以下の情報を引き継ぎます。
- PID & PPID
- ユーザーID & グループID
- 作業ディレクトリ
- umask値
- シグナルマスク
- ファイル記述子
- プロセスがそれまで使用していた資源

# fork() & execve()の流れ
1. 親プロセスがfork()を実行
2. 親プロセスのメモリ領域をコピーした子プロセスが生成される
3. 生成された子プロセスがexecve()を実行
4. 実行ファイルからプログラムを取り込み子プロセスのメモリ領域を上書き

![](/images/k8s-custom-resource-definitions/fork-execve.drawio.png)

main.py
```
import os, sys

ret = os.fork()
if ret == 0:
    print("PID: %s, PPID: %d" % (os.getpid(), os.getppid()))
    os.execv('/bin/echo', ['/bin/echo', '子プロセス echo'])
    exit()
else:
    print("PPID: %s, PID: %d" % (os.getpid(), ret))
    exit()
sys.exit(1)
```

command
```
❯ python main.py
PPID: 85239, PID: 85240
PID: 85240, PPID: 85239
子プロセス echo
```

# 参考資料
- https://kimamani89.com/2021/02/23/linuxprocess/
- https://endy-tech.hatenablog.jp/entry/system_call_fork_clone_execve 
- https://baubaubau.hatenablog.com/entry/2020/10/16/155512
