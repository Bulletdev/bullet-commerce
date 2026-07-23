package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Active(t *testing.T) {
	assert.True(t, Config{Enabled: true, Bucket: "b"}.Active())
	assert.False(t, Config{Enabled: false, Bucket: "b"}.Active(), "disabled gate is inactive")
	assert.False(t, Config{Enabled: true, Bucket: ""}.Active(), "no bucket is inactive")
}

// NewS3Provider must refuse to build a client when the gate is off, so the media handler can
// fall back to "upload unavailable" instead of holding a half-configured provider.
func TestNewS3Provider_DisabledReturnsError(t *testing.T) {
	_, err := NewS3Provider(Config{Enabled: false, Bucket: "b"})
	assert.ErrorIs(t, err, ErrStorageDisabled)
}

func TestNewS3Provider_ActiveBuilds(t *testing.T) {
	p, err := NewS3Provider(Config{
		Enabled:       true,
		Bucket:        "media",
		Endpoint:      "https://s3.example.com",
		Region:        "us-east-1",
		AccessKey:     "ak",
		Secret:        "sk",
		PublicBaseURL: "https://cdn.example.com",
	})
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestEndpointHostAndScheme(t *testing.T) {
	host, secure := endpointHostAndScheme("https://s3.example.com")
	assert.Equal(t, "s3.example.com", host)
	assert.True(t, secure)

	host, secure = endpointHostAndScheme("http://localhost:9000")
	assert.Equal(t, "localhost:9000", host)
	assert.False(t, secure)

	host, secure = endpointHostAndScheme("play.min.io")
	assert.Equal(t, "play.min.io", host)
	assert.True(t, secure, "scheme-less endpoint defaults to secure")
}

func TestFakeProvider_PresignPut(t *testing.T) {
	f := &FakeProvider{PublicBaseURL: "https://cdn.example.com/"}
	upload, public, err := f.PresignPut(context.Background(), "products/1/a.jpg", "image/jpeg")
	require.NoError(t, err)
	assert.Contains(t, upload, "products/1/a.jpg")
	assert.Equal(t, "https://cdn.example.com/products/1/a.jpg", public)
}

func TestFakeProvider_PresignPut_Error(t *testing.T) {
	f := &FakeProvider{Err: errors.New("boom")}
	_, _, err := f.PresignPut(context.Background(), "k", "image/jpeg")
	assert.Error(t, err)
}
