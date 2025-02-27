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

func TestWeakPointer(t *testing.T) {
	a := new(string)
	println("original:", a)

	// make a weak pointer
	weakA := weak.Make(a)
	strong := weakA.Value()
	require.True(t, strong != nil)
	runtime.GC()

	// use weakA
	fmt.Printf("value: %v, %v\n", weakA.Value(), a)

	runtime.GC()

	// use weakA again
	strong = weakA.Value()
	fmt.Printf("value: %v\n", strong)
	require.True(t, strong == nil)
}
