package ledger

import (
	"bytes"
	"testing"

	"github.com/lunfardo314/proxima/util"
	"github.com/stretchr/testify/require"
)

func TestUint64FromBytes(t *testing.T) {
	t.Run("1", func(t *testing.T) {
		for i := 0; i <= 8; i++ {
			data := bytes.Repeat([]byte{0}, i)
			require.EqualValues(t, 0, MustUint64FromBytes(data))
		}
	})
	t.Run("1", func(t *testing.T) {
		data := bytes.Repeat([]byte{0}, 9)
		_, err := Uint64FromBytes(data)
		util.RequireErrorWith(t, err, "can't be more than 8 bytes")
	})
	t.Run("2", func(t *testing.T) {
		data := []byte{1, 0xff}
		require.EqualValues(t, 256+255, int(MustUint64FromBytes(data)))
	})
}
