package task

import (
	"time"

	"github.com/lunfardo314/proxima/core/attacher"
	"github.com/lunfardo314/proxima/core/vertex"
)

// e1 is a proposer strategy which endorses one other sequencer

const TraceTagEndorse1Proposer = "propose-endorse1"

func init() {
	registerProposerStrategy(&Strategy{
		Name:             "endorse1",
		ShortName:        "e1",
		GenerateProposal: endorse1ProposeGenerator,
	})
}

func endorse1ProposeGenerator(p *Proposer) (*attacher.IncrementalAttacher, bool) {
	if p.targetTs.IsSlotBoundary() {
		// the proposer does not generate branch transactions
		return nil, true
	}
	// choose extend-endorse pair with optimization. If that pair was chosen in the past and newOutputs didn't arrive
	// since last check, use that pair to create new attacher (if not conflicting)
	newOutputsArrived := p.Backlog().ArrivedOutputsSince(p.Task.slotData.lastTimeBacklogCheckedE1)
	p.Task.slotData.lastTimeBacklogCheckedE1 = time.Now()
	a := p.ChooseFirstExtendEndorsePair(false, func(extend vertex.WrappedOutput, endorse *vertex.WrappedTx) bool {
		if newOutputsArrived {
			// use pair with new tag-along outputs
			return true
		}
		alreadyChecked, _ := p.Task.slotData.wasCombinationChecked(extend, endorse)
		return !alreadyChecked
	})

	if a == nil {
		p.Tracef(TraceTagEndorse1Proposer, "propose: ChooseFirstExtendEndorsePair returned nil")
		return nil, false
	}
	if !a.Completed() {
		endorsing := a.Endorsing()[0]
		extending := a.Extending()
		p.Tracef(TraceTagTask, "proposal [extend=%s, endorsing=%s] not complete 1", extending.IDStringShort, endorsing.IDShortString)
		return nil, false
	}

	p.insertInputs(a)

	if !a.Completed() {
		endorsing := a.Endorsing()[0]
		extending := a.Extending()
		p.Tracef(TraceTagTask, "proposal [extend=%s, endorsing=%s] not complete 2", extending.IDStringShort, endorsing.IDShortString)
		return nil, false
	}

	return a, false
}
