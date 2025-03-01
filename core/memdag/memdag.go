package memdag

import (
	"fmt"
	"sort"
	"sync"
	"time"
	"weak"

	"github.com/lunfardo314/proxima/core/vertex"
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/util"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/maps"
)

type (
	environment interface {
		global.StartStop
		StateStore() multistate.StateStore
		MetricsRegistry() *prometheus.Registry
	}

	keepVertexData struct {
		*vertex.WrappedTx
		keepUntil time.Time
	}

	// MemDAG is a global map of all in-memory vertices of the transaction DAG
	MemDAG struct {
		environment

		// cache of vertices as weak pointers. Key of the map is transaction ID. Value of the map is *vertex.WrappedTx.
		// The pointer value *vertex.WrappedTx is used as a unique identifier of the transaction while being
		// loaded into the memory.
		// The vertices map may be seen as encoding table between transaction ID and
		// more economic (memory-wise) yet transient in-memory ID *vertex.WrappedTx
		// in most other data structures, such as attachers, transactions are represented as *vertex.WrappedTx
		mutex    sync.RWMutex
		vertices map[ledger.TransactionID]weak.Pointer[vertex.WrappedTx] // <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<
		keep     []keepVertexData

		// latestBranchSlot maintained by EvidenceBranchSlot
		latestBranchSlot        ledger.Slot
		latestHealthyBranchSlot ledger.Slot

		// cache of state readers. One state (trie) reader for the branch/root. When accessed through the cache,
		// reading is highly optimized because each state reader keeps its trie cache, so consequent calls to
		// HasUTXO, GetUTXO and similar does not require database involvement during attachment and solidification
		// in the same slot.
		// Inactive cached readers with their trie caches are constantly cleaned up by the pruner
		stateReadersMutex sync.RWMutex
		stateReaders      map[ledger.TransactionID]*cachedStateReader
	}

	cachedStateReader struct {
		multistate.IndexedStateReader
		rootRecord   *multistate.RootRecord
		lastActivity time.Time
	}
)

func New(env environment) *MemDAG {
	ret := &MemDAG{
		environment:  env,
		vertices:     make(map[ledger.TransactionID]weak.Pointer[vertex.WrappedTx]),
		keep:         []keepVertexData{},
		stateReaders: make(map[ledger.TransactionID]*cachedStateReader),
	}
	if env != nil {
		ret.RepeatInBackground("memdag-doMaintenance", 5*time.Second, func() bool {
			ret.doMaintenance() // GC-ing, pruning etc
			return true
		})
	}
	return ret
}

const (
	sharedStateReaderCacheSize = 3000
	vertexTTLSlots             = 6
	stateReaderTTLSlots        = 2
)

func (d *MemDAG) WithGlobalWriteLock(fun func()) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	fun()
}

func (d *MemDAG) GetVertexNoLock(txid *ledger.TransactionID) *vertex.WrappedTx {
	if weakp, found := d.vertices[*txid]; found {
		return weakp.Value()
	}
	return nil
}

func (d *MemDAG) GetVertex(txid *ledger.TransactionID) *vertex.WrappedTx {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	return d.GetVertexNoLock(txid)
}

// NumVertices number of vertices
func (d *MemDAG) NumVertices() int {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	return len(d.vertices)
}

func (d *MemDAG) NumStateReaders() int {
	d.stateReadersMutex.RLock()
	defer d.stateReadersMutex.RUnlock()

	return len(d.stateReaders)
}

var vertexTTL time.Duration

func _vertexTTL() time.Duration {
	if vertexTTL == 0 {
		vertexTTL = vertexTTLSlots * ledger.SlotDuration()
	}
	return vertexTTL
}

func (d *MemDAG) AddVertexNoLock(vid *vertex.WrappedTx) {
	util.Assertf(d.GetVertexNoLock(&vid.ID) == nil, "d.GetVertexNoLock(vid.ID())==nil")
	d.vertices[vid.ID] = weak.Make(vid)
	// will keep vid from GC for TTL
	d.keep = append(d.keep, keepVertexData{vid, time.Now().Add(_vertexTTL())})
}

// purgeGarbageCollectedVertices with global lock
func (d *MemDAG) purgeGarbageCollectedVertices() {
	d.WithGlobalWriteLock(func() {
		for txid, weakp := range d.vertices {
			if weakp.Value() == nil {
				delete(d.vertices, txid)
			}
		}
	})
}

