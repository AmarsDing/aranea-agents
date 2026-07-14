#!/usr/bin/env bash
#
# Download evaluation datasets for summary benchmarking.
#
# Supported datasets:
#   - MT-Bench-101: multi-turn dialogue benchmark for baseline vs summary
#   - QMSum: long-meeting benchmark for long_context / summary / summary_ondemand
#   - LongMemEval: multi-session user/assistant dialogue memory benchmark
#

set -euo pipefail

DATASET_SELECTOR="${1:-all}"
DATA_DIR="${2:-.}"

print_usage() {
    cat <<'EOF'
Usage:
  ./download_datasets.sh [all|mtbench101|qmsum|longmemeval|lme] [data_dir]

Examples:
  ./download_datasets.sh
  ./download_datasets.sh qmsum
  ./download_datasets.sh longmemeval
  ./download_datasets.sh mtbench101 ./data
EOF
}

normalize_selector() {
    case "${1,,}" in
        all|"")
            echo "all"
            ;;
        mtbench101|mt-bench-101|mtbench|mt-bench)
            echo "mtbench101"
            ;;
        qmsum)
            echo "qmsum"
            ;;
        longmemeval|lme)
            echo "longmemeval"
            ;;
        *)
            return 1
            ;;
    esac
}

download_mtbench101() {
    local target_dir="$1/mt-bench-101"
    if [ -f "$target_dir/subjective/mtbench101.jsonl" ]; then
        echo "MT-Bench-101 already exists at $target_dir, skipping."
        return
    fi

    echo "=== Downloading MT-Bench-101 ==="
    mkdir -p "$target_dir"
    rm -rf "$target_dir/repo"

    if git clone --depth 1 --filter=blob:none --sparse \
        https://github.com/mtbench101/mt-bench-101.git "$target_dir/repo"; then
        (
            cd "$target_dir/repo"
            git sparse-checkout set data
        )
        if [ -d "$target_dir/repo/data" ]; then
            cp -R "$target_dir/repo/data/." "$target_dir/"
            rm -rf "$target_dir/repo"
            echo "MT-Bench-101 downloaded to $target_dir"
            return
        fi
    fi

    rm -rf "$target_dir/repo"
    echo "Warning: failed to download MT-Bench-101 automatically."
    echo "Manual source: https://github.com/mtbench101/mt-bench-101"
}

download_qmsum() {
    local target_dir="$1/QMSum"
    if [ -d "$target_dir/data/Committee/test" ]; then
        echo "QMSum already exists at $target_dir, skipping."
        return
    fi

    echo "=== Downloading QMSum ==="
    rm -rf "$target_dir"
    if git clone --depth 1 https://github.com/Yale-LILY/QMSum.git "$target_dir"; then
        echo "QMSum downloaded to $target_dir"
        return
    fi

    rm -rf "$target_dir"
    echo "Warning: failed to download QMSum automatically."
    echo "Manual source: https://github.com/Yale-LILY/QMSum"
}

download_longmemeval() {
    local target_dir="$1/longmemeval-cleaned"
    if [ -f "$target_dir/longmemeval_s_cleaned.json" ] && [ "$(wc -c < "$target_dir/longmemeval_s_cleaned.json")" -gt 1000 ]; then
        echo "LongMemEval already exists at $target_dir, skipping."
        return
    fi

    echo "=== Downloading LongMemEval ==="
    mkdir -p "$target_dir"

    local base_url="https://huggingface.co/datasets/xiaowu0162/longmemeval-cleaned/resolve/main"
    local files=("longmemeval_s_cleaned.json" "longmemeval_oracle.json")

    for file in "${files[@]}"; do
        echo "  Downloading $file..."
        if ! wget -q "$base_url/$file" -O "$target_dir/$file"; then
            echo "Warning: failed to download $file"
            rm -f "$target_dir/$file"
        fi
    done

    if [ -f "$target_dir/longmemeval_s_cleaned.json" ]; then
        echo "LongMemEval downloaded to $target_dir"
    else
        echo "Warning: failed to download LongMemEval automatically."
        echo "Manual source: https://huggingface.co/datasets/xiaowu0162/longmemeval-cleaned"
    fi
}

print_dataset_info() {
    local base_dir="$1"

    if [ -f "$base_dir/mt-bench-101/subjective/mtbench101.jsonl" ]; then
        local case_count
        case_count="$(wc -l < "$base_dir/mt-bench-101/subjective/mtbench101.jsonl")"
        echo
        echo "MT-Bench-101:"
        echo "  Location: $base_dir/mt-bench-101"
        echo "  Cases:    $case_count"
    fi

    if [ -d "$base_dir/QMSum/data" ]; then
        local meeting_count
        meeting_count="$(find "$base_dir/QMSum/data" -path '*/test/*.json' | wc -l | tr -d ' ')"
        echo
        echo "QMSum:"
        echo "  Location: $base_dir/QMSum"
        echo "  Test JSON files: $meeting_count"
    fi

    if [ -f "$base_dir/longmemeval-cleaned/longmemeval_s_cleaned.json" ]; then
        echo
        echo "LongMemEval:"
        echo "  Location: $base_dir/longmemeval-cleaned"
        echo "  Files: longmemeval_s_cleaned.json, longmemeval_oracle.json"
    fi
}

SELECTOR="$(normalize_selector "$DATASET_SELECTOR")" || {
    print_usage
    exit 1
}

mkdir -p "$DATA_DIR"
echo "Data directory: $DATA_DIR"

case "$SELECTOR" in
    all)
        download_mtbench101 "$DATA_DIR"
        echo
        download_qmsum "$DATA_DIR"
        echo
        download_longmemeval "$DATA_DIR"
        ;;
    mtbench101)
        download_mtbench101 "$DATA_DIR"
        ;;
    qmsum)
        download_qmsum "$DATA_DIR"
        ;;
    longmemeval)
        download_longmemeval "$DATA_DIR"
        ;;
esac

print_dataset_info "$DATA_DIR"

echo
echo "=== Usage Examples ==="
echo "MT-Bench-101:"
echo "  cd summary/trpc-agent-go-impl"
echo "  go run . -dataset ../data/mt-bench-101 -dataset-format mtbench101 -task CM -num-cases 10"
echo
echo "QMSum:"
echo "  cd summary/trpc-agent-go-impl"
echo "  go run . -dataset ../data/QMSum -dataset-format qmsum -qmsum-domain Committee -num-cases 5 -qmsum-visible-events 20 -qmsum-min-distance-from-end 80"
echo
echo "LongMemEval:"
echo "  cd summary/trpc-agent-go-impl"
echo "  PGVECTOR_DSN=\"postgres://...\" go run . -dataset ../data/longmemeval-cleaned/longmemeval_s_cleaned.json -dataset-format longmemeval -lme-question-types single-session-user -num-cases 5 -events 40 -lme-visible-events 20 -detailed-prompt=true"
