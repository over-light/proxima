package seenset

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"
)

func TestBasic(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		ss := New[int]()
		require.False(t, ss.Seen(314))
		require.True(t, ss.Seen(314))
		require.False(t, ss.Seen(2718, true))
		require.False(t, ss.Seen(2718))
		require.True(t, ss.Seen(2718))
	})
	t.Run("2", func(t *testing.T) {
		ss := New[[32]byte]()
		h1 := blake2b.Sum256([]byte{1})
		require.False(t, ss.Seen(h1))
		require.True(t, ss.Seen(h1))
		h2 := blake2b.Sum256([]byte{2})
		require.False(t, ss.Seen(h2, true))
		require.False(t, ss.Seen(h2))
		require.True(t, ss.Seen(h2))
	})
}