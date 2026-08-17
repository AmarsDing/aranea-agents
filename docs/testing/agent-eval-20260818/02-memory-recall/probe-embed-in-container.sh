#!/bin/sh
# embed latency from inside aranea-admin container, verify real response
BODY='{"model":"bge-m3","prompt":"核心交换机巡检周期是多久"}'
printf '%s' "$BODY" > /tmp/emb.json
for i in 1 2 3 4 5 6 7 8 9 10; do
  START=$(date +%s%3N)
  wget -q -O /tmp/emb.out --post-file=/tmp/emb.json --header="Content-Type: application/json" http://host.docker.internal:11434/api/embeddings
  RC=$?
  END=$(date +%s%3N)
  SIZE=$(wc -c < /tmp/emb.out)
  echo "embed_$i rc=$RC ms=$((END-START)) bytes=$SIZE"
done
