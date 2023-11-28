package workflow

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lunfardo314/proxima/core"
	utangle "github.com/lunfardo314/proxima/utangle"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/lines"
)

const SolidifyConsumerName = "solidify"

type (
	SolidifyInputData struct {
		// if not nil, its is a message to notify Solidify consumer that new transaction (valid and solid) has arrived to the tangle
		newSolidDependency *utangle.WrappedTx
		// used if newTx is == nil
		*PrimaryTransactionData
		// If true, PrimaryTransactionData bears txid to be removed
		Remove bool
	}

	SolidifyConsumer struct {
		*Consumer[*SolidifyInputData]
		stopBackgroundChan chan struct{}
		// mutex for main data structures
		mutex sync.RWMutex
		// txPending is list of draft vertices waiting for solidification to be sent for validation
		txPending map[core.TransactionID]draftVertexData
		// txDependencies is a list of transaction IDs which are needed for solidification of pending tx-es
		txDependencies map[core.TransactionID]*txDependency
	}

	draftVertexData struct {
		*PrimaryTransactionData
		draftVertex *utangle.Vertex
		pulled      bool // transaction was received as a result of the pull request
		// stemInputAlreadyPulled for pull sequence and priorities
		stemInputAlreadyPulled            bool
		sequencerPredecessorAlreadyPulled bool
		allInputsAlreadyPulled            bool
	}

	txDependency struct {
		// whoIsWaiting a list of txID which depends on the txid in the key of txDependency map
		// The list should not be empty
		whoIsWaiting []*core.TransactionID
		// since when dependency is known
		since time.Time
	}
)

const (
	keepNotSolid = 10 * time.Second // only for testing. Must be longer in reality
)

func (w *Workflow) initSolidifyConsumer() {
	c := &SolidifyConsumer{
		Consumer:           NewConsumer[*SolidifyInputData](SolidifyConsumerName, w),
		txPending:          make(map[core.TransactionID]draftVertexData),
		txDependencies:     make(map[core.TransactionID]*txDependency),
		stopBackgroundChan: make(chan struct{}),
	}
	c.AddOnConsume(func(inp *SolidifyInputData) {
		if inp.Remove {
			c.traceTx(inp.PrimaryTransactionData, "IN (remove)")
			return
		}
		if inp.newSolidDependency == nil {
			c.traceTx(inp.PrimaryTransactionData, "IN (solidify)")
			return
		}
		c.traceTx(inp.PrimaryTransactionData, "IN (check dependency)")
	})
	c.AddOnConsume(c.consume)
	c.AddOnClosed(func() {
		close(c.stopBackgroundChan)
		w.validateConsumer.Stop()
		w.terminateWG.Done()
	})
	w.solidifyConsumer = c
	go c.backgroundLoop()
}

func (c *SolidifyConsumer) IsWaitedTransaction(txid *core.TransactionID) bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	_, ret := c.txDependencies[*txid]
	return ret
}

func (c *SolidifyConsumer) consume(inp *SolidifyInputData) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.setTrace(inp.PrimaryTransactionData.SourceType == TransactionSourceTypeAPI)

	if inp.Remove {
		// command to remove the transaction and other depending on it from the solidification pool
		inp.eventCallback(SolidifyConsumerName+".in.remove", inp.Tx)
		c.removeNonSolidifiableFutureCone(inp.Tx.ID())
		return
	}
	if inp.newSolidDependency == nil {
		// new transaction for solidification arrived
		inp.eventCallback(SolidifyConsumerName+".in.new", inp.Tx)
		c.glb.IncCounter(c.Name() + ".in.new")
		c.newVertexToSolidify(inp)
	} else {
		// new solid transaction has been appended to the tangle, probably some transactions are waiting for it
		inp.eventCallback(SolidifyConsumerName+".in.check", inp.Tx)
		c.glb.IncCounter(c.Name() + ".in.check")
		c.checkNewDependency(inp)
	}
}

