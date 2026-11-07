#!/usr/bin/env bash
# Starts a disposable SeaweedFS, runs the Go interruption test, then deletes everything it created.
set -uo pipefail
cd "$(dirname "$0")"
DC="docker compose -p pocketcloud-proto"
docker version >/dev/null 2>&1 || { echo "Cannot reach Docker. Is Docker Desktop running? If it says permission denied, open a new Ubuntu window."; exit 1; }
trap '$DC down -v >/dev/null 2>&1; rm -rf .local' EXIT
# If this mounted file was missing, Docker created a root-owned directory in its place. Clear it.
if [ -d .local/s3.json ]; then docker run --rm --entrypoint rm -v "$PWD:/w" "$(sed -n 's/^ *image: //p' compose.yaml)" -rf /w/.local; fi
mkdir -p .local; chmod 700 .local
AK=$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')
SK=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')
printf '{"identities":[{"name":"proto","credentials":[{"accessKey":"%s","secretKey":"%s"}],"actions":["Admin","Read","List","Write"]}]}\n' "$AK" "$SK" > .local/s3.json
chmod 644 .local/s3.json
# Pure Go: no C compiler or headers needed.
CGO_ENABLED=0 go -C resume build -o ../.local/resume . || { echo "Could not build the test"; exit 1; }
$DC up -d --quiet-pull 2>&1 | tail -1 || { echo "Could not start SeaweedFS"; exit 1; }
for _ in $(seq 1 60); do
  [ "$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18333/)" != 000 ] && break
  sleep 2
done
sleep 3
./.local/resume
