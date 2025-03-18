package global

import "time"

func (l *Global) RepeatInBackground(name string, period time.Duration, fun func() bool, skipFirst ...bool) {
	l.MarkWorkProcessStarted(name)
	l.Infof0("[%s] STARTED", name)

	go func() {
		defer func() {
			l.MarkWorkProcessStopped(name)
			l.Infof0("[%s] STOPPED", name)
		}()

		if len(skipFirst) == 0 || !skipFirst[0] {
			fun()
		}
		for {
			select {
			case <-l.Ctx().Done():
				return
			case <-time.After(period):
				if !fun() {
					return
				}
			}
		}
	}()
}
