package checkgc

import (
	"fmt"
	"sync"
	"time"
	"weak"

	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/lines"
)

// debug tool

type List[T any] struct {
	sync.Mutex
	m      map[string]weak.Pointer[T]
	prnFun func(p *T) string
}

func NewList[T any](prnFun func(p *T) string) *List[T] {
	return &List[T]{
		m:      make(map[string]weak.Pointer[T], 0),
		prnFun: prnFun,
	}
}

func (gcp *List[T]) RegisterPointer(p *T) {
	gcp.Mutex.Lock()
	defer gcp.Mutex.Unlock()

	s := fmt.Sprintf("%p", p)
	gcp.m[s] = weak.Make(p)
}

func (gcp *List[T]) LinesNotGCed(prefix ...string) *lines.Lines {
	ret := lines.New(prefix...)

	gcp.Mutex.Lock()
	defer gcp.Mutex.Unlock()

	for _, wp := range gcp.m {
		if p := wp.Value(); p != nil {
			ret.Add(gcp.prnFun(p))
		}
	}
	return ret
}

func (gcp *List[T]) LinesOfTracked(prefix ...string) *lines.Lines {
	ret := lines.New(prefix...)

	gcp.Mutex.Lock()
	defer gcp.Mutex.Unlock()

	for _, wp := range gcp.m {
		if p := wp.Value(); p != nil {
			ret.Add("NOT GCed: %s", gcp.prnFun(p))
		} else {
			ret.Add("    GCed: %s", gcp.prnFun(p))
		}
	}
	return ret
}

func (gcp *List[T]) IsPointerGCed(p *T) bool {
	gcp.Mutex.Lock()
	defer gcp.Mutex.Unlock()

	s := fmt.Sprintf("%p", p)
	wp, found := gcp.m[s]
	util.Assertf(found, "checkgc: pointer %s not found", s)
	return wp.Value() == nil
}

func (gcp *List[T]) TrackPointer(p *T, msg string) {
	gcp.RegisterPointer(p)
	s := fmt.Sprintf("%p", p)

	go func() {
		for {
			time.Sleep(1 * time.Millisecond)

			gcp.Mutex.Lock()
			if wp := gcp.m[s]; wp.Value() == nil {
				fmt.Printf(">>>>>>>>>>>>>>>> TrackPointer GCed: '%s'\n", msg)
			}
			gcp.Mutex.Unlock()
		}
	}()
}
