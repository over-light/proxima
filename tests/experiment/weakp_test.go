package experiment

import (
	"fmt"
	"runtime"
	"testing"
	"weak"

	"github.com/stretchr/testify/require"
)

type T struct {
	a int
	b int
}

func TestWeakPointerSlice(t *testing.T) {
	data := []byte("0123456789")
	fmt.Printf("original: '%s'\n", string(data))

	// make a weak pointer
	p0 := &data[0]
	p5 := &data[5]
	fmt.Printf("p0: '%c', p5: '%c'\n", *p0, *p5)

	weak0 := weak.Make(p0)
	weak5 := weak.Make(p5)

	p0 = weak0.Value()
	require.True(t, p0 != nil)
	p5 = weak5.Value()
	require.True(t, p5 != nil)
	fmt.Printf("p0: '%c', p5: '%c'\n", *p0, *p5)

	runtime.GC()
	fmt.Printf("----- run GC 1\n")

	runtime.KeepAlive(data)

	p0 = weak0.Value()
	require.True(t, p0 != nil)
	p5 = weak5.Value()
	require.True(t, p5 != nil)

	runtime.GC()
	fmt.Printf("----- run GC 2\n")

	p0 = weak0.Value()
	require.True(t, p0 == nil)
	p5 = weak5.Value()
	require.True(t, p5 == nil)
}
