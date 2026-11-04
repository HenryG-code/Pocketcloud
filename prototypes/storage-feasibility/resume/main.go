// Command resume asks: if a multipart upload is cut off, or storage crashes,
// can it continue from the parts already saved and still end with exact bytes?
// It also measures how much disk unfinished and abandoned uploads hold on to.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	endpoint = "http://127.0.0.1:18333"
	bucket   = "proto"
	partSize = 8 << 20 // 8 MiB per part.
	numParts = 8       // 64 MiB in total.
)

var (
	ctx    = context.Background()
	client *s3.Client
	signer *s3.PresignClient
	passed int
	failed int
)

func main() {
	client = newClient()
	signer = s3.NewPresignClient(client, s3.WithPresignExpires(10*time.Minute))
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		log.Fatalf("create bucket: %v", err)
	}

	data := make([]byte, partSize*numParts)
	if _, err := rand.Read(data); err != nil {
		log.Fatal(err)
	}
	source := sha256.Sum256(data)
	sourceHash := hex.EncodeToString(source[:])
	part := func(n int) []byte { return data[(n-1)*partSize : n*partSize] }

	fmt.Println("== start, and save parts 1 to 3")
	key := "resumed"
	id := startUpload(key)
	alloc("before any parts")
	for n := 1; n <= 3; n++ {
		check(fmt.Sprintf("part %d saved", n), sendPart(key, id, n, part(n)), 200)
	}
	alloc("after 3 parts (24 MiB)")

	fmt.Println("== cut the connection halfway through part 4")
	cutPart(key, id, 4, part(4), partSize/2)
	check("parts recorded after the cut", fmt.Sprint(listNumbers(key, id)), "[1 2 3]")
	alloc("after the cut part (4 MiB sent)")

	fmt.Println("== kill storage with no warning, then restart it")
	crashStorage()
	check("parts recorded after the crash", fmt.Sprint(listNumbers(key, id)), "[1 2 3]")
	alloc("after the crash")

	fmt.Println("== retry part 2, which was already saved")
	check("retried part accepted", sendPart(key, id, 2, part(2)), 200)
	check("known quirk: a retried part is listed twice", fmt.Sprint(listNumbers(key, id)), "[1 2 2 3]")
	alloc("after retrying part 2 (8 MiB resent)")

	fmt.Println("== resume: send only the missing parts")
	missing := missingParts(listNumbers(key, id))
	fmt.Println("INFO  missing parts:", missing)
	check("resume asks for only 4 to 8", fmt.Sprint(missing), "[4 5 6 7 8]")
	for _, n := range missing {
		sendPart(key, id, n, part(n))
	}
	alloc("after all parts (64 MiB)")

	fmt.Println("== finish, and compare bytes with the source")
	check("complete accepted", completeUpload(key, id, nil), "ok")
	alloc("after complete")
	check("stored size", fmt.Sprint(storedSize(key)), fmt.Sprint(partSize*numParts))
	check("SHA-256 matches source", downloadHash(key), sourceHash)
	fmt.Println("INFO  repeat complete on the same upload:", completeUpload(key, id, nil))
	check("SHA-256 matches after repeat", downloadHash(key), sourceHash)
	crashStorage()
	check("SHA-256 matches after another crash", downloadHash(key), sourceHash)

	fmt.Println("== a wrong part tag must not complete")
	bad := "bad-tag"
	badID := startUpload(bad)
	sendPart(bad, badID, 1, part(1))
	result := completeUpload(bad, badID, aws.String("\"00000000000000000000000000000000\""))
	check("complete with a wrong tag refused", fmt.Sprint(result != "ok"), "true")
	check("no file published", fmt.Sprint(storedSize(bad)), "-1")
	abort(bad, badID)

	fmt.Println("== retry a part with different bytes: which bytes win?")
	first, second := part(1), part(2)
	secondSum := sha256.Sum256(second)
	secondHash := hex.EncodeToString(secondSum[:])

	chosen := "retry-chosen-tag"
	chosenID := startUpload(chosen)
	sendPart(chosen, chosenID, 1, first)
	_, secondTag := sendPartTag(chosen, chosenID, 1, second)
	fmt.Println("INFO  parts listed after the retry:", listNumbers(chosen, chosenID))
	check("complete naming the retry's tag", completeUpload(chosen, chosenID, aws.String(secondTag)), "ok")
	check("retry's bytes stored", downloadHash(chosen), secondHash)

	listed := "retry-listed-tags"
	listedID := startUpload(listed)
	sendPart(listed, listedID, 1, first)
	sendPart(listed, listedID, 1, second)
	fmt.Println("INFO  complete using the listing as returned:", completeUpload(listed, listedID, nil))
	fmt.Println("INFO  stored size:", storedSize(listed), "of", partSize, "expected")
	fmt.Println("INFO  stored bytes are the retry's:", downloadHash(listed) == secondHash)
	removeObject(chosen)
	removeObject(listed)

	fmt.Println("== abandon an upload: what does it cost, and how is it reclaimed?")
	removeObject(key)
	time.Sleep(10 * time.Second)
	vacuum()
	alloc("baseline with nothing stored")
	gone := "abandoned"
	goneID := startUpload(gone)
	for n := 1; n <= 3; n++ {
		sendPart(gone, goneID, n, part(n))
	}
	alloc("3 abandoned parts (24 MiB)")
	vacuum()
	alloc("after vacuum, not aborted")
	abort(gone, goneID)
	alloc("right after abort")
	vacuum()
	alloc("vacuum straight after abort")
	for _, wait := range []int{15, 45, 120} {
		time.Sleep(time.Duration(wait) * time.Second)
		vacuum()
		alloc(fmt.Sprintf("vacuum after waiting a further %d seconds", wait))
	}

	fmt.Printf("== result: %d passed, %d failed\n", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func check(name, got, want any) {
	if fmt.Sprint(got) == fmt.Sprint(want) {
		fmt.Printf("PASS  %s (%s)\n", name, shorten(fmt.Sprint(got)))
		passed++
		return
	}
	fmt.Printf("FAIL  %s (got %s, wanted %s)\n", name, shorten(fmt.Sprint(got)), shorten(fmt.Sprint(want)))
	failed++
}

func shorten(s string) string {
	if len(s) > 40 {
		return s[:12] + "..."
	}
	return s
}

func startUpload(key string) string {
	up, err := client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		log.Fatalf("start upload: %v", err)
	}
	return aws.ToString(up.UploadId)
}

