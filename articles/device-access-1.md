---
title: "デバイスアクセス①(デバイスファイル & デバイスドライバ)"
emoji: "✨"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: ["linux", "deviceaccess"]
published: true
---

# 自己紹介
都内SaaS 企業でSRE エンジニア ２年目のFujihara Akitoです。
週一程度簡単なmemo書を残したいと思っています。

# プロセスがデバイスにアクセスするまでのフロー
1. プロセスがデバイスファイルを介してデバイスドライバでデバイスにアクセスしにいく
2. CPUがカーネルモードに切り替わり、デバイスドライバがデバイスのレジスタを介してデバイスを操作
3. デバイスが処理を実行
4. デバイスドライバが処理を検出して受け取る
5. CPUがユーザモードに切り替わりプロセスがデバイスドライバの処理を受け取る

![](/images/device-access-overview.drawio.png)

## デバイスファイル
Linuxではデバイスファイルはデバイス毎に存在しており、デバイスファイルを操作するとカーネルの中のデバイスドライバというソフトウェアがユーザの代わりにデバイスにアクセスします。
プロセスは通常のファイルと同じようにデバイスファイルを操作することができ、open(), read(), write()のシステムコールの発行によりそれぞれのデバイスにアクセスすることができる。

※EC2のubuntu amiを使用

実際にデバイスファイルを見てみると、

```
$ ls -l /dev/
total 0
crw-r--r-- 1 root root     10, 235 Sep  9 05:25 autofs
drwxr-xr-x 2 root root         280 Sep  9 05:25 block
crw------- 1 root root     10, 234 Sep  9 05:25 btrfs-control
drwxr-xr-x 2 root root        2620 Sep  9 05:25 char
crw--w---- 1 root tty       5,   1 Sep  9 05:25 console
```
頭文字が
- cならキャラクタデバイス
- bならブロックデバイス

実際にデバイスファイルを触ってみる
```
$ ps ax | grep bash
・・・・
   3970 pts/3    Ss+    0:00 /home/ubuntu/.c9/bin/tmux -u2 -L cloud92.2 new -s cloud9_terminal_48 export ISOUTPUTPANE=0;bash -l ; set -q -g status off ; set -q destroy-unattached off ; set -q mouse-select-pane on ; set -q set-titles on ; set -q quiet on ; set -q -g prefix C-b ; set -q -g default-terminal xterm-256color ; setw -q -g xterm-keys on
   3972 pts/4    Ss     0:00 bash -l
   4170 pts/4    R+     0:00 grep bash
```
bash が /dev/pts/4を利用していることがわかった
/dev/pts/4 ファイルを触ってみると
```
$ sudo su

# echo hello world! > /dev/pts/4
hello world!
```

別のbashを開いて /dev/pts/7 を操作してみる
```
$ ps ax | grep bash
  19433 pts/6    Ss+    0:00 /home/ubuntu/.c9/bin/tmux -u2 -L cloud92.2 new -s cloud9_terminal_444 export ISOUTPUTPANE=0;bash -l ; set -q -g status off ; set -q destroy-unattached off ; set -q mouse-select-pane on ; set -q set-titles on ; set -q quiet on ; set -q -g prefix C-b ; set -q -g default-terminal xterm-256color ; setw -q -g xterm-keys on
  19435 pts/7    Ss     0:00 bash -l
  24836 pts/7    R+     0:00 grep bash

$ sudo su
# echo hello world! > /dev/pts/4
#
```
デバイスファイルが操作されたので元のbashで
```
$ hello world!
```
が出力される

## デバイスドライバ
デバイスドライバは制御対象のデバイスを適切にコントロールし、ハードウェアが提供する機能を実行するものです。
デバイスを操作するためには各デバイスに内蔵されたレジスタを操作する必要がありますが、その仕様はデバイスによって異なります。
Linuxではアプリケーションとカーネル間のインターフェイスの取り決めとしてAPIを定義しているため同じプログラムで違ったプログラムを操作することができるようになっています。

![](/images/device-access-detail.drawio.png)