func (c *SolidifyConsumer) newVertexToSolidify(inp *SolidifyInputData) {
	_, already := c.txPending[*inp.Tx.ID()]
	util.Assertf(!already, "transaction is in the solidifier isInPullList: %s", inp.Tx.IDString())

	// fetches available inputs, makes draftVertex
	draftVertex, err := c.glb.utxoTangle.MakeDraftVertex(inp.Tx)
	if err != nil {
		// not solidifiable
		c.IncCounter("err")
		c.removeNonSolidifiableFutureCone(inp.Tx.ID())
		c.glb.DropTransaction(*inp.Tx.ID(), "%v", err)
		return
	}

	if solid := !c.putIntoSolidifierIfNeeded(inp, draftVertex); solid {
		// all inputs solid. Send for validation
		c.passToValidation(inp.PrimaryTransactionData, draftVertex)
	}
}

func (c *SolidifyConsumer) passToValidation(primaryTxData *PrimaryTransactionData, draftVertex *utangle.Vertex) {
	util.Assertf(draftVertex.IsSolid(), "v.IsSolid()")
	c.traceTx(primaryTxData, "solidified in %v", time.Now().Sub(primaryTxData.ReceivedWhen))

	delete(c.txPending, *primaryTxData.Tx.ID())

	c.glb.validateConsumer.Push(&ValidateConsumerInputData{
		PrimaryTransactionData: primaryTxData,
		draftVertex:            draftVertex,
	})
}

// returns if draftVertex was placed into the solidifier for further tracking
func (c *SolidifyConsumer) putIntoSolidifierIfNeeded(inp *SolidifyInputData, draftVertex *utangle.Vertex) bool {
	unknownInputTxIDs := draftVertex.MissingInputTxIDSet()
	if len(unknownInputTxIDs) == 0 {
		c.IncCounter("new.solid")
		return false
	}
	// some inputs unknown
	inp.eventCallback("notsolid."+SolidifyConsumerName, inp.Tx)

	util.Assertf(!draftVertex.IsSolid(), "inconsistency 1")
	c.IncCounter("new.notsolid")
	for unknownTxID := range unknownInputTxIDs {
		c.traceTx(inp.PrimaryTransactionData, "unknown input tx %s", unknownTxID.StringShort())
	}

	// for each unknown input, add the new draftVertex to the list of txids
	// dependent on it (past cone tips, known consumers)
	nowis := time.Now()
	for wantedTxID := range unknownInputTxIDs {
		if dept, dependencyExists := c.txDependencies[wantedTxID]; dependencyExists {
			fmt.Printf(">>>>> add dept: wanted: %s, consumer: %s\n", wantedTxID.StringShort(), draftVertex.Tx.IDShort())

			dept.whoIsWaiting = append(dept.whoIsWaiting, draftVertex.Tx.ID())
		} else {
			c.txDependencies[wantedTxID] = &txDependency{
				since:        nowis,
				whoIsWaiting: util.List(draftVertex.Tx.ID()),
			}
		}
	}
	// add to the list of vertices waiting for solidification
	vd := draftVertexData{
		PrimaryTransactionData: inp.PrimaryTransactionData,
		draftVertex:            draftVertex,
		pulled:                 inp.WasPulled,
	}
	c.txPending[*draftVertex.Tx.ID()] = vd

	// optionally initialize pull request to other peers if needed
	c.pullIfNeeded(&vd)
	return true
}

// collectDependingFutureCone collects all known (to solidifier) txids from the future cone which directly or indirectly depend on the txid
// It is recursive traversing of the DAG in the opposite order in the future cone of dependencies
func (c *SolidifyConsumer) collectDependingFutureCone(txid *core.TransactionID, ret map[core.TransactionID]struct{}) {
	if _, already := ret[*txid]; already {
		return
	}
	if dep, isKnownDependency := c.txDependencies[*txid]; isKnownDependency {
		for _, txid1 := range dep.whoIsWaiting {
			c.collectDependingFutureCone(txid1, ret)
		}
	}
	ret[*txid] = struct{}{}
}

