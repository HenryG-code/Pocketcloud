// Command batch asks: does a representative batch of files, far larger than
// memory, survive storage, backup, destroying the original and restore?
// Set SIZE_MIB for the batch size. Data is generated and streamed, never kept.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	endpoint    = "http://127.0.0.1:18333"
	bucket      = "proto"
	original    = "pocketcloud-proto"
	restored    = "pocketcloud-proto-restore"
	archivePath = ".local/backup.tar"
	partSize    = 64 << 20
	midSize     = 32 << 20
	smallFiles  = 2000
)

var (
	ctx    = context.Background()
	client *s3.Client
	passed int
	failed int
)

func main() {
	sizeMiB, err := strconv.Atoi(os.Getenv("SIZE_MIB"))
	if err != nil || sizeMiB < 256 {
		log.Fatal("set SIZE_MIB to 256 or more")
	}
	client = newClient()
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		log.Fatalf("create bucket: %v", err)
	}

	total := int64(sizeMiB) << 20
	big := total / 2
	mids := int((total - big) / midSize)
	fmt.Printf("== store: one %d MiB file, %d files of 32 MiB, %d small files, and edge cases\n", big>>20, mids, smallFiles)
	alloc(original, "before any files")
	manifest := map[string]string{}
	started := time.Now()

	manifest["big/large.bin"] = putMultipart("big/large.bin", big)
	alloc(original, "after the large file")
	for i := 0; i < mids; i++ {
		put(manifest, fmt.Sprintf("mid/%04d.bin", i), randomBytes(midSize))
	}
	put(manifest, "empty.txt", []byte{})
	put(manifest, "nested/a/b/c/deep.txt", randomBytes(1024))
	put(manifest, "unicode/日本語 ファイル 🎉.txt", randomBytes(1024))
	put(manifest, "symbols/spaces + plus & amp (1).txt", randomBytes(1024))
	var smallBytes int64
	for i := 0; i < smallFiles; i++ {
		n := 4096 + i%60*1024 // 4 KiB to about 63 KiB.
		smallBytes += int64(n)
		put(manifest, fmt.Sprintf("small/%04d.bin", i), randomBytes(n))
	}
	seconds := time.Since(started).Seconds()
	stored := big + int64(mids)*midSize + smallBytes + 3072
	fmt.Printf("INFO  stored %d files, %d KiB of data, in %.0f seconds\n", len(manifest), stored/1024, seconds)
	allocated := alloc(original, "after all files")
	fmt.Printf("INFO  overhead: %d KiB (%.2f%%)\n", allocated-stored/1024, 100*float64(allocated-stored/1024)/float64(stored/1024))
	verifyAll(manifest, "before backup")

	fmt.Println("== back up with storage stopped")
	compose(original, "stop")
	started = time.Now()
	tarVolume(original+"_data", archivePath)
	info, err := os.Stat(archivePath)
	if err != nil {
		log.Fatal(err)
	}
	archiveHash := fileHash(archivePath)
	fmt.Printf("INFO  archive: %d KiB, %.0f seconds including its hash\n", info.Size()/1024, time.Since(started).Seconds())

	fmt.Println("== destroy the original")
	compose(original, "down", "-v")
	check("original volume gone", exec.Command("docker", "volume", "inspect", original+"_data").Run() == nil, false)

	fmt.Println("== restore into a separate deployment, and verify every file")
	check("archive unchanged", fileHash(archivePath), archiveHash)
	started = time.Now()
	compose(restored, "create", "--quiet-pull")
	untarVolume(restored+"_data", archivePath)
	compose(restored, "up", "-d", "--quiet-pull")
	waitReady()
	fmt.Printf("INFO  restore and start: %.0f seconds\n", time.Since(started).Seconds())
	alloc(restored, "after restore")
	check("file names match exactly", diffNames(manifest), "none")
	started = time.Now()
	verifyAll(manifest, "after restore")
	fmt.Printf("INFO  verified every byte in %.0f seconds\n", time.Since(started).Seconds())
	if err := os.Remove(archivePath); err != nil {
		log.Fatal(err)
	}

	fmt.Println("== delete everything, and see what comes back")
	for key := range manifest {
		if _, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}); err != nil {
			log.Fatalf("delete: %v", err)
		}
	}
	alloc(restored, "straight after deleting")
	time.Sleep(30 * time.Second)
	vacuum(restored)
	alloc(restored, "after vacuum")

	fmt.Printf("== result: %d passed, %d failed\n", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func check(name string, got, want any) {
	g, w := fmt.Sprint(got), fmt.Sprint(want)
	if g == w {
		fmt.Printf("PASS  %s (%s)\n", name, g[:min(len(g), 16)])
		passed++
		return
	}
	fmt.Printf("FAIL  %s (got %s, wanted %s)\n", name, g[:min(len(g), 200)], w[:min(len(w), 16)])
	failed++
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return b
}

func put(manifest map[string]string, key string, body []byte) {
	if _, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(body)}); err != nil {
		log.Fatalf("put %q: %v", key, err)
	}
	sum := sha256.Sum256(body)
	manifest[key] = hex.EncodeToString(sum[:])
}

