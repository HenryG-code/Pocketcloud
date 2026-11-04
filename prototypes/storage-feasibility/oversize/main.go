// Command oversize asks one question: can a signed upload stop a client from
// sending more bytes than it declared? The answer decides ADR 0004.
//
// Each case grants permission to upload 1 MiB, then tries to send 2 MiB and
// reports whether the extra bytes were refused and whether anything was stored.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
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
	allowed  = 1 << 20 // 1 MiB: what each case grants.
	sent     = 2 << 20 // 2 MiB: what an oversized client sends.
)

type outcome struct {
	name   string
	status int
	stored int64 // Bytes the store kept, or -1 if nothing was stored.
}

func main() {
	ctx := context.Background()
	client := newClient(ctx)
	presign := s3.NewPresignClient(client, s3.WithPresignExpires(5*time.Minute))
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		log.Fatalf("create bucket: %v", err)
	}

	sizedURL := func(key string) string {
		req, err := presign.PresignPutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket), Key: aws.String(key), ContentLength: aws.Int64(allowed),
		})
		if err != nil {
			log.Fatalf("presign %s: %v", key, err)
		}
		return req.URL
	}

	exact := sizedURL("exact")
	fmt.Println("Signed headers on the sized URL:", signedHeaders(exact))

	var results []outcome
	run := func(name, key string, status int) {
		results = append(results, outcome{name, status, storedSize(ctx, client, key)})
	}
	run("Sized URL, exactly 1 MiB (control)", "exact", put(exact, allowed, false))
	run("Sized URL, 2 MiB with honest length", "over", put(sizedURL("over"), sent, false))
	run("Sized URL, 2 MiB with no declared length", "chunked", put(sizedURL("chunked"), sent, true))

	plain, err := presign.PresignPutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String("plain")})
	if err != nil {
		log.Fatal(err)
	}
	run("Unsized URL, 2 MiB", "plain", put(plain.URL, sent, false))

	results = append(results, multipartCase(ctx, client, presign))
	results = append(results, postCase(ctx, client, presign, "post-exact", allowed, "POST policy, exactly 1 MiB (control)"))
	results = append(results, postCase(ctx, client, presign, "post-over", sent, "POST policy limit 1 MiB, 2 MiB sent"))

	fmt.Printf("\n%-42s %-7s %-12s %s\n", "Case", "Status", "Stored", "Verdict")
	for _, r := range results {
		fmt.Printf("%-42s %-7d %-12s %s\n", r.name, r.status, size(r.stored), verdict(r))
	}
}

func newClient(ctx context.Context) *s3.Client {
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
	})
}

// put sends n random bytes. Hiding the length makes Go use chunked encoding,
// so the server cannot learn the size from a Content-Length header.
func put(target string, n int, hideLength bool) int {
	var body io.Reader = bytes.NewReader(randomBytes(n))
	if hideLength {
		body = io.MultiReader(body)
	}
	req, err := http.NewRequest(http.MethodPut, target, body)
	if err != nil {
		log.Fatal(err)
	}
	return do(req)
}

func multipartCase(ctx context.Context, c *s3.Client, p *s3.PresignClient) outcome {
	key := aws.String("part")
	up, err := c.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: aws.String(bucket), Key: key})
	if err != nil {
		log.Fatal(err)
	}
	req, err := p.PresignUploadPart(ctx, &s3.UploadPartInput{
		Bucket: aws.String(bucket), Key: key, UploadId: up.UploadId, PartNumber: aws.Int32(1), ContentLength: aws.Int64(allowed),
	})
	if err != nil {
		log.Fatal(err)
	}
	status := put(req.URL, sent, false)
	stored := int64(-1)
	parts, err := c.ListParts(ctx, &s3.ListPartsInput{Bucket: aws.String(bucket), Key: key, UploadId: up.UploadId})
	if err == nil && len(parts.Parts) > 0 {
		stored = sumParts(parts.Parts)
	}
	_, _ = c.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{Bucket: aws.String(bucket), Key: key, UploadId: up.UploadId})
	return outcome{"Sized part URL, 2 MiB in one part", status, stored}
}

func postCase(ctx context.Context, c *s3.Client, p *s3.PresignClient, key string, n int, name string) outcome {
	post, err := p.PresignPostObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}, func(o *s3.PresignPostOptions) {
		o.Conditions = []interface{}{[]interface{}{"content-length-range", 1, allowed}}
		o.Expires = 5 * time.Minute
	})
	if err != nil {
		log.Fatalf("presign post: %v", err)
	}
	var form bytes.Buffer
	w := multipart.NewWriter(&form)
	for k, v := range post.Values {
		_ = w.WriteField(k, v)
	}
	part, _ := w.CreateFormFile("file", key)
	_, _ = part.Write(randomBytes(n))
	_ = w.Close()

	target := post.URL
	if !strings.Contains(target, "/"+bucket) {
		target = strings.TrimRight(endpoint, "/") + "/" + bucket
	}
	req, err := http.NewRequest(http.MethodPost, target, &form)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return outcome{name, do(req), storedSize(ctx, c, key)}
}

func do(req *http.Request) int {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("  transport error:", err)
		return 0
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func storedSize(ctx context.Context, c *s3.Client, key string) int64 {
	head, err := c.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return -1
	}
	return aws.ToInt64(head.ContentLength)
}

func sumParts(parts []types.Part) int64 {
	var total int64
	for _, p := range parts {
		total += aws.ToInt64(p.Size)
	}
	return total
}

func signedHeaders(target string) string {
	u, err := url.Parse(target)
	if err != nil {
		return "unknown"
	}
	return u.Query().Get("X-Amz-SignedHeaders")
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return b
}

func size(n int64) string {
	if n < 0 {
		return "nothing"
	}
	return fmt.Sprintf("%d KiB", n/1024)
}

func verdict(r outcome) string {
	switch {
	case r.stored > allowed:
		return "NOT STOPPED: stored more than allowed"
	case r.stored >= 0:
		return "within limit"
	default:
		return "refused"
	}
}
