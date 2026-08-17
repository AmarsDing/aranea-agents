#!/bin/sh
# embed latency from inside aranea-admin container: batch timing (busybox date lacks %3N)
BODY='{"model":"bge-m3","prompt":"核心交换机巡检周期是多久"}'
printf '%s' "$BODY" > /tmp/emb.json
# warmup
wget -q -O /tmp/emb.out --post-file=/tmp/emb.json --header="Content-Type: application/json" http://host.docker.internal:11434/api/embeddings
echo "warmup_bytes=$(wc -c < /tmp/emb.out)"
START=$(date +%s)
i=0
while [ $i -lt 30 ]; do
  wget -q -O /tmp/emb.out --post-file=/tmp/emb.json --header="Content-Type: application/json" http://host.docker.internal:11434/api/embeddings || echo "FAIL_$i"
  i=$((i+1))
done
END=$(date +%s)
echo "batch30 total_s=$((END-START)) final_bytes=$(wc -c < /tmp/emb.out)"