// partURL signs one part with its size, as the planned API would.
func partURL(key, id string, n, size int) string {
	req, err := signer.PresignUploadPart(ctx, &s3.UploadPartInput{
		Bucket: aws.String(bucket), Key: aws.String(key), UploadId: aws.String(id),
		PartNumber: aws.Int32(int32(n)), ContentLength: aws.Int64(int64(size)),
	})
	if err != nil {
		log.Fatalf("sign part: %v", err)
	}
	return req.URL
}

func sendPart(key, id string, n int, body []byte) int {
	status, _ := sendPartTag(key, id, n, body)
	return status
}

// sendPartTag also returns the ETag that storage assigned to these bytes.
func sendPartTag(key, id string, n int, body []byte) (int, string) {
	req, err := http.NewRequest(http.MethodPut, partURL(key, id, n, len(body)), bytes.NewReader(body))
	if err != nil {
		log.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("INFO  part error:", err)
		return 0, ""
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, resp.Header.Get("ETag")
}

// cutPart promises a full part, sends only the first bytes, then drops the
// connection, like a lost network or a closed browser tab.
func cutPart(key, id string, n int, body []byte, sendBytes int) {
	pr, pw := io.Pipe()
	req, err := http.NewRequest(http.MethodPut, partURL(key, id, n, len(body)), pr)
	if err != nil {
		log.Fatal(err)
	}
	req.ContentLength = int64(len(body))
	done := make(chan error, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
		done <- err
	}()
	_, _ = pw.Write(body[:sendBytes])
	time.Sleep(500 * time.Millisecond)
	_ = pw.CloseWithError(errors.New("simulated connection loss"))
	<-done
}

func listNumbers(key, id string) []int {
	parts := listParts(key, id)
	numbers := make([]int, 0, len(parts))
	for _, p := range parts {
		numbers = append(numbers, int(aws.ToInt32(p.PartNumber)))
	}
	return numbers
}

func listParts(key, id string) []types.Part {
	out, err := client.ListParts(ctx, &s3.ListPartsInput{Bucket: aws.String(bucket), Key: aws.String(key), UploadId: aws.String(id)})
	if err != nil {
		log.Fatalf("list parts: %v", err)
	}
	return out.Parts
}

func missingParts(have []int) []int {
	seen := map[int]bool{}
	for _, n := range have {
		seen[n] = true
	}
	var missing []int
	for n := 1; n <= numParts; n++ {
		if !seen[n] {
			missing = append(missing, n)
		}
	}
	return missing
}

// completeUpload assembles the parts. A non-nil tag replaces every part's tag.
func completeUpload(key, id string, tag *string) string {
	var done []types.CompletedPart
	for _, p := range listPartsOrNil(key, id) {
		etag := p.ETag
		if tag != nil {
			etag = tag
		}
		done = append(done, types.CompletedPart{PartNumber: p.PartNumber, ETag: etag})
	}
	_, err := client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket: aws.String(bucket), Key: aws.String(key), UploadId: aws.String(id),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: done},
	})
	if err != nil {
		return "refused: " + firstLine(err.Error())
	}
	return "ok"
}

