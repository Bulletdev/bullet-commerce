package storage

import (
	"context"
	"strings"
)

// FakeProvider is a network-free Provider for tests and local wiring. It fabricates a
// deterministic upload URL and the same publicURL scheme the real adapter uses, so handler
// tests can exercise the presign path without an S3 endpoint or credentials.
type FakeProvider struct {
	// PublicBaseURL mirrors Config.PublicBaseURL so the produced publicURL matches production shape.
	PublicBaseURL string
	// Err, when set, is returned from PresignPut to exercise the error branch.
	Err error
}

func (f *FakeProvider) PresignPut(_ context.Context, key, _ string) (uploadURL, publicURL string, err error) {
	if f.Err != nil {
		return "", "", f.Err
	}
	base := strings.TrimRight(f.PublicBaseURL, "/")
	key = strings.TrimLeft(key, "/")
	return "https://upload.fake.local/" + key + "?signed=1", base + "/" + key, nil
}
