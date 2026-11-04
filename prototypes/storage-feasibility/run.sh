#!/usr/bin/env bash
# Measures SeaweedFS allocation and access control with generated data.
# Usage: ./run.sh [size_mib]   |   ./run.sh clean
set -uo pipefail
cd "$(dirname "$0")"
EP=http://127.0.0.1:18333
DC="docker compose -p pocketcloud-proto"

if [ "${1:-}" = clean ]; then $DC down -v; rm -rf .local; exit 0; fi
SIZE_MIB=${1:-1024}

pass=0; fail=0
check() { if [ "$2" = "$3" ]; then echo "PASS  $1 ($2)"; pass=$((pass+1)); else echo "FAIL  $1 (got $2, wanted $3)"; fail=$((fail+1)); fi; }
alloc() { echo "ALLOC $1: $($DC exec -T seaweedfs du -sk /data | cut -f1) KiB"; }
s3() { curl -sS -K .local/curlrc --aws-sigv4 "aws:amz:us-east-1:s3" -H "x-amz-content-sha256: UNSIGNED-PAYLOAD" "$@"; }
wait_ready() { for _ in $(seq 1 60); do [ "$(curl -s -o /dev/null -w '%{http_code}' $EP/)" != 000 ] && { sleep 3; return 0; }; sleep 2; done; echo "S3 did not start"; exit 1; }

mkdir -p .local; chmod 700 .local
AK=$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n'); SK=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')
printf '{"identities":[{"name":"proto","credentials":[{"accessKey":"%s","secretKey":"%s"}],"actions":["Admin","Read","List","Write"]}]}\n' "$AK" "$SK" > .local/s3.json
printf 'user = "%s:%s"\n' "$AK" "$SK" > .local/curlrc; chmod 600 .local/curlrc; chmod 644 .local/s3.json

echo "== start"
if ! docker version >/dev/null 2>&1; then echo "Cannot reach Docker. Is Docker Desktop running? If it says permission denied, open a new Ubuntu window."; exit 1; fi
$DC up -d --quiet-pull 2>&1 | tail -1 || { echo "Could not start SeaweedFS"; exit 1; }
wait_ready; alloc "after startup"
$DC exec -T seaweedfs sh -c 'id -u' | sed 's/^/INFO  container uid: /'

echo "== access control"
check "credentialed bucket create" "$(s3 -o /dev/null -w '%{http_code}' -X PUT $EP/proto)" 200
check "anonymous list refused" "$(curl -s -o /dev/null -w '%{http_code}' $EP/proto)" 403
check "anonymous write refused" "$(curl -s -o /dev/null -w '%{http_code}' -X PUT --data x $EP/proto/anon)" 403
check "wrong secret refused" "$(curl -s -o /dev/null -w '%{http_code}' --user "$AK:wrong" --aws-sigv4 'aws:amz:us-east-1:s3' -H 'x-amz-content-sha256: UNSIGNED-PAYLOAD' $EP/proto)" 403
for p in 9333 8888 8080; do check "port $p not published" "$(curl -s -m 3 -o /dev/null -w '%{http_code}' http://127.0.0.1:$p/)" 000; done

echo "== upload ${SIZE_MIB} MiB of random data"
head -c $((SIZE_MIB*1048576)) /dev/urandom > .local/test.bin
SRC=$(sha256sum .local/test.bin | cut -d' ' -f1)
s=$(date +%s.%N)
check "upload" "$(s3 -o /dev/null -w '%{http_code}' -T .local/test.bin $EP/proto/test.bin)" 200
echo "INFO  upload seconds: $(echo "$(date +%s.%N) $s" | awk '{printf "%.1f",$1-$2}')"
alloc "after upload"
$DC exec -T seaweedfs sh -c 'ls -l /data | awk "{print \$5, \$9}" | sort -rn | head -6' | sed 's/^/FILE  /'
check "download matches source" "$(s3 $EP/proto/test.bin | sha256sum | cut -d' ' -f1)" "$SRC"

echo "== restart"
$DC restart seaweedfs >/dev/null 2>&1; wait_ready; alloc "after restart"
check "matches after restart" "$(s3 $EP/proto/test.bin | sha256sum | cut -d' ' -f1)" "$SRC"
$DC down >/dev/null 2>&1; $DC up -d --quiet-pull >/dev/null 2>&1; wait_ready
check "matches after container recreated" "$(s3 $EP/proto/test.bin | sha256sum | cut -d' ' -f1)" "$SRC"

echo "== delete"
check "delete" "$(s3 -o /dev/null -w '%{http_code}' -X DELETE $EP/proto/test.bin)" 204
sleep 5; alloc "right after delete"
$DC exec -T seaweedfs sh -c 'printf "lock\nvolume.vacuum -garbageThreshold 0.001\nunlock\n" | weed shell -master=127.0.0.1:9333' >/dev/null 2>&1
sleep 5; alloc "after manual vacuum"
rm -f .local/test.bin
echo "== result: $pass passed, $fail failed"
