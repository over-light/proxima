package trackgc

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

func New[T any](prnFun func(p *T) string) *List[T] {
	return &List[T]{
		m:      make(map[string]weak.Pointer[T], 0),
		prnFun: prnFun,
	}
}

func (gcp *List[T]) RegisterPointer(p *T) {
	gcp.Mutex.Lock()
	defer gcp.Mutex.Unlock()

	gcp.m[gcp.prnFun(p)] = weak.Make(p)
}

func (gcp *List[T]) Stats() (gced int, notgced int) {
	gcp.Mutex.Lock()
	defer gcp.Mutex.Unlock()

	for _, wp := range gcp.m {
		if p := wp.Value(); p != nil {
			notgced++
		} else {
			gced++
		}
	}
	return
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

	for pstr, wp := range gcp.m {
		if p := wp.Value(); p != nil {
			ret.Add("NOT GCed: %s", pstr)
		} else {
			ret.Add("    GCed: %s", pstr)
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

func (gcp *List[T]) TrackPointerGCed(p *T, msg string) {
	gcp.RegisterPointer(p)
	s := fmt.Sprintf("%p", p)

	go func() {
		for {
			time.Sleep(1 * time.Millisecond)

			gcp.Mutex.Lock()
			wp := gcp.m[s]
			gcp.Mutex.Unlock()

			if wp.Value() == nil {
				fmt.Printf(">>>>>>>>>>>>>>>> TrackPointerGCed GCed: '%s'\n", msg)
				return
			}
		}
	}()
}

func (gcp *List[T]) TrackPointerNotGCed(p *T, msg string, timeout time.Duration, panicOnTimeout ...bool) {
	gcp.RegisterPointer(p)
	s := fmt.Sprintf("%p", p)

	nowis := time.Now()
	deadline := nowis.Add(timeout)
	go func() {
		for {
			time.Sleep(10 * time.Millisecond)

			gcp.Mutex.Lock()
			wp := gcp.m[s]
			gcp.Mutex.Unlock()

			strong := wp.Value()
			if strong == nil {
				fmt.Printf(">>>>>>>>>>>>>>>> TrackPointerNotGCed: exit OK in %v: '%s'\n", time.Since(nowis), msg)
				return
			}
			if time.Now().After(deadline) {
				msg = fmt.Sprintf(">>>>>>>>>>>>>>>> TrackPointerNotGCed: GC timeout in %s (%v after start tracking) -- %s, string = %p", msg, timeout, msg, strong)
				if len(panicOnTimeout) > 0 && panicOnTimeout[0] {
					panic(msg)
				} else {
					fmt.Printf("%s\n", msg)
				}
				return
			}
		}
	}()
}
