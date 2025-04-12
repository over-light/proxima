package task

import (
	"github.com/lunfardo314/proxima/core/attacher"
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/util"
)

// boot proposer generates non-branch transaction proposals with LRB as explicit baseline
// when own latest milestone is more than 1 slot in the past from now
// The purpose of it is to bootstrap the network. When network starts from scratch, all start UTXOs of sequencers
// are in the past. It makes impossible to issue sequencer transaction with implicit baseline,
// because there's no what to endorse.
// With boot proposer, sequencer issues non-branch transactions by bypassing endorsement and setting explicit
// baseline branch. Thus, transaction can be solidified and when several sequencers are issuing bootstrap transactions,
// the next ones can endorse it and ledger coverage start groving.
// After the bootstrap phase, the boot proposer becomes idle

const (
	TraceTagBootProposer = "propose-boot"
)

func init() {
	registerProposerStrategy(&proposerStrategy{
		Name:             "boot",
		ShortName:        "x",
		GenerateProposal: bootProposeGenerator,
	})
}

func bootProposeGenerator(p *proposer) (*attacher.IncrementalAttacher, bool) {
	p.Tracef(TraceTagBootProposer, "IN base proposer %s", p.Name)

	extend := p.OwnLatestMilestoneOutput()
	if extend.VID == nil {
		p.Log().Warnf("BootProposer-%s: can't find own milestone output", p.Name)
		return nil, true
	}

	if p.targetTs.IsSlotBoundary() || extend.VID.Slot()+1 >= p.targetTs.Slot {
		// idle phase of the base proposer
		return nil, true
	}

	lrb := multistate.FindLatestReliableBranch(p.StateStore(), global.FractionHealthyBranch)
	if extend.VID == nil {
		p.Log().Warnf("BootProposer-%s: can't find latest reliable branch", p.Name)
		return nil, true
	}

	a, err := attacher.NewIncrementalAttacherWithExplicitBaseline(p.Name, p.environment, p.targetTs, extend, lrb.Stem.ID.TransactionID())
	if err != nil {
		p.Tracef(TraceTagBootProposer, "%s can't create attacher: '%v'", p.Name, err)
		return nil, true
	}
	p.Tracef(TraceTagBootProposer, "%s created attacher with baseline %s, cov: %s",
		p.Name, a.BaselineBranch().IDShortString, func() string { return util.Th(a.LedgerCoverage()) },
	)

	p.Tracef(TraceTagBootProposer, "exit with proposal in %s: extend = %s",
		p.Name, extend.IDStringShort)
	return a, true
}