// removeNonSolidifiableFutureCone removes from solidifier all txids which directly or indirectly depend on txid
func (c *SolidifyConsumer) removeNonSolidifiableFutureCone(txid *core.TransactionID) {
	c.Log().Debugf("remove non-solidifiable future cone of %s", txid.StringShort())

	ns := make(map[core.TransactionID]struct{})
	c.collectDependingFutureCone(txid, ns)
	for txid1 := range ns {
		if v, ok := c.txPending[txid1]; ok {
			pendingDept := v.draftVertex.PendingDependenciesLines("   ").String()
			v.PrimaryTransactionData.eventCallback("finish.remove."+SolidifyConsumerName,
				fmt.Errorf("%s solidication problem. Pending dependencies:\n%s", txid1.StringShort(), pendingDept))
		}

		delete(c.txPending, txid1)
		delete(c.txDependencies, txid1)

		c.Log().Debugf("remove %s", txid1.StringShort())
	}
}

// checkNewDependency checks all pending transactions waiting for the new incoming transaction
// The new vertex has just been added to the tangle
func (c *SolidifyConsumer) checkNewDependency(inp *SolidifyInputData) {
	dep, isKnownDependency := c.txDependencies[*inp.Tx.ID()]
	if !isKnownDependency {
		return
	}
	// it is not needed in the dependencies list anymore
	delete(c.txDependencies, *inp.Tx.ID())

	whoIsWaiting := dep.whoIsWaiting
	util.Assertf(len(whoIsWaiting) > 0, "len(whoIsWaiting)>0")

	c.traceTx(inp.PrimaryTransactionData, "whoIsWaiting: %s", __txLstString(whoIsWaiting))

	// looping over pending vertices which are waiting for the dependency newTxID
	for _, txid := range whoIsWaiting {
		pending, found := c.txPending[*txid]
		if !found {
			c.Log().Debugf("%s was waiting for %s, not pending anymore", txid.StringShort(), inp.Tx.IDShort())
			// not pending anymore
			return
		}
		if conflict := pending.draftVertex.FetchMissingDependencies(c.glb.utxoTangle); conflict != nil {
			// tx cannot be solidified, remove
			c.removeNonSolidifiableFutureCone(txid)
			err := fmt.Errorf("conflict at %s", conflict.Short())
			inp.eventCallback("finish.fail."+SolidifyConsumerName, err)
			c.glb.DropTransaction(*txid, "%v", err)
			continue
		}
		if pending.draftVertex.IsSolid() {
			// all inputs are solid, send it to the validation
			c.passToValidation(pending.PrimaryTransactionData, pending.draftVertex)
			continue
		}
		//c.traceTx(pending.PrimaryTransactionData, "not solid yet. Missing: %s\nTransaction: %s",
		//	pending.draftVertex.MissingInputTxIDString(), pending.draftVertex.Lines().String())

		// ask for missing inputs from peering
		c.pullIfNeeded(&pending)
	}
}

func __txLstString(lst []*core.TransactionID) string {
	ret := lines.New()
	for _, txid := range lst {
		ret.Add(txid.StringShort())
	}
	return ret.Join(",")
}

const (
	pullImmediately                       = time.Duration(0)
	pullDelayFirstPeriodSequencer         = 1 * time.Second
	pullDelayFirstPeriodOtherTransactions = 1 * time.Second
)

