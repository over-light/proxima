package util

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/exp/rand"
)

func TestScaleBytesAsBigInt(t *testing.T) {
	r := RandomFromSeed([]byte("abc"), 3)
	require.True(t, r < 3)
	h := blake2b.Sum256([]byte("abc"))
	r = RandomFromSeed(h[:], 1337)
	require.True(t, r < 1337)

	for i := 0; i < 1000; i++ {
		h = blake2b.Sum256([]byte(fmt.Sprintf("%d%d", i, i)))
		scale := rand.Int31n(500)
		if scale <= 0 {
			scale = 1 - scale
		}
		r = RandomFromSeed(h[:], uint64(scale))
		require.True(t, r < uint64(scale))
	}
}
