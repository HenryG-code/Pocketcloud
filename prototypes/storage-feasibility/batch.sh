#!/usr/bin/env bash
# Stores, backs up, restores and verifies a generated batch, then deletes everything it created.
# Usage: ./batch.sh [size_mib]   Default 1024. A 10240 run needs about 30 GiB free and takes a while.
set -uo pipefail
cd "$(dirname "$0")"
export SIZE_MIB=${1:-1024}
# Enough 256 MiB volumes to hold the batch twice over.
export VOLUME_MAX=$(( SIZE_MIB / 128 + 16 ))
DC="docker compose -p pocketcloud-proto"
DR="docker compose -p pocketcloud-proto-restore"
docker version >/dev/null 2>&1 || { echo "Cannot reach Docker. Is Docker Desktop running? If it says permission denied, open a new Ubuntu window."; exit 1; }
need_kib=$(( SIZE_MIB * 1024 * 3 ))
free_kib=$(df -k --output=avail . | tail -1 | tr -d ' ')
[ "$free_kib" -gt "$need_kib" ] || { echo "Not enough free space for this run"; exit 1; }
trap '$DC down -v >/dev/null 2>&1; $DR down -v >/dev/null 2>&1; rm -rf .local' EXIT
# If this mounted file was missing, Docker created a root-owned directory in its place. Clear it.
if [ -d .local/s3.json ]; then docker run --rm --entrypoint rm -v "$PWD:/w" "$(sed -n 's/^ *image: //p' compose.yaml)" -rf /w/.local; fi
mkdir -p .local; chmod 700 .local
AK=$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')
SK=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')
printf '{"identities":[{"name":"proto","credentials":[{"accessKey":"%s","secretKey":"%s"}],"actions":["Admin","Read","List","Write"]}]}\n' "$AK" "$SK" > .local/s3.json
chmod 644 .local/s3.json
# Pure Go: no C compiler or headers needed.
CGO_ENABLED=0 go -C batch build -o ../.local/batch . || { echo "Could not build the test"; exit 1; }
$DC up -d --quiet-pull 2>&1 | tail -1 || { echo "Could not start SeaweedFS"; exit 1; }
for _ in $(seq 1 60); do
  [ "$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18333/)" != 000 ] && break
  sleep 2
done
sleep 3
./.local/batch