func (d *MemDAG) doMaintenance() {
	deleted := d.updateKeepList()
	detachDeleted(deleted)
	d.purgeGarbageCollectedVertices()
	d.purgeCachedStateReaders()
}

func (d *MemDAG) updateKeepList() []keepVertexData {
	var deleted []keepVertexData
	d.WithGlobalWriteLock(func() {
		nowis := time.Now()
		d.keep, deleted = util.PurgeSliceExtended(d.keep, func(keepData keepVertexData) bool {
			return keepData.keepUntil.After(nowis)
		})
	})
	return deleted
}

func detachDeleted(lst []keepVertexData) {
	for i := range lst {
		lst[i].ConvertToVirtualTx()
	}
}

var stateReaderTTL time.Duration

func _stateReaderCacheTTL() time.Duration {
	if stateReaderTTL == 0 {
		stateReaderTTL = stateReaderTTLSlots * ledger.SlotDuration()
	}
	return stateReaderTTL
}

func (d *MemDAG) purgeCachedStateReaders() (int, int) {
	ttl := _stateReaderCacheTTL()
	count := 0

	d.stateReadersMutex.Lock()
	defer d.stateReadersMutex.Unlock()

	for txid, b := range d.stateReaders {
		if time.Since(b.lastActivity) > ttl {
			delete(d.stateReaders, txid)
			count++
		}
	}
	return count, len(d.stateReaders)
}

func (d *MemDAG) GetStateReaderForTheBranch(branchID ledger.TransactionID) multistate.IndexedStateReader {
	util.Assertf(branchID.IsBranchTransaction(), "GetStateReaderForTheBranchExt: branch tx expected. Got: %s", branchID.StringShort())

	d.stateReadersMutex.Lock()
	defer d.stateReadersMutex.Unlock()

	ret := d.stateReaders[branchID]
	if ret != nil {
		ret.lastActivity = time.Now()
		return ret.IndexedStateReader
	}
	rootRecord, found := multistate.FetchRootRecord(d.StateStore(), branchID)
	if !found {
		return nil
	}
	d.stateReaders[branchID] = &cachedStateReader{
		IndexedStateReader: multistate.MustNewReadable(d.StateStore(), rootRecord.Root, sharedStateReaderCacheSize),
		rootRecord:         &rootRecord,
		lastActivity:       time.Now(),
	}
	return d.stateReaders[branchID]
}

func (d *MemDAG) GetStemWrappedOutput(branch *ledger.TransactionID) (ret vertex.WrappedOutput) {
	if vid := d.GetVertex(branch); vid != nil {
		ret = vid.StemWrappedOutput()
	}
	return
}

func (d *MemDAG) HeaviestStateForLatestTimeSlotWithBaseline() (multistate.SugaredStateReader, *vertex.WrappedTx) {
	branchRecords := multistate.FetchLatestBranches(d.StateStore())
	util.Assertf(len(branchRecords) > 0, "len(branchRecords)>0")

	return multistate.MakeSugared(multistate.MustNewReadable(d.StateStore(), branchRecords[0].Root, 0)),
		d.GetVertex(branchRecords[0].TxID())
}

func (d *MemDAG) LatestReliableState() (multistate.SugaredStateReader, error) {
	branchRecord := multistate.FindLatestReliableBranch(d.StateStore(), global.FractionHealthyBranch)
	if branchRecord == nil {
		return multistate.SugaredStateReader{}, fmt.Errorf("LatestReliableState: can't find latest reliable branch")
	}
	return multistate.MakeSugared(multistate.MustNewReadable(d.StateStore(), branchRecord.Root, 0)), nil
}

func (d *MemDAG) MustLatestReliableState() multistate.SugaredStateReader {
	ret, err := d.LatestReliableState()
	util.AssertNoError(err)
	return ret
}

func (d *MemDAG) HeaviestStateForLatestTimeSlot() multistate.SugaredStateReader {
	rootRecords := multistate.FetchLatestRootRecords(d.StateStore())
	util.Assertf(len(rootRecords) > 0, "len(rootRecords)>0")

	return multistate.MakeSugared(multistate.MustNewReadable(d.StateStore(), rootRecords[0].Root, 0))
}

