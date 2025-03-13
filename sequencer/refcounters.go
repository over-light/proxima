package sequencer

import (
	"github.com/lunfardo314/proxima/core/vertex"
)

// ReferenceVID maintains reference counter for VID
func (seq *Sequencer) ReferenceVID(vid *vertex.WrappedTx) {
	seq.mutexReferenceCounters.Lock()
	defer seq.mutexReferenceCounters.Unlock()

	c := seq.referenceCounters[vid]
	if c == 0 {
		vid.SetFlagIsReferencedFromSequencer(true)
	}
	seq.referenceCounters[vid] = c + 1
}

// UnReferenceVID decrements counter and deletes flag if vid remains referenced
func (seq *Sequencer) UnReferenceVID(vid *vertex.WrappedTx) {
	seq.mutexReferenceCounters.Lock()
	defer seq.mutexReferenceCounters.Unlock()

	c, found := seq.referenceCounters[vid]
	if !found {
		return
	}
	seq.Assertf(c > 0, "c > 0")
	c--
	if c == 0 {
		delete(seq.referenceCounters, vid)
		vid.SetFlagIsReferencedFromSequencer(false)
	} else {
		seq.referenceCounters[vid] = c
	}
}
