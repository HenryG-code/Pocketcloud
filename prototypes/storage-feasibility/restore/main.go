// Command restore asks: if the original storage is destroyed, can a backup bring
// back every file exactly? It also damages one stored file on purpose, to see
// whether storage notices or silently returns wrong bytes.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	endpoint    = "http://127.0.0.1:18333"
	bucket      = "proto"
	original    = "pocketcloud-proto"
	restored    = "pocketcloud-proto-restore"
	archivePath = ".local/backup.tar"
	bigFile     = "big/random.bin"
	smallFiles  = 1000
	smallSize   = 4 << 10 // 4 KiB.
	bigSize     = 64 << 20
)

var (
	ctx    = context.Background()
	client *s3.Client
	passed int
	failed int
)

func main() {
	client = newClient()
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		log.Fatalf("create bucket: %v", err)
	}

	fmt.Println("== store a realistic mix of files")
	alloc(original, "before any files")
	manifest := map[string]string{} // Object key to source SHA-256: our independent record.
	put(manifest, bigFile, randomBytes(bigSize))
	alloc(original, "after one 64 MiB file")
	put(manifest, "empty.txt", nil)
	put(manifest, "nested/a/b/c/deep.txt", randomBytes(1024))
	put(manifest, "unicode/日本語 ファイル 🎉.txt", randomBytes(1024))
	put(manifest, "symbols/spaces + plus & amp (1).txt", randomBytes(1024))
	for i := 0; i < smallFiles; i++ {
		put(manifest, fmt.Sprintf("small/%04d.bin", i), randomBytes(smallSize))
	}
	alloc(original, fmt.Sprintf("after %d small files (%d KiB of data)", smallFiles, smallFiles*smallSize/1024))
	fmt.Println("INFO  files in manifest:", len(manifest))
	verifyAll(manifest, "before backup")

	fmt.Println("== back up with storage stopped, so nothing changes during the copy")
	compose(original, "stop")
	backupStart := time.Now()
	tarVolume(original+"_data", archivePath)
	info, err := os.Stat(archivePath)
	if err != nil {
		log.Fatal(err)
	}
	archiveHash := fileHash(archivePath)
	fmt.Printf("INFO  archive: %d KiB in %.1f seconds\n", info.Size()/1024, time.Since(backupStart).Seconds())

	fmt.Println("== destroy the original, proving the backup stands alone")
	compose(original, "down", "-v")
	check("original volume gone", fmt.Sprint(volumeExists(original+"_data")), "false")

	fmt.Println("== restore into a separate, isolated deployment")
	check("archive unchanged", fileHash(archivePath), archiveHash)
	restoreStart := time.Now()
	compose(restored, "create", "--quiet-pull")
	untarVolume(restored+"_data", archivePath)
	compose(restored, "up", "-d", "--quiet-pull")
	waitReady()
	fmt.Printf("INFO  restore and start: %.1f seconds\n", time.Since(restoreStart).Seconds())
	alloc(restored, "after restore")

	fmt.Println("== verify every file against the source manifest")
	check("file names match exactly", diffNames(manifest), "none")
	verifyAll(manifest, "after restore")
	check("new write accepted after restore", putStatus("after-restore.txt"), "ok")
	check("anonymous read still refused", anonymousStatus(), "403")

	fmt.Println("== damage one byte of stored data, then read it back")
	compose(restored, "stop")
	damaged := damageLargestVolume(restored + "_data")
	fmt.Println("INFO  changed one byte in the middle of:", damaged)
	compose(restored, "start")
	waitReady()
	report := map[string]int{"exact match": 0, "wrong bytes returned": 0, "read refused": 0}
	for key, want := range manifest {
		got, err := download(key)
		switch {
		case err != nil:
			report["read refused"]++
		case got == want:
			report["exact match"]++
		default:
			report["wrong bytes returned"]++
			fmt.Println("INFO  wrong bytes returned for:", key)
		}
	}
	fmt.Printf("INFO  reads after damage: %d exact, %d refused, %d wrong bytes\n", report["exact match"], report["read refused"], report["wrong bytes returned"])
	check("damage is never returned as good data", report["wrong bytes returned"], 0)

	fmt.Printf("== result: %d passed, %d failed\n", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func check(name string, got, want any) {
	g, w := fmt.Sprint(got), fmt.Sprint(want)
	if g == w {
		fmt.Printf("PASS  %s (%s)\n", name, shorten(g))
		passed++
		return
	}
	fmt.Printf("FAIL  %s (got %s, wanted %s)\n", name, shorten(g), shorten(w))
	failed++
}

func shorten(s string) string {
	if len(s) > 40 {
		return s[:12] + "..."
	}
	return s
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return b
}

func put(manifest map[string]string, key string, body []byte) {
	if body == nil {
		body = []byte{}
	}
	if _, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(body)}); err != nil {
		log.Fatalf("put %q: %v", key, err)
	}
	sum := sha256.Sum256(body)
	manifest[key] = hex.EncodeToString(sum[:])
}

