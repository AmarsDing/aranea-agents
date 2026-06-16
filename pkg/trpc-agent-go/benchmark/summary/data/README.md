# Summary Benchmark Data

This directory stores datasets used by the `summary/` benchmark suite.

## Supported Datasets

### MT-Bench-101

Use MT-Bench-101 to evaluate baseline vs session-summary behavior on multi-turn dialogue tasks.

Expected layout:

```text
data/
└── mt-bench-101/
    └── subjective/
        └── mtbench101.jsonl
```

### QMSum

Use QMSum to evaluate long-session detail recovery under:

- `long_context`
- `summary`
- `summary_ondemand`

Expected layout:

```text
data/
└── QMSum/
    └── data/
        ├── ALL/
        ├── Academic/
        ├── Committee/
        └── Product/
```

### LongMemEval

Use LongMemEval to evaluate long-term memory recall over realistic multi-session user/assistant dialogues.
The benchmark currently uses the cleaned single-session-user slice from `longmemeval_s_cleaned.json` for the
main 70-case comparison.

Expected layout:

```text
data/
└── longmemeval-cleaned/
    ├── longmemeval_s_cleaned.json
    ├── longmemeval_m_cleaned.json      # optional / tiny placeholder in the upstream dataset
    └── longmemeval_oracle.json         # optional metadata / oracle file
```

## Download

Download everything:

```bash
./download_datasets.sh
```

Download only MT-Bench-101:

```bash
./download_datasets.sh mtbench101
```

Download only QMSum:

```bash
./download_datasets.sh qmsum
```

Download only LongMemEval:

```bash
./download_datasets.sh longmemeval
# aliases: lme
```

## References

- [MT-Bench-101 Paper](https://arxiv.org/abs/2402.14762)
- [MT-Bench-101 GitHub](https://github.com/mtbench101/mt-bench-101)
- [QMSum Paper](https://arxiv.org/abs/2104.05938)
- [QMSum GitHub](https://github.com/Yale-LILY/QMSum)
- [LongMemEval Paper](https://arxiv.org/abs/2410.10813)
- [LongMemEval Cleaned Dataset](https://huggingface.co/datasets/xiaowu0162/longmemeval-cleaned)