func (c *SolidifyConsumer) pullIfNeeded(vd *draftVertexData) {
	if vd.draftVertex.IsSolid() {
		return
	}
	if vd.allInputsAlreadyPulled {
		return
	}
	if vd.Tx.IsBranchTransaction() && !vd.draftVertex.IsStemInputSolid() {
		// first need to solidify stem input. Only when stem input is solid, we pull the rest
		// this makes node synchronization more sequential, from past to present slot by slot
		if !vd.stemInputAlreadyPulled {
			// pull immediately
			c.pull(vd.Tx.SequencerTransactionData().StemOutputData.PredecessorOutputID.TransactionID(), pullImmediately)
			vd.stemInputAlreadyPulled = true
		}
		return
	}

	if vd.Tx.IsSequencerMilestone() {
		//stem is isInPullList solid, we can pull sequencer input
		if isSolid, seqInputIdx := vd.draftVertex.IsSequencerInputSolid(); !isSolid {
			seqInputOID := vd.Tx.MustInputAt(seqInputIdx)
			var delayFirst time.Duration
			if vd.WasPulled {
				delayFirst = pullImmediately
			} else {
				delayFirst = pullDelayFirstPeriodSequencer
			}
			c.pull(seqInputOID.TransactionID(), delayFirst)
			vd.sequencerPredecessorAlreadyPulled = true
			return
		}
	}

	// now we can pull the rest
	vd.draftVertex.MissingInputTxIDSet().ForEach(func(txid core.TransactionID) bool {
		var delayFirst time.Duration
		if vd.WasPulled {
			delayFirst = pullImmediately
		} else {
			delayFirst = pullDelayFirstPeriodOtherTransactions
		}
		c.pull(txid, delayFirst)
		return true
	})
}

func (c *SolidifyConsumer) pull(txid core.TransactionID, initialDelay time.Duration) {
	c.glb.pullConsumer.Push(&PullTxData{
		TxID:         txid,
		InitialDelay: initialDelay,
	})
}

const solidifyBackgroundLoopPeriod = 100 * time.Millisecond

func (c *SolidifyConsumer) backgroundLoop() {
	defer c.Log().Debugf("background loop stopped")

	for {
		select {
		case <-c.stopBackgroundChan:
			return
		case <-time.After(solidifyBackgroundLoopPeriod):
		}
		c.doBackgroundCheck()
	}
}

func (c *SolidifyConsumer) doBackgroundCheck() {
	toRemove := c.collectToRemove()
	if len(toRemove) > 0 {
		c.removeDueToDeadline(toRemove)

		for i := range toRemove {
			c.glb.DropTransaction(toRemove[i], "solidification timeout %v. Missing: %s",
				keepNotSolid, c.missingInputsString(toRemove[i]))
		}
	}
}

func (c *SolidifyConsumer) collectToRemove() []core.TransactionID {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	nowis := time.Now()
	ret := make([]core.TransactionID, 0)
	for txid, dep := range c.txDependencies {
		if nowis.After(dep.since.Add(keepNotSolid)) {
			ret = append(ret, txid)
		}
	}
	return ret
}

func (c *SolidifyConsumer) removeDueToDeadline(toRemove []core.TransactionID) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for _, txid := range toRemove {
		c.removeNonSolidifiableFutureCone(&txid)
	}
}

func (c *SolidifyConsumer) missingInputsString(txid core.TransactionID) string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if vertexData, found := c.txPending[txid]; found {
		return vertexData.draftVertex.MissingInputTxIDString()
	}
	return "(none)"
}

func (d *txDependency) __text(dep *core.TransactionID) string {
	txids := make([]string, 0)
	for _, id := range d.whoIsWaiting {
		txids = append(txids, id.StringShort())
	}
	return fmt.Sprintf("%s <- [%s]", dep.StringShort(), strings.Join(txids, ","))
}

func (c *SolidifyConsumer) DumpUnresolvedDependencies() *lines.Lines {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	ret := lines.New()
	ret.Add("======= unresolved dependencies in solidifier")
	for txid, v := range c.txDependencies {
		ret.Add(v.__text(&txid))
	}
	return ret
}

func (c *SolidifyConsumer) DumpPending() *lines.Lines {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	ret := lines.New()
	ret.Add("======= transactions pending in solidifier")
	for txid, v := range c.txPending {
		ret.Add("pending %s", txid.StringShort())
		ret.Append(v.draftVertex.PendingDependenciesLines("  "))
	}
	return ret
}
