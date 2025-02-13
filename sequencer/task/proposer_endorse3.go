package task

import (
	"time"

	"github.com/lunfardo314/proxima/core/attacher"
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
	a := p.ChooseFirstExtendEndorsePair(false, nil)
	if a == nil {
		p.Tracef(TraceTagEndorse3Proposer, "propose: ChooseFirstExtendEndorsePair returned nil")
		return nil, false
	}
	endorsing := a.Endorsing()[0]
	extending := a.Extending()
	if !a.Completed() {
		a.Close()
		p.Tracef(TraceTagEndorse3Proposer, "proposal [extend=%s, endorsing=%s] not complete 1", extending.IDShortString, endorsing.IDShortString)
		return nil, false
	}

	newOutputArrived := p.Backlog().ArrivedOutputsSince(p.slotData.lastTimeBacklogCheckedE2)
	p.slotData.lastTimeBacklogCheckedE2 = time.Now()

	// then try to add one endorsement more
	addedSecond := false
	endorsing0 := a.Endorsing()[0]
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
			checked, _ := p.Task.slotData.wasCombinationChecked(extending, endorsing, endorsementCandidate)
			if checked {
				continue
			}
		}

		if err := a.InsertEndorsement(endorsementCandidate); err == nil {
			addedSecond = true
			break //>>>> return attacher
		}
		p.Tracef(TraceTagEndorse3Proposer, "failed to include endorsement target %s", endorsementCandidate.IDShortString)
	}
	if !addedSecond {
		// no need to repeat job of endorse1
		a.Close()
		return nil, false
	}

	p.insertInputs(a)

	if !a.Completed() {
		a.Close()
		endorsing = a.Endorsing()[0]
		extending = a.Extending()
		p.Tracef(TraceTagEndorse3Proposer, "proposal [extend=%s, endorsing=%s] not complete 2", extending.IDShortString, endorsing.IDShortString)
		return nil, false
	}

	return a, false
}
