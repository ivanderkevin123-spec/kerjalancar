package auth_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/auth"
)

// Real Argon2id, light cost: the production cost (DefaultParams) takes seconds under -race.
var light = auth.Params{MemoryKiB: 8192, Time: 1, Threads: 1}

func TestDefaultParamsAreProductionGrade(t *testing.T) {
	assert.Equal(t, auth.Params{MemoryKiB: 64 * 1024, Time: 3, Threads: 2}, auth.DefaultParams)
}

func TestHashPasswordProducesPHCArgon2id(t *testing.T) {
	hash, err := auth.HashPasswordWith(light, "kata-sandi-rahasia")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(hash, "$argon2id$v=19$m=8192,t=1,p=1$"), hash)
	assert.NotContains(t, hash, "kata-sandi-rahasia")

	again, err := auth.HashPasswordWith(light, "kata-sandi-rahasia")
	require.NoError(t, err)
	assert.NotEqual(t, hash, again, "a fresh random salt every time")
}

func TestVerifyPassword(t *testing.T) {
	hash, err := auth.HashPasswordWith(light, "benar123")
	require.NoError(t, err)

	ok, err := auth.VerifyPassword("benar123", hash)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = auth.VerifyPassword("salah123", hash)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestVerifyPasswordUsesParametersFromTheHash(t *testing.T) {
	// A hash made with lighter parameters must still verify: parameters live in the string.
	salt := []byte("saltsaltsaltsalt")
	key := argon2.IDKey([]byte("pw"), salt, 1, 8192, 1, 32)
	b64 := base64.RawStdEncoding.EncodeToString
	light := "$argon2id$v=19$m=8192,t=1,p=1$" + b64(salt) + "$" + b64(key)
	ok, err := auth.VerifyPassword("pw", light)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	for _, bad := range []string{"", "plain", "$argon2i$v=19$m=1,t=1,p=1$abc$def", "$argon2id$v=19$m=x,t=1,p=1$abc$def"} {
		ok, err := auth.VerifyPassword("pw", bad)
		assert.Error(t, err, bad)
		assert.False(t, ok)
	}
}

func TestHashPasswordRejectsEmptyOrHugeInput(t *testing.T) {
	_, err := auth.HashPasswordWith(light, "")
	assert.Error(t, err)
	_, err = auth.HashPasswordWith(light, strings.Repeat("a", 1025))
	assert.Error(t, err)
	_, err = auth.HashPasswordWith(auth.Params{}, "ok")
	assert.Error(t, err, "zero parameters are a programming error")
}