func (d *MemDAG) CheckTransactionInLRB(txid ledger.TransactionID, maxDepth int) (lrbid ledger.TransactionID, foundAtDepth int) {
	lrb, atDepth := multistate.CheckTransactionInLRB(d.StateStore(), txid, maxDepth, global.FractionHealthyBranch)
	foundAtDepth = atDepth
	if lrb != nil {
		lrbid = lrb.Stem.ID.TransactionID()
	}
	return
}

// WaitUntilTransactionInHeaviestState for testing mostly
func (d *MemDAG) WaitUntilTransactionInHeaviestState(txid ledger.TransactionID, timeout ...time.Duration) (*vertex.WrappedTx, error) {
	deadline := time.Now().Add(10 * time.Minute)
	if len(timeout) > 0 {
		deadline = time.Now().Add(timeout[0])
	}
	for {
		rdr, baseline := d.HeaviestStateForLatestTimeSlotWithBaseline()
		if rdr.KnowsCommittedTransaction(&txid) {
			return baseline, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("WaitUntilTransactionInHeaviestState: timeout")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// EvidenceBranchSlot maintains cached values
func (d *MemDAG) EvidenceBranchSlot(s ledger.Slot, isHealthy bool) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.latestBranchSlot < s {
		d.latestBranchSlot = s
	}
	if isHealthy {
		if d.latestHealthyBranchSlot < s {
			d.latestHealthyBranchSlot = s
		}
	}
}

// LatestBranchSlots return latest committed slots and the sync flag.
// The latter indicates if current node is in sync with the network.
// If network is unreachable or nobody else is active it will return false
// Node is out of sync if current slots are behind from now
// Being synced or not is subjective
func (d *MemDAG) LatestBranchSlots() (slot, healthySlot ledger.Slot, synced bool) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	if d.latestBranchSlot == 0 {
		d.latestBranchSlot = multistate.FetchLatestCommittedSlot(d.StateStore())
		if d.latestBranchSlot == 0 {
			synced = true
		}
	}
	if d.latestHealthyBranchSlot == 0 {
		healthyExists := false
		d.latestHealthyBranchSlot, healthyExists = multistate.FindLatestHealthySlot(d.StateStore(), global.FractionHealthyBranch)
		util.Assertf(healthyExists, "assume healthy slot exists: FIX IT")
	}
	nowSlot := ledger.TimeNow().Slot()
	// synced criterion. latest slot max 3 behind, latest healthy max 6 behind
	slot, healthySlot = d.latestBranchSlot, d.latestHealthyBranchSlot
	const (
		latestSlotBehindMax        = 2
		latestHealthySlotBehindMax = 6
	)
	synced = synced || (slot+latestSlotBehindMax > nowSlot && healthySlot+latestHealthySlotBehindMax > nowSlot)
	return
}

func (d *MemDAG) LatestHealthySlot() ledger.Slot {
	_, ret, _ := d.LatestBranchSlots()
	return ret
}

func (d *MemDAG) ParseMilestoneData(msVID *vertex.WrappedTx) (ret *ledger.MilestoneData) {
	msVID.Unwrap(vertex.UnwrapOptions{
		Vertex: func(v *vertex.Vertex) {
			ret = ledger.ParseMilestoneData(v.Tx.SequencerOutput().Output)
		},
		VirtualTx: func(v *vertex.VirtualTransaction) {
			seqOut, _ := v.SequencerOutputs()
			ret = ledger.ParseMilestoneData(seqOut)
		},
	})
	return
}

// Vertices to avoid global lock while traversing all utangle
func (d *MemDAG) Vertices() []*vertex.WrappedTx {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	ret := make([]*vertex.WrappedTx, 0, len(d.vertices))
	for _, weakp := range d.vertices {
		if strongP := weakp.Value(); strongP != nil {
			ret = append(ret, strongP)
		}
	}
	return ret
}

func (d *MemDAG) VerticesFiltered(filterByID func(txid *ledger.TransactionID) bool) []*vertex.WrappedTx {
	return util.PurgeSlice(d.Vertices(), func(vid *vertex.WrappedTx) bool {
		return filterByID(&vid.ID)
	})
}

func (d *MemDAG) VerticesDescending() []*vertex.WrappedTx {
	ret := d.Vertices()
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Timestamp().After(ret[j].Timestamp())
	})
	return ret
}

// RecreateVertexMap to avoid memory leak
func (d *MemDAG) RecreateVertexMap() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.vertices = maps.Clone(d.vertices)
}
