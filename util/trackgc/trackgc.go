// Package trackgc contains tools to track garbage collection
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

type Options struct {
	timeout        time.Duration
	reportTimeout  bool
	panicOnTimeout bool
	reportExit     bool
}

var _optionsDefault = Options{
	timeout:        time.Second * 10,
	panicOnTimeout: false,
	reportTimeout:  true,
	reportExit:     true,
}

func WithTimeout(to time.Duration) func(opt *Options) {
	return func(opt *Options) {
		opt.timeout = to
	}
}

func WithPanicOnTimeout(yes bool) func(opt *Options) {
	return func(opt *Options) {
		opt.panicOnTimeout = yes
	}
}

func WithReportTimeout(yes bool) func(opt *Options) {
	return func(opt *Options) {
		opt.reportTimeout = yes
	}
}

func WithReportGC(yes bool) func(opt *Options) {
	return func(opt *Options) {
		opt.reportExit = yes
	}
}

func (gcp *List[T]) TrackPointerNotGCed(p *T, opts ...func(opt *Options)) {
	options := _optionsDefault
	for _, opt := range opts {
		opt(&options)
	}
	gcp.RegisterPointer(p)
	key := gcp.Key(p)
	prnObj := gcp.PrintObj(p)
	objType := fmt.Sprintf("%T", p)
	objPointer := fmt.Sprintf("%p", p)

	nowis := time.Now()
	deadline := nowis.Add(options.timeout)
	go func() {
		for {
			time.Sleep(10 * time.Millisecond)

			gcp.Mutex.Lock()
			wp := gcp.m[key]
			gcp.Mutex.Unlock()

			strong := wp.Value()
			if strong == nil {
				if options.reportExit {
					fmt.Printf(">>>>>>>>>>>>>>>> TrackPointerNotGCed[%s,%s]: GCed OK in %v: key = '%s'\n", objType, objPointer, time.Since(nowis), key)
				}
				return
			}
			if time.Now().After(deadline) {
				msg := fmt.Sprintf(">>>>>>>>>>>>>>>> TrackPointerNotGCed[%s,%s]: GC TIMEOUT (%v since start tracking)\n%s", objType, objPointer, time.Since(nowis), prnObj)
				if options.panicOnTimeout {
					panic(msg)
				} else {
					if options.reportTimeout {
						fmt.Printf("%s\n", msg)
					}
				}
				time.Sleep(time.Second)
			}
		}
	}()
}

func MustGoWithTimeout(fun func(), name string, timeout time.Duration) {
	ch := make(chan struct{})
	go func() {
		fun()
		close(ch)
	}()
	go func() {
		select {
		case <-ch:
			return
		case <-time.After(timeout):
			panic(fmt.Sprintf("goroutine '%s' didn't finish in timeout %v", name, timeout))
		}
	}()
}