// putMultipart streams a file larger than memory, one part at a time, hashing as it goes.
func putMultipart(key string, size int64) string {
	up, err := client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		log.Fatalf("start: %v", err)
	}
	h := sha256.New()
	var done []types.CompletedPart
	buffer := make([]byte, partSize)
	for n, left := int32(1), size; left > 0; n++ {
		chunk := buffer[:min(left, partSize)]
		if _, err := rand.Read(chunk); err != nil {
			log.Fatal(err)
		}
		h.Write(chunk)
		part, err := client.UploadPart(ctx, &s3.UploadPartInput{
			Bucket: aws.String(bucket), Key: aws.String(key), UploadId: up.UploadId, PartNumber: aws.Int32(n), Body: bytes.NewReader(chunk),
		})
		if err != nil {
			log.Fatalf("part %d: %v", n, err)
		}
		done = append(done, types.CompletedPart{PartNumber: aws.Int32(n), ETag: part.ETag})
		left -= int64(len(chunk))
	}
	if _, err := client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket: aws.String(bucket), Key: aws.String(key), UploadId: up.UploadId, MultipartUpload: &types.CompletedMultipartUpload{Parts: done},
	}); err != nil {
		log.Fatalf("complete: %v", err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func download(key string, h hash.Hash) error {
	obj, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return err
	}
	defer obj.Body.Close()
	_, err = io.Copy(h, obj.Body)
	return err
}

func verifyAll(manifest map[string]string, when string) {
	bad := 0
	for key, want := range manifest {
		h := sha256.New()
		if err := download(key, h); err != nil || hex.EncodeToString(h.Sum(nil)) != want {
			bad++
			fmt.Printf("INFO  mismatch %s: %q\n", when, key)
		}
	}
	check("files with wrong bytes "+when, bad, 0)
}

func diffNames(manifest map[string]string) string {
	found := 0
	var diff []string
	pager := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "list failed: " + err.Error()
		}
		for _, o := range page.Contents {
			found++
			if _, ok := manifest[aws.ToString(o.Key)]; !ok {
				diff = append(diff, "extra "+aws.ToString(o.Key))
			}
		}
	}
	if found != len(manifest) {
		diff = append(diff, fmt.Sprintf("listed %d of %d", found, len(manifest)))
	}
	if len(diff) == 0 {
		return "none"
	}
	return strings.Join(diff, "; ")
}

func alloc(project, label string) int64 {
	out := compose(project, "exec", "-T", "seaweedfs", "du", "-sk", "/data")
	kib, err := strconv.ParseInt(strings.Fields(out)[0], 10, 64)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("ALLOC %s: %d KiB\n", label, kib)
	return kib
}

func vacuum(project string) {
	shell := `printf 'lock\nvolume.vacuum -garbageThreshold 0.001\nunlock\n' | weed shell -master=127.0.0.1:9333`
	compose(project, "exec", "-T", "seaweedfs", "sh", "-c", shell)
	time.Sleep(10 * time.Second)
}

func image() string {
	return strings.TrimSpace(compose(original, "config", "--images"))
}

func tarVolume(volume, path string) {
	file, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	// The archive travels over stdout, so turn logging off or Docker also stores a copy in its logs.
	cmd := exec.Command("docker", "run", "--rm", "--log-driver", "none", "--entrypoint", "tar", "-v", volume+":/data:ro", image(), "-C", "/data", "-cf", "-", ".")
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
	for i := 0; i < 90; i++ {
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
