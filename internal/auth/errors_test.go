package auth

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckPassword_IncompatibleVersion(t *testing.T) {
	h := NewArgon2idHasher().(*argon2idHasher)

	// Craft a PHC string with a future version number
	salt := strings.Repeat("a", 22) // 16 bytes base64-raw = ~22 chars
	hash := strings.Repeat("b", 43)
	encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		999, h.memory, h.iterations, h.parallelism, salt, hash)

	assert.ErrorIs(t, h.CheckPassword(encoded, "pw"), ErrIncompatibleVersion)
}

func TestCheckPassword_BadBase64Salt(t *testing.T) {
	h := NewArgon2idHasher()
	encoded := "$argon2id$v=19$m=65536,t=3,p=4$!!!invalid!!!$abc"
	assert.ErrorIs(t, h.CheckPassword(encoded, "pw"), ErrInvalidHash)
}

func TestCheckPassword_BadBase64Hash(t *testing.T) {
	h := NewArgon2idHasher()
	encoded := "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$!!!invalid!!!"
	assert.ErrorIs(t, h.CheckPassword(encoded, "pw"), ErrInvalidHash)
}

func TestCheckPassword_BadParams(t *testing.T) {
	h := NewArgon2idHasher()
	encoded := "$argon2id$v=19$notparams$c2FsdA$aGFzaA"
	assert.ErrorIs(t, h.CheckPassword(encoded, "pw"), ErrInvalidHash)
}

func TestHashPassword_ProducesVerifiableHash(t *testing.T) {
	h := NewArgon2idHasher()
	hash, err := h.HashPassword("secret")
	require.NoError(t, err)
	require.NoError(t, h.CheckPassword(hash, "secret"))
	assert.ErrorIs(t, h.CheckPassword(hash, "wrong"), ErrPasswordMismatch)
}