func listPartsOrNil(key, id string) []types.Part {
	out, err := client.ListParts(ctx, &s3.ListPartsInput{Bucket: aws.String(bucket), Key: aws.String(key), UploadId: aws.String(id)})
	if err != nil {
		return nil
	}
	return out.Parts
}

func abort(key, id string) {
	if _, err := client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{Bucket: aws.String(bucket), Key: aws.String(key), UploadId: aws.String(id)}); err != nil {
		fmt.Println("INFO  abort error:", firstLine(err.Error()))
	}
}

func removeObject(key string) {
	if _, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}); err != nil {
		log.Fatalf("delete: %v", err)
	}
}

func storedSize(key string) int64 {
	head, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return -1
	}
	return aws.ToInt64(head.ContentLength)
}

// downloadHash streams the object through SHA-256 without holding it in memory.
func downloadHash(key string) string {
	obj, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return "download failed: " + firstLine(err.Error())
	}
	defer obj.Body.Close()
	h := sha256.New()
	if _, err := io.Copy(h, obj.Body); err != nil {
		return "read failed: " + firstLine(err.Error())
	}
	return hex.EncodeToString(h.Sum(nil))
}

// crashStorage sends SIGKILL, so the process gets no chance to flush or tidy up.
func crashStorage() {
	compose("kill", "seaweedfs")
	compose("up", "-d", "--quiet-pull")
	for i := 0; i < 60; i++ {
		if _, err := client.ListBuckets(ctx, &s3.ListBucketsInput{}); err == nil {
			time.Sleep(3 * time.Second)
			return
		}
		time.Sleep(2 * time.Second)
	}
	log.Fatal("storage did not come back after the crash")
}

func vacuum() {
	shell := `printf 'lock\nvolume.vacuum -garbageThreshold 0.001\nunlock\n' | weed shell -master=127.0.0.1:9333`
	compose("exec", "-T", "seaweedfs", "sh", "-c", shell)
	time.Sleep(5 * time.Second)
}

func alloc(label string) {
	out := compose("exec", "-T", "seaweedfs", "du", "-sk", "/data")
	fmt.Printf("ALLOC %s: %s KiB\n", label, strings.Fields(out)[0])
}

func compose(args ...string) string {
	cmd := exec.Command("docker", append([]string{"compose", "-p", "pocketcloud-proto"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("docker compose %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return s
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
