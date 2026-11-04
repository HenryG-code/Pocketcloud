# Storage feasibility prototype

Disposable test of whether SeaweedFS fits PocketCloud's limits. Generated data only, never real files.

```bash
./run.sh          # start, test and measure with 1024 MiB of random data
./run.sh 64       # the same with a smaller file
./run.sh clean    # delete this prototype's container, volume and generated files
./oversize.sh     # check whether signed uploads refuse extra bytes, then delete everything it created
./resume.sh       # cut off an upload, crash storage, resume and verify, then delete everything it created
```

`resume.sh` takes about four minutes because it waits for cleanup to finish.

Credentials are generated into the ignored `.local/` folder on each run. Results go in `docs/benchmarks/`.
