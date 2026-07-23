// Package storage is the gated port for object storage. It exists so the media vertical can
// offer upload-via-presigned-URL WITHOUT the API ever touching file bytes: the client PUTs
// straight to an S3-compatible bucket (S3 / Cloudflare R2 / MinIO) using a short-lived signed
// URL, then registers the resulting public URL as product media. The feature is behind a gate
// (Config.Active) mirroring the project's other optional providers - when storage is not
// configured the URL-reference flow keeps working and only the upload endpoint is unavailable.
package storage

import "context"

// Provider mints presigned upload URLs. It is deliberately tiny: the only capability the media
// vertical needs is "give the client a way to PUT one object and tell me its public URL".
type Provider interface {
	// PresignPut returns a short-lived signed PUT URL for key (the client uploads to uploadURL
	// with the given contentType) and the publicURL the object will be reachable at afterwards.
	PresignPut(ctx context.Context, key, contentType string) (uploadURL, publicURL string, err error)
}

// Config carries the storage credentials/coordinates. Values are injected by the composition
// root (main.go); this package never reads internal/config so it stays free of the config
// vertical. Secrets live only in these fields at runtime - never hardcoded.
type Config struct {
	Enabled       bool
	Bucket        string
	Endpoint      string
	Region        string
	AccessKey     string
	Secret        string
	PublicBaseURL string
}

// Active reports whether the upload flow can run. WHY both checks: an operator may flip Enabled
// on before finishing setup, and a bucket-less config cannot presign anything - either way the
// handler must fall back to "upload unavailable" instead of dereferencing a half-built client.
func (c Config) Active() bool {
	return c.Enabled && c.Bucket != ""
}
