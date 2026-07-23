package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndValidateToken(t *testing.T) {
	secret := "test-secret-32-chars-minimum-len"
	userID := uuid.New()

	token, err := GenerateToken(userID, secret, time.Hour)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := ValidateToken(token, secret)
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, "bullet-commerce", claims.Issuer)
}

func TestValidateToken_WrongSecret(t *testing.T) {
	userID := uuid.New()

	token, err := GenerateToken(userID, "correct-secret-32-chars-minimum-", time.Hour)
	require.NoError(t, err)

	_, err = ValidateToken(token, "wrong-secret-32-chars-minimum-xx")
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestValidateToken_Expired(t *testing.T) {
	secret := "test-secret-32-chars-minimum-len"
	userID := uuid.New()

	token, err := GenerateToken(userID, secret, -time.Second)
	require.NoError(t, err)

	_, err = ValidateToken(token, secret)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestValidateToken_Malformed(t *testing.T) {
	_, err := ValidateToken("not.a.jwt", "secret")
	assert.ErrorIs(t, err, ErrInvalidToken)
}
