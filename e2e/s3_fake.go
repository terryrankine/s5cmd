package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/igungor/gofakes3"
	"github.com/igungor/gofakes3/backend/s3bolt"
	"github.com/igungor/gofakes3/backend/s3mem"
	"gotest.tools/v3/fs"
)

func s3ServerEndpoint(t *testing.T, testdir *fs.Dir, loglvl, backend string, timeSource gofakes3.TimeSource, enableProxy bool, bucketRegion string) string {
	var s3backend gofakes3.Backend
	switch backend {
	case "mem":
		s3backend = s3mem.New()
	case "bolt":
		dbpath := testdir.Join("s3.boltdb")
		// we use boltdb as the s3 backend because listing buckets in in-memory
		// backend is not deterministic.
		var err error
		var opts []s3bolt.Option
		if timeSource != nil {
			opts = append(opts, s3bolt.WithTimeSource(timeSource))
		}

		s3backend, err = s3bolt.NewFile(dbpath, opts...)
		if err != nil {
			t.Fatal(err)
		}
	}

	var opts []gofakes3.Option
	withLogger := gofakes3.WithLogger(
		gofakes3.GlobalLog(
			gofakes3.LogLevel(strings.ToUpper(loglvl)),
		),
	)
	opts = append(opts, withLogger)

	if timeSource != nil {
		opts = append(
			opts,
			gofakes3.WithTimeSource(timeSource),
			// disable time skew with custom time source,
			// requests from past or future would cause 'RequestTimeTooSkewed'
			gofakes3.WithTimeSkewLimit(0),
		)
	}
	faker := gofakes3.New(s3backend, opts...)

	handler := faker.Server()
	if bucketRegion != "" {
		handler = regionRedirect(bucketRegion, handler)
	}
	s3srv := httptest.NewServer(handler)

	t.Cleanup(func() {
		s3srv.Close()
		// no need to remove boltdb file since 'testdir' will be cleaned up
		// after each test.
	})

	if enableProxy {
		parsedURL, err := url.Parse(s3srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		proxyEnabledURL := "http://localhost.:" + parsedURL.Port()
		return proxyEnabledURL
	}
	return s3srv.URL
}

// regionRedirect makes the fake server behave as if every bucket lives in
// the given region. A request signed for another region is answered the way
// Amazon S3 answers a request sent to the wrong regional endpoint: with a
// "301 PermanentRedirect" that carries the bucket's region in the
// x-amz-bucket-region header. The AWS SDK turns that into a BucketRegionError
// and, when it probes for the bucket's region, reads it from the header.
func regionRedirect(bucketRegion string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		region := signedRegion(r)
		if region == "" || region == bucketRegion {
			next.ServeHTTP(w, r)
			return
		}

		bucket := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)[0]
		w.Header().Set("x-amz-bucket-region", bucketRegion)
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMovedPermanently)
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<Error><Code>PermanentRedirect</Code><Message>The bucket you are attempting to access must be addressed using the specified endpoint. Please send all future requests to this endpoint.</Message><Endpoint>%s.s3.%s.amazonaws.com</Endpoint><Bucket>%s</Bucket></Error>`, bucket, bucketRegion, bucket)
	})
}

// signedRegion returns the region in the SigV4 credential scope of the
// request, or "" if the request is not signed.
func signedRegion(r *http.Request) string {
	credential := r.URL.Query().Get("X-Amz-Credential")
	if credential == "" {
		// Authorization: AWS4-HMAC-SHA256 Credential=<id>/<date>/<region>/s3/aws4_request, ...
		_, after, found := strings.Cut(r.Header.Get("Authorization"), "Credential=")
		if !found {
			return ""
		}
		credential, _, _ = strings.Cut(after, ",")
	}

	parts := strings.Split(credential, "/")
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}
