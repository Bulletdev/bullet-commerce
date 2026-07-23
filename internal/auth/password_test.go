package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArgon2idHasher_RoundTrip(t *testing.T) {
	h := NewArgon2idHasher()

	hash, err := h.HashPassword("correcthorsebatterystaple")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(hash, "$argon2id$"))

	assert.NoError(t, h.CheckPassword(hash, "correcthorsebatterystaple"))
}

func TestArgon2idHasher_WrongPassword(t *testing.T) {
	h := NewArgon2idHasher()

	hash, err := h.HashPassword("original")
	require.NoError(t, err)

	assert.ErrorIs(t, h.CheckPassword(hash, "wrong"), ErrPasswordMismatch)
}

func TestArgon2idHasher_UniqueHashes(t *testing.T) {
	h := NewArgon2idHasher()

	hash1, err := h.HashPassword("same")
	require.NoError(t, err)
	hash2, err := h.HashPassword("same")
	require.NoError(t, err)

	// Different salts must produce different hashes
	assert.NotEqual(t, hash1, hash2)
	assert.NoError(t, h.CheckPassword(hash1, "same"))
	assert.NoError(t, h.CheckPassword(hash2, "same"))
}

func TestArgon2idHasher_InvalidHashFormat(t *testing.T) {
	h := NewArgon2idHasher()

	assert.ErrorIs(t, h.CheckPassword("notahash", "pw"), ErrInvalidHash)
	assert.ErrorIs(t, h.CheckPassword("$bcrypt$something", "pw"), ErrInvalidHash)
	assert.ErrorIs(t, h.CheckPassword("", "pw"), ErrInvalidHash)
}

func TestArgon2idHasher_No72ByteTruncation(t *testing.T) {
	// bcrypt silently truncates at 72 bytes; Argon2id must NOT.
	h := NewArgon2idHasher()
	long := strings.Repeat("a", 73)
	short := strings.Repeat("a", 72)

	hashLong, err := h.HashPassword(long)
	require.NoError(t, err)

	assert.NoError(t, h.CheckPassword(hashLong, long))
	assert.ErrorIs(t, h.CheckPassword(hashLong, short), ErrPasswordMismatch)
}
