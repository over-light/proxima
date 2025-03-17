package checkgc

import (
	"sync"
	"weak"

	"github.com/lunfardo314/proxima/util/lines"
)

// debug tool

type List[T any] struct {
	sync.Mutex
	lst    []weak.Pointer[T]
	prnFun func(p *T) string
}

func NewList[T any](prnFun func(p *T) string) *List[T] {
	return &List[T]{
		lst:    make([]weak.Pointer[T], 0),
		prnFun: prnFun,
	}
}

func (gcp *List[T]) RegisterPointer(p *T) {
	gcp.lst = append(gcp.lst, weak.Make(p))
}

func (gcp *List[T]) LinesNotGCed(prefix ...string) *lines.Lines {
	ret := lines.New(prefix...)
	for i := range gcp.lst {
		if p := gcp.lst[i].Value(); p != nil {
			ret.Add(gcp.prnFun(p))
		}
	}
	return ret
}
