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
	m         map[string]weak.Pointer[T]
	keyFun    func(p *T) string
	prnObjFun func(p *T) string
}

func New[T any](keyFun func(p *T) string, prnObjFun ...func(p *T) string) *List[T] {
	ret := &List[T]{
		m:      make(map[string]weak.Pointer[T], 0),
		keyFun: keyFun,
	}
	if len(prnObjFun) > 0 {
		ret.prnObjFun = prnObjFun[0]
	} else {
		ret.prnObjFun = keyFun
	}
	return ret
}

func (gcp *List[T]) Key(p *T) string {
	return gcp.keyFun(p)
}

func (gcp *List[T]) PrintObj(p *T) string {
	return gcp.prnObjFun(p)
}

func (gcp *List[T]) RegisterPointer(p *T) {
	gcp.Mutex.Lock()
	defer gcp.Mutex.Unlock()

	gcp.m[gcp.keyFun(p)] = weak.Make(p)
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
			ret.Add(gcp.keyFun(p))
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

func (gcp *List[T]) TrackPointerNotGCed(p *T, timeout time.Duration, panicOnTimeout ...bool) {
	gcp.RegisterPointer(p)
	key := gcp.Key(p)
	prnObj := gcp.PrintObj(p)
	objType := fmt.Sprintf("%T", p)
	objPointer := fmt.Sprintf("%p", p)

	nowis := time.Now()
	deadline := nowis.Add(timeout)
	go func() {
		for {
			time.Sleep(10 * time.Millisecond)

			gcp.Mutex.Lock()
			wp := gcp.m[key]
			gcp.Mutex.Unlock()

			strong := wp.Value()
			if strong == nil {
				fmt.Printf(">>>>>>>>>>>>>>>> TrackPointerNotGCed[%s,%s]: exit OK in %v: key = '%s'\n", objType, objPointer, time.Since(nowis), key)
				return
			}
			if time.Now().After(deadline) {
				msg := fmt.Sprintf(">>>>>>>>>>>>>>>> TrackPointerNotGCed[%s,%s]: GC timeout (%v after start tracking)\n%s", objType, objPointer, timeout, prnObj)
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
