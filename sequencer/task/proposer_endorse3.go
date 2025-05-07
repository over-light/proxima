package task

import (
	"time"

	"github.com/lunfardo314/proxima/core/attacher"
	"github.com/lunfardo314/proxima/core/vertex"
)

// e3 is a proposer strategy which proposes transactions with 3 endorsements chosen by selecting
// endorsement targets with the priority of bigger coverage

const TraceTagEndorse3Proposer = "propose-endorse3"

func init() {
	registerProposerStrategy(&proposerStrategy{
		Name:             "endorse3",
		ShortName:        "e3",
		GenerateProposal: endorse3ProposeGenerator,
	})
}

func endorse3ProposeGenerator(p *proposer) (*attacher.IncrementalAttacher, bool) {
	if p.targetTs.IsSlotBoundary() {
		// the proposer does not generate branch transactions
		return nil, true
	}

	// CheckConflicts all pairs, in descending order
	a := p.ChooseFirstExtendEndorsePair(false, func(extend vertex.WrappedOutput, endorse *vertex.WrappedTx) bool {
		checked, consistent := p.taskData.slotData.wasCombinationChecked(extend, endorse)
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
		p.Tracef(TraceTagEndorse3Proposer, "proposal [extend=%s, endorsing=%s] not complete 1", extending.IDStringShort, endorsing0.IDShortString)
		return nil, false
	}

	var newOutputArrived bool
	p.slotData.withWriteLock(func() {
		newOutputArrived = p.Backlog().ArrivedOutputsSince(p.slotData.lastTimeBacklogCheckedE3)
		p.slotData.lastTimeBacklogCheckedE3 = time.Now()
	})

	// then try to add one endorsement more
	var endorsing1 *vertex.WrappedTx

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
			checked, _ := p.taskData.slotData.wasCombinationChecked(extending, endorsing0, endorsementCandidate)
			if checked {
				continue
			}
		}
		if err := a.InsertEndorsement(endorsementCandidate); err == nil {
			p.taskData.slotData.markCombinationChecked(true, extending, endorsing0, endorsementCandidate)
			endorsing1 = endorsementCandidate
			break //>>>> second endorsement
		} else {
			p.taskData.slotData.markCombinationChecked(false, extending, endorsing0, endorsementCandidate)
		}
		p.Tracef(TraceTagEndorse3Proposer, "failed to include endorsement target %s", endorsementCandidate.IDShortString)
	}
	if endorsing1 == nil {
		// no need to repeat job of endorse1
		a.Close()
		return nil, false
	}
	// try to add 3rd endorsement
	var endorsing2 *vertex.WrappedTx

	for _, endorsementCandidate := range p.Backlog().CandidatesToEndorseSorted(p.targetTs) {
		select {
		case <-p.ctx.Done():
			a.Close()
			return nil, true
		default:
		}
		if endorsementCandidate == endorsing0 || endorsementCandidate == endorsing1 {
			continue
		}
		if !newOutputArrived {
			checked, _ := p.taskData.slotData.wasCombinationChecked(extending, endorsing0, endorsing1, endorsementCandidate)
			if checked {
				continue
			}
		}
		if err := a.InsertEndorsement(endorsementCandidate); err == nil {
			p.taskData.slotData.markCombinationChecked(true, extending, endorsing0, endorsing1, endorsementCandidate)
			endorsing2 = endorsementCandidate
			break //>>>> third endorsement
		} else {
			p.taskData.slotData.markCombinationChecked(false, extending, endorsing0, endorsing1, endorsementCandidate)
		}
		p.Tracef(TraceTagEndorse3Proposer, "failed to include endorsement target %s", endorsementCandidate.IDShortString)
	}
	if endorsing2 == nil {
		// no need to repeat job of endorse2
		a.Close()
		return nil, false
	}

	// insert tag along and delegation outputs
	p.insertInputs(a)

	if !a.Completed() {
		a.Close()
		endorsing0 = a.Endorsing()[0]
		endorsing1 = a.Endorsing()[1]
		endorsing2 = a.Endorsing()[2]
		extending = a.Extending()
		p.Tracef(TraceTagEndorse3Proposer, "proposal [extend=%s, endorsing=%s, %s, %s] not complete 2",
			extending.IDStringShort, endorsing0.IDShortString, endorsing1.IDShortString, endorsing2.IDShortString)
		return nil, false
	}

	return a, false
}
