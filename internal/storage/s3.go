package storage

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// presignTTL bounds how long a minted upload URL stays valid. Short on purpose: the URL is
// handed to a client to use immediately, so a tight window limits the blast radius of a leak.
const presignTTL = 15 * time.Minute

// ErrStorageDisabled is returned when a presign is attempted on an inactive Config. Callers
// (the media handler) translate it into "upload unavailable" so the URL-reference flow stands.
var ErrStorageDisabled = errors.New("object storage is not configured")

// s3Provider is the S3-compatible adapter (works against AWS S3, Cloudflare R2, MinIO) built on
// minio-go. It holds no secrets beyond the injected Config and the client minio derives from it.
type s3Provider struct {
	client *minio.Client
	cfg    Config
}

// NewS3Provider builds a Provider from cfg. It returns ErrStorageDisabled when the gate is off
// so main.go can decide whether to wire a real provider or leave the upload endpoint dark.
func NewS3Provider(cfg Config) (Provider, error) {
	if !cfg.Active() {
		return nil, ErrStorageDisabled
	}

	host, secure := endpointHostAndScheme(cfg.Endpoint)
	client, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.Secret, ""),
		Secure: secure,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, err
	}
	return &s3Provider{client: client, cfg: cfg}, nil
}

func (p *s3Provider) PresignPut(ctx context.Context, key, contentType string) (uploadURL, publicURL string, err error) {
	// PresignHeader (not PresignedPutObject) so Content-Type is bound into the signature: the
	// client MUST upload with the same content type it declared, closing off a mismatched-type
	// upload against a URL minted for something else.
	headers := http.Header{}
	if contentType != "" {
		headers.Set("Content-Type", contentType)
	}

	u, err := p.client.PresignHeader(ctx, http.MethodPut, p.cfg.Bucket, key, presignTTL, url.Values{}, headers)
	if err != nil {
		return "", "", err
	}

	// publicURL is where the object is served from AFTER upload (a CDN/base URL the operator
	// controls), which is generally NOT the signing endpoint — hence a separate PublicBaseURL.
	publicURL = strings.TrimRight(p.cfg.PublicBaseURL, "/") + "/" + strings.TrimLeft(key, "/")
	return u.String(), publicURL, nil
}

// endpointHostAndScheme splits a configured endpoint into the bare host minio.New wants plus
// whether TLS is used. A scheme-less endpoint defaults to secure (the safe default for a public
// bucket); http:// opts out explicitly for local MinIO.
func endpointHostAndScheme(endpoint string) (host string, secure bool) {
	switch {
	case strings.HasPrefix(endpoint, "https://"):
		return strings.TrimPrefix(endpoint, "https://"), true
	case strings.HasPrefix(endpoint, "http://"):
		return strings.TrimPrefix(endpoint, "http://"), false
	default:
		return endpoint, true
	}
}
