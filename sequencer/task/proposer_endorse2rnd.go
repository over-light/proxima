package task

import (
	"time"

	"github.com/lunfardo314/proxima/core/attacher"
	"github.com/lunfardo314/proxima/core/vertex"
)

// r2 is a proposer strategy which proposes transactions with 2 endorsements chosen by selecting endorsement targets at random

const TraceTagEndorse2RndProposer = "propose-endorse2rnd"

func init() {
	registerProposerStrategy(&Strategy{
		Name:             "endorse2rnd",
		ShortName:        "r2",
		GenerateProposal: endorse2RndProposeGenerator,
	})
}

func endorse2RndProposeGenerator(p *Proposer) (*attacher.IncrementalAttacher, bool) {
	if p.targetTs.IsSlotBoundary() {
		// the proposer does not generate branch transactions
		return nil, true
	}

	// Check peers in RANDOM order
	a := p.ChooseFirstExtendEndorsePair(true, func(extend vertex.WrappedOutput, endorse *vertex.WrappedTx) bool {
		checked, consistent := p.Task.slotData.wasCombinationChecked(extend, endorse)
		return !checked || consistent
	})
	if a == nil {
		p.Tracef(TraceTagEndorse2RndProposer, "propose: ChooseFirstExtendEndorsePair returned nil")
		return nil, false
	}
	endorsing := a.Endorsing()[0]
	extending := a.Extending()
	if !a.Completed() {
		a.Close()
		p.Tracef(TraceTagEndorse2RndProposer, "proposal [extend=%s, endorsing=%s] not complete 1", extending.IDShortString, endorsing.IDShortString)
		return nil, false
	}

	newOutputArrived := p.Backlog().ArrivedOutputsSince(p.slotData.lastTimeBacklogCheckedR2)
	p.slotData.lastTimeBacklogCheckedR2 = time.Now()

	// then try to add one endorsement more
	addedSecond := false
	endorsing0 := a.Endorsing()[0]
	// RANDOM order
	for _, endorsementCandidate := range p.Backlog().CandidatesToEndorseShuffled(p.targetTs) {
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
			checked, _ := p.slotData.wasCombinationChecked(extending, endorsing, endorsementCandidate)
			if checked {
				continue
			}
		}

		if err := a.InsertEndorsement(endorsementCandidate); err == nil {
			p.Task.slotData.markCombinationChecked(true, extending, endorsing, endorsementCandidate)
			addedSecond = true
			break //>>>> return attacher
		} else {
			p.Task.slotData.markCombinationChecked(false, extending, endorsing, endorsementCandidate)
		}

		p.Tracef(TraceTagEndorse2RndProposer, "failed to include endorsement target %s", endorsementCandidate.IDShortString)
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
		p.Tracef(TraceTagEndorse2Proposer, "proposal [extend=%s, endorsing=%s] not complete 2", extending.IDShortString, endorsing.IDShortString)
		return nil, false
	}

	return a, false
}
