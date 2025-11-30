---
title: "GPT-2 で理解する Transformer の入出力 - データ構造から読み解く仕組み"
emoji: "🤖"
type: "tech"
topics: ["transformer", "gpt2", "nlp", "python", "huggingface"]
published: true
---

# はじめに

この記事では、OpenAI の **GPT-2** モデルを使って、Transformer の入出力を **データ構造** の観点から理解していきます。実際にコードを動かしながら、テキストがどのように処理されて「次の単語」を予測するのかを追いかけてみましょう。

# Transformer とは？

## Transformer の誕生

Transformer は、2017 年に Google が発表した論文「[Attention Is All You Need](https://arxiv.org/abs/1706.03762)」で提案されたモデルアーキテクチャです。それまでの RNN（再帰型ニューラルネットワーク）に代わり、**Attention（注意機構）** を中心に据えた設計が特徴です。

## GPT-2 とは？

GPT-2（Generative Pre-trained Transformer 2）は、OpenAI が 2019 年に公開した言語モデルです。大量のテキストデータで事前学習されており、**次の単語を予測する** タスクに特化しています。

# 環境セットアップ

## 必要なライブラリ

```python
import torch
from transformers import GPT2Tokenizer, GPT2Model, GPT2LMHeadModel
import matplotlib.pyplot as plt
import numpy as np
```

主なライブラリ：

- **transformers**: Hugging Face が提供する Transformer モデルのライブラリ
- **torch**: PyTorch（深層学習フレームワーク）
- **matplotlib**: グラフ描画ライブラリ

## GPT-2 モデルのロード

```python
# トークナイザーとモデルをロード
tokenizer = GPT2Tokenizer.from_pretrained('gpt2')
model = GPT2Model.from_pretrained('gpt2', attn_implementation="eager")
model_lm = GPT2LMHeadModel.from_pretrained('gpt2', attn_implementation="eager")

# 推論モードに設定（学習はしない）
model.eval()
model_lm.eval()
```

ここでは 2 種類のモデルをロードしています：

- **GPT2Model**: Transformer の基本モデル（隠れ状態を出力）
- **GPT2LMHeadModel**: 言語モデル用（次の単語の確率を出力）

## GPT-2 の基本スペック

```python
config = model.config
print(f"語彙サイズ: {config.vocab_size}")
print(f"隠れ層の次元: {config.n_embd}")
print(f"Transformer レイヤー数: {config.n_layer}")
print(f"Attention ヘッド数: {config.n_head}")
print(f"最大シーケンス長: {config.n_positions}")
```

| 項目         | 値    | 説明                                                      |
| ------------ | ----- | --------------------------------------------------------- |
| vocab_size   | 50257 | 語彙数（モデルが知っている「単語」の数）                  |
| hidden_size  | 768   | 内部表現の次元数（各トークンを 768 次元のベクトルで表現） |
| n_layer      | 12    | Transformer ブロックの層数                                |
| n_head       | 12    | Attention ヘッドの数（異なる観点で文脈を捉える）          |
| max_position | 1024  | 一度に処理できる最大トークン数                            |

# Tokenizer: テキストを数字に変換する

## なぜ数字に変換するのか？

コンピュータは文字列をそのまま理解できません。数学的な演算を行うためには、テキストを **数字の配列** に変換する必要があります。この変換を行うのが **Tokenizer（トークナイザー）** です。

## トークン化の流れ

テキストからモデルへの入力までの流れは以下の通りです：

```
テキスト → トークン（部分文字列） → Token ID（数字）
```

実際に見てみましょう：

```python
text = "Hello, how are you?"

# テキストをトークンに分割
tokens = tokenizer.tokenize(text)
print(f"元のテキスト: {text}")
print(f"トークン列: {tokens}")
print(f"トークン数: {len(tokens)}")
```

出力：

```
元のテキスト: Hello, how are you?
トークン列: ['Hello', ',', ' how', ' are', ' you', '?']
トークン数: 6
```

GPT-2 は **BPE（Byte Pair Encoding）** というアルゴリズムでトークン化を行います。単語単位ではなく、よく出現する文字列のパターンを学習して分割するため、「how」が「 how」（先頭にスペース付き）のようになることがあります。

## Token ID への変換

```python
# トークンを Token ID に変換
token_ids = tokenizer.encode(text)
print(f"Token IDs: {token_ids}")

# 各トークンと ID の対応
for token, token_id in zip(tokens, token_ids):
    print(f"  '{token}' -> {token_id}")
```

出力：

```
Token IDs: [15496, 11, 703, 389, 345, 30]
  'Hello' -> 15496
  ',' -> 11
  ' how' -> 703
  ' are' -> 389
  ' you' -> 345
  '?' -> 30
```

各トークンには一意の ID（0 〜 50256 の整数）が割り当てられています。

## モデル入力用の形式

実際にモデルに入力する際は、`tokenizer()` を直接呼び出すのが便利です：

```python
inputs = tokenizer(text, return_tensors='pt')

print(f"input_ids: {inputs['input_ids']}")
print(f"input_ids.shape: {inputs['input_ids'].shape}")  # (batch_size, sequence_length)
print(f"attention_mask: {inputs['attention_mask']}")
```

出力：

```
input_ids: tensor([[15496,    11,   703,   389,   345,    30]])
input_ids.shape: torch.Size([1, 6])  # (バッチサイズ=1, シーケンス長=6)
attention_mask: tensor([[1, 1, 1, 1, 1, 1]])
```

- **input_ids**: Token ID のテンソル（2 次元配列）
- **attention_mask**: 各トークンを Attention 計算に含めるかどうか（1 = 含める）

# Embedding: 数字をベクトルに変換する

## なぜベクトルにするのか？

Token ID は単なる整数で、数字自体には意味がありません。例えば、「cat」が 100 で「dog」が 200 だとしても、200 が 100 より「大きい」わけではありません。

そこで、各トークンを **高次元のベクトル（768 次元）** に変換します。このベクトルは「単語の意味」を数学的に表現しており、似た意味の単語は近いベクトルになるよう学習されています。

## トークン埋め込みと位置埋め込み

GPT-2 では 2 種類の埋め込み（Embedding）を使います：

```python
# トークン埋め込み層
print(f"トークン埋め込み (wte): {model.wte.weight.shape}")  # (50257, 768)

# 位置埋め込み層
print(f"位置埋め込み (wpe): {model.wpe.weight.shape}")  # (1024, 768)
```

| 埋め込み                      | 形状         | 役割                                   |
| ----------------------------- | ------------ | -------------------------------------- |
| wte (Word Token Embedding)    | (50257, 768) | 各トークンの「意味」を表すベクトル     |
| wpe (Word Position Embedding) | (1024, 768)  | 各位置の「文中での場所」を表すベクトル |

## 埋め込みの計算

最終的な入力は、トークン埋め込みと位置埋め込みを **足し合わせて** 作ります：

```python
text = "Hello world"
inputs = tokenizer(text, return_tensors='pt')
input_ids = inputs['input_ids']

# トークン埋め込みを取得
token_embeddings = model.wte(input_ids)
print(f"トークン埋め込み: {token_embeddings.shape}")  # (1, 2, 768)

# 位置 ID を作成
seq_length = input_ids.shape[1]
position_ids = torch.arange(seq_length).unsqueeze(0)  # [[0, 1]]

# 位置埋め込みを取得
position_embeddings = model.wpe(position_ids)
print(f"位置埋め込み: {position_embeddings.shape}")  # (1, 2, 768)

# 最終的な入力 = トークン埋め込み + 位置埋め込み
final_embeddings = token_embeddings + position_embeddings
print(f"最終入力: {final_embeddings.shape}")  # (1, 2, 768)
```

この `final_embeddings` が Transformer ブロックへの入力となります。

# モデルの出力: 次の単語を予測する

## 基本モデルの出力

まずは基本モデル（GPT2Model）の出力を確認しましょう：

```python
text = "The quick brown fox"
inputs = tokenizer(text, return_tensors='pt')

with torch.no_grad():  # 勾配計算をオフ（推論時）
    outputs = model(**inputs)

print(f"入力形状: {inputs['input_ids'].shape}")  # (1, 4)
print(f"出力形状: {outputs.last_hidden_state.shape}")  # (1, 4, 768)
```

出力：

```
入力形状: torch.Size([1, 4])  # (バッチサイズ=1, トークン数=4)
出力形状: torch.Size([1, 4, 768])  # (バッチ, シーケンス, 隠れ次元)
```

**last_hidden_state** は、各トークンの **文脈を考慮した表現** です。入力時の埋め込みは単語単独の意味でしたが、Transformer を通過することで周囲の単語の情報が反映されます。

## 言語モデルの出力（Logits）

次に、言語モデル（GPT2LMHeadModel）の出力を見てみましょう：

```python
with torch.no_grad():
    outputs_lm = model_lm(**inputs)

print(f"logits 形状: {outputs_lm.logits.shape}")  # (1, 4, 50257)
```

出力：

```
logits 形状: torch.Size([1, 4, 50257])  # (バッチ, シーケンス, 語彙サイズ)
```

**logits** は、各位置での「次に来る単語の確率（未正規化）」を表します。最後の次元が 50257 なのは、語彙にある全単語それぞれのスコアを持っているためです。

## 次の単語を予測してみる

「The quick brown fox」の続きを予測してみましょう：

```python
# 最後の位置の logits を取得
last_logits = outputs_lm.logits[0, -1, :]  # (50257,)

# softmax で確率に変換
probs = torch.softmax(last_logits, dim=-1)

# 上位 5 件を表示
top_k = 5
top_probs, top_ids = torch.topk(probs, top_k)

print(f"'{text}' の次に来る可能性が高い単語:")
for prob, token_id in zip(top_probs, top_ids):
    token = tokenizer.decode([token_id])
    print(f"  '{token}': {prob.item():.4f}")
```

出力例：

```
'The quick brown fox' の次に来る可能性が高い単語:
  ' jumps': 0.1842
  ' jumped': 0.0821
  ',': 0.0534
  ' is': 0.0312
  ' runs': 0.0287
```

「The quick brown fox **jumps** over the lazy dog」という有名なフレーズを学習しているため、「jumps」が最も確率が高くなっています。

# Attention: どこに注目しているか

## Attention とは？

Attention（注意機構）は、Transformer の核となる仕組みです。文中の各単語が、**他のどの単語に注目しているか** を計算します。

例えば、「The cat sat on the mat」という文で「sat」という単語を処理するとき、Attention によって「cat」に強く注目することで、「誰が座ったのか」という情報を取り込むことができます。

## Attention Weights の取得

Attention の重みを可視化してみましょう：

```python
text = "The cat sat on the mat"
inputs = tokenizer(text, return_tensors='pt')

with torch.no_grad():
    outputs = model(**inputs, output_attentions=True)

# Attention weights を取得
attentions = outputs.attentions
print(f"レイヤー数: {len(attentions)}")
print(f"各レイヤーの形状: {attentions[0].shape}")
```

出力：

```
レイヤー数: 12
各レイヤーの形状: torch.Size([1, 12, 7, 7])  # (バッチ, ヘッド数, シーケンス, シーケンス)
```

Attention の形状 `(1, 12, 7, 7)` の意味：

- **1**: バッチサイズ
- **12**: Attention ヘッドの数（異なる観点で注目パターンを学習）
- **7 × 7**: 7 トークン同士の注目度合い（行 = Query、列 = Key）

## ヒートマップで可視化

![](/images/transformer-io/attention-layer0-head0.png)

ヒートマップの見方：

- **横軸（Key）**: 注目される側のトークン
- **縦軸（Query）**: 注目する側のトークン
- **色の濃さ**: 注目度合い（濃いほど強く注目）

## Causal Attention（因果的注意）

GPT-2 は **自己回帰モデル** なので、各トークンは **自分より前のトークンにしか注目できません**。これを Causal Attention（または Masked Self-Attention）と呼びます。

ヒートマップを見ると、対角線より上（未来のトークン）の値がすべて 0 になっていることが分かります。

```python
attn = attentions[0][0, 0].numpy()  # Layer 0, Head 0
print("Attention 行列（下三角のみ有効）:")
print(np.round(attn, 2))
```

これにより、モデルは「カンニング」せずに、過去の情報のみから次の単語を予測することができます。

## 複数ヘッドの Attention パターン

GPT-2 には 12 個の Attention ヘッドがあり、それぞれ異なるパターンで注目します：

![](/images/transformer-io/attention-multi-heads.png)

各ヘッドの役割（例）：

- **直前のトークンに注目**: 局所的な文法関係を捉える
- **文頭のトークンに注目**: 文全体の主題を捉える
- **特定の品詞に注目**: 動詞と主語の関係を捉える

複数のヘッドを持つことで、多様な観点から文脈を理解できます。

# 入力長とバッチ処理

## 入力長による出力形状の変化

Transformer の出力は、入力の長さに応じて変わります：

```python
short_text = "Hi"
long_text = "The quick brown fox jumps over the lazy dog"

inputs_short = tokenizer(short_text, return_tensors='pt')
inputs_long = tokenizer(long_text, return_tensors='pt')

print(f"短いテキスト: トークン数 = {inputs_short['input_ids'].shape[1]}")
print(f"長いテキスト: トークン数 = {inputs_long['input_ids'].shape[1]}")

with torch.no_grad():
    outputs_short = model_lm(**inputs_short)
    outputs_long = model_lm(**inputs_long)

print(f"短い出力: {outputs_short.logits.shape}")  # (1, 1, 50257)
print(f"長い出力: {outputs_long.logits.shape}")   # (1, 10, 50257)
```

## 最大シーケンス長

GPT-2 は最大 1024 トークンまでしか処理できません。それを超える場合は切り詰め（truncation）が必要です：

```python
very_long_text = "Hello " * 600  # 非常に長いテキスト

inputs = tokenizer(
    very_long_text,
    return_tensors='pt',
    truncation=True,  # 切り詰めを有効化
    max_length=1024   # 最大長を指定
)
print(f"切り詰め後: {inputs['input_ids'].shape[1]} トークン")
```

## バッチ処理とパディング

複数のテキストを同時に処理する場合、長さを揃える必要があります：

```python
# GPT-2 にはデフォルトで pad_token がないので設定
tokenizer.pad_token = tokenizer.eos_token

texts = [
    "Hello",
    "Hello, how are you?",
    "The quick brown fox jumps over the lazy dog"
]

# バッチ処理（パディングあり）
batch_inputs = tokenizer(texts, return_tensors='pt', padding=True)

print(f"バッチ形状: {batch_inputs['input_ids'].shape}")  # (3, 10)
print(f"Attention Mask:\n{batch_inputs['attention_mask']}")
```

出力：

```
バッチ形状: torch.Size([3, 10])  # 3 つのテキスト、最大 10 トークン
Attention Mask:
tensor([[1, 0, 0, 0, 0, 0, 0, 0, 0, 0],  # 実際は 1 トークン、残りパディング
        [1, 1, 1, 1, 1, 1, 0, 0, 0, 0],  # 6 トークン
        [1, 1, 1, 1, 1, 1, 1, 1, 1, 1]]) # 10 トークン（最長）
```

**attention_mask** の役割：

- **1**: 実際のトークン（Attention 計算に含める）
- **0**: パディングトークン（無視する）

# まとめ: データの流れ全体像

テキストから次の単語予測までのデータの流れを整理しましょう：

```
テキスト
   ↓ Tokenizer
Token ID: (Batch, Sequence)
   ↓ Embedding
埋め込み: (Batch, Sequence, Hidden=768)
   ↓ Transformer × 12 層
Hidden State: (Batch, Sequence, Hidden=768)
   ↓ LM Head
Logits: (Batch, Sequence, Vocab=50257)
   ↓ Softmax
確率分布 → 次の単語を予測
```

## 主要なデータ構造まとめ

| データ              | 形状                            | 説明                       |
| ------------------- | ------------------------------- | -------------------------- |
| input_ids           | (Batch, Sequence)               | トークンの ID 列           |
| attention_mask      | (Batch, Sequence)               | パディングのマスク         |
| token_embeddings    | (Batch, Sequence, 768)          | トークンの意味ベクトル     |
| position_embeddings | (Batch, Sequence, 768)          | 位置のベクトル             |
| last_hidden_state   | (Batch, Sequence, 768)          | 文脈を考慮した表現         |
| logits              | (Batch, Sequence, 50257)        | 次の単語の確率（未正規化） |
| attentions          | (Batch, 12, Sequence, Sequence) | Attention の重み           |

# 参考リンク

- [Hugging Face Transformers ドキュメント](https://huggingface.co/docs/transformers)
- [Attention Is All You Need（原論文）](https://arxiv.org/abs/1706.03762)
- [GPT-2 論文（Language Models are Unsupervised Multitask Learners）](https://openai.com/research/better-language-models)
- [The Illustrated Transformer（図解 Transformer）](https://jalammar.github.io/illustrated-transformer/)