func putStatus(key string) string {
	if _, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: strings.NewReader("x")}); err != nil {
		return err.Error()
	}
	return "ok"
}

func anonymousStatus() string {
	out, _ := exec.Command("curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", endpoint+"/"+bucket+"/empty.txt").Output()
	return string(out)
}

// download streams an object through SHA-256 without holding it in memory.
func download(key string) (string, error) {
	obj, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return "", err
	}
	defer obj.Body.Close()
	h := sha256.New()
	if _, err := io.Copy(h, obj.Body); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func verifyAll(manifest map[string]string, when string) {
	bad := 0
	for key, want := range manifest {
		if got, err := download(key); err != nil || got != want {
			bad++
			fmt.Printf("INFO  mismatch %s: %q\n", when, key)
		}
	}
	check(fmt.Sprintf("files with wrong bytes %s", when), bad, 0)
}

func diffNames(manifest map[string]string) string {
	stored := map[string]bool{}
	pager := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "list failed: " + err.Error()
		}
		for _, o := range page.Contents {
			stored[aws.ToString(o.Key)] = true
		}
	}
	var diff []string
	for key := range manifest {
		if !stored[key] {
			diff = append(diff, "missing "+key)
		}
	}
	for key := range stored {
		if _, ok := manifest[key]; !ok {
			diff = append(diff, "extra "+key)
		}
	}
	if len(diff) == 0 {
		return "none"
	}
	sort.Strings(diff)
	return strings.Join(diff, "; ")
}

func alloc(project, label string) {
	out := compose(project, "exec", "-T", "seaweedfs", "du", "-sk", "/data")
	fmt.Printf("ALLOC %s: %s KiB\n", label, strings.Fields(out)[0])
}

func image() string {
	return strings.TrimSpace(compose(original, "config", "--images"))
}

// tarVolume streams a volume into a file, so the archive belongs to this user.
func tarVolume(volume, path string) {
	file, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	cmd := exec.Command("docker", "run", "--rm", "--entrypoint", "tar", "-v", volume+":/data:ro", image(), "-C", "/data", "-cf", "-", ".")
	cmd.Stdout = file
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("backup: %v", err)
	}
}

func untarVolume(volume, path string) {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	cmd := exec.Command("docker", "run", "--rm", "-i", "--entrypoint", "tar", "-v", volume+":/data", image(), "-C", "/data", "-xf", "-")
	cmd.Stdin = file
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("restore: %v", err)
	}
}

func damageLargestVolume(volume string) string {
	script := `f=$(ls -S /data/*.dat | head -1); s=$(wc -c < "$f"); printf X | dd of="$f" bs=1 seek=$((s/2)) conv=notrunc 2>/dev/null; basename "$f"`
	out, err := exec.Command("docker", "run", "--rm", "--entrypoint", "sh", "-v", volume+":/data", image(), "-c", script).CombinedOutput()
	if err != nil {
		log.Fatalf("damage: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func volumeExists(name string) bool {
	return exec.Command("docker", "volume", "inspect", name).Run() == nil
}

func fileHash(path string) string {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		log.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func waitReady() {
	for i := 0; i < 60; i++ {
		if _, err := client.ListBuckets(ctx, &s3.ListBucketsInput{}); err == nil {
			time.Sleep(3 * time.Second)
			return
		}
		time.Sleep(2 * time.Second)
	}
	log.Fatal("storage did not start")
}

func compose(project string, args ...string) string {
	cmd := exec.Command("docker", append([]string{"compose", "-p", project}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("docker compose %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func newClient() *s3.Client {
	raw, err := os.ReadFile(".local/s3.json")
	if err != nil {
		log.Fatal(err)
	}
	var conf struct {
		Identities []struct {
			Credentials []struct{ AccessKey, SecretKey string }
		}
	}
	if err := json.Unmarshal(raw, &conf); err != nil {
		log.Fatal(err)
	}
	c := conf.Identities[0].Credentials[0]
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(c.AccessKey, c.SecretKey, "")),
		config.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
	)
	if err != nil {
		log.Fatal(err)
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
		o.DisableLogOutputChecksumValidationSkipped = true
	})
}
