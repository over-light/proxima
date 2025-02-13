package task

import (
	"time"

	"github.com/lunfardo314/proxima/core/attacher"
	"github.com/lunfardo314/proxima/core/vertex"
)

const TraceTagEndorse3Proposer = "propose-endorse3"

// TODO WIP

//func init() {
//	registerProposerStrategy(&Strategy{
//		Name:             "endorse3",
//		ShortName:        "e3",
//		GenerateProposal: endorse3ProposeGenerator,
//	})
//}

func endorse3ProposeGenerator(p *Proposer) (*attacher.IncrementalAttacher, bool) {
	if p.targetTs.IsSlotBoundary() {
		// the proposer does not generate branch transactions
		return nil, true
	}

	// Check all pairs, in descending order
	a := p.ChooseFirstExtendEndorsePair(false, func(extend vertex.WrappedOutput, endorse *vertex.WrappedTx) bool {
		checked, consistent := p.Task.slotData.wasCombinationChecked(extend, endorse)
		return !checked || consistent
	})
	if a == nil {
		p.Tracef(TraceTagEndorse3Proposer, "propose: ChooseFirstExtendEndorsePair returned nil")
		return nil, false
	}
	endorsing0 := a.Endorsing()[0]
	extending := a.Extending()
	if !a.Completed() {
		a.Close()
		p.Tracef(TraceTagEndorse3Proposer, "proposal [extend=%s, endorsing=%s] not complete 1", extending.IDShortString, endorsing0.IDShortString)
		return nil, false
	}

	newOutputArrived := p.Backlog().ArrivedOutputsSince(p.slotData.lastTimeBacklogCheckedE3)
	p.slotData.lastTimeBacklogCheckedE3 = time.Now()

	// then try to add one endorsement more
	var endorsement1 *vertex.WrappedTx

	for _, endorsementCandidate := range p.Backlog().CandidatesToEndorseSorted(p.targetTs) {
		select {
		case <-p.ctx.Done():
			a.Close()
			return nil, true
		default:
		}
		if endorsementCandidate == endorsing0 {
			continue
		}
		if !newOutputArrived {
			checked, _ := p.Task.slotData.wasCombinationChecked(extending, endorsing0, endorsementCandidate)
			if checked {
				continue
			}
		}
		if err := a.InsertEndorsement(endorsementCandidate); err == nil {
			p.Task.slotData.markCombinationChecked(true, extending, endorsing0, endorsementCandidate)
			endorsement1 = endorsementCandidate
			break //>>>> second endorsement
		} else {
			p.Task.slotData.markCombinationChecked(false, extending, endorsing0, endorsementCandidate)
		}
		p.Tracef(TraceTagEndorse3Proposer, "failed to include endorsement target %s", endorsementCandidate.IDShortString)
	}
	if endorsement1 == nil {
		// no need to repeat job of endorse1
		a.Close()
		return nil, false
	}
	// try to add 3rd endorsement

	// insert tag along and delegation outputs
	p.insertInputs(a)

	if !a.Completed() {
		a.Close()
		endorsing0 = a.Endorsing()[0]
		extending = a.Extending()
		p.Tracef(TraceTagEndorse3Proposer, "proposal [extend=%s, endorsing=%s] not complete 2", extending.IDShortString, endorsing0.IDShortString)
		return nil, false
	}

	return a, false
}
