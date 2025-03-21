package workflow

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/lunfardo314/proxima/core/memdag"
	"github.com/lunfardo314/proxima/core/txmetadata"
	"github.com/lunfardo314/proxima/core/vertex"
	"github.com/lunfardo314/proxima/core/work_process/events"
	"github.com/lunfardo314/proxima/core/work_process/poker"
	"github.com/lunfardo314/proxima/core/work_process/pull_tx_server"
	"github.com/lunfardo314/proxima/core/work_process/snapshot"
	"github.com/lunfardo314/proxima/core/work_process/tippool"
	"github.com/lunfardo314/proxima/core/work_process/txinput_queue"
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/peering"
	"github.com/lunfardo314/proxima/util/eventtype"
	"github.com/lunfardo314/proxima/util/set"
	"github.com/spf13/viper"
)

type (
	environment interface {
		global.NodeGlobal
		StateStore() multistate.StateStore
		TxBytesStore() global.TxBytesStore
		PullFromNPeers(nPeers int, txid ledger.TransactionID) int
		GetOwnSequencerID() *ledger.ChainID
		EvidencePastConeSize(sz int)
		EvidenceNumberOfTxDependencies(n int)
		SnapshotBranchID() ledger.TransactionID
		DurationSinceLastMessageFromPeer() time.Duration
		SelfPeerID() peer.ID
	}

	Workflow struct {
		environment
		*memdag.MemDAG
		cfg          *ConfigParams
		peers        *peering.Peers
		earliestSlot ledger.Slot // cached, immutable
		// queues and daemons
		pullTxServer *pull_tx_server.PullTxServer
		poker        *poker.Poker
		events       *events.Events
		txInputQueue *txinput_queue.TxInputQueue
		tippool      *tippool.SequencerTips
		// particular event handlers
		txListener *txListener
		//
		enableTrace    atomic.Bool
		traceTagsMutex sync.RWMutex
		traceTags      set.Set[string]
	}
)

var EventNewTx = eventtype.RegisterNew[*vertex.WrappedTx]("new tx") // event may be posted more than once for the transaction

const recreateMapPeriod = time.Minute

func Start(env environment, peers *peering.Peers, opts ...ConfigOption) *Workflow {
	cfg := defaultConfigParams()
	for _, opt := range opts {
		opt(&cfg)
	}
	cfg.log(env.Log())

	ret := &Workflow{
		environment:  env,
		cfg:          &cfg,
		peers:        peers,
		traceTags:    set.New[string](),
		earliestSlot: multistate.FetchEarliestSlot(env.StateStore()),
	}
	ret.MemDAG = memdag.New(ret)
	ret.poker = poker.New(ret)
	ret.events = events.New(ret)
	ret.pullTxServer = pull_tx_server.New(ret)
	ret.tippool = tippool.New(ret)
	ret.txInputQueue = txinput_queue.New(ret)
	snapshot.Start(ret)
	ret.startListeningTransactions()

	ret.peers.OnReceiveTxBytes(func(from peer.ID, txBytes []byte, metadata *txmetadata.TransactionMetadata, txData []byte) {
		ret.TxBytesInFromPeerQueued(txBytes, metadata, from, txData)
	})

	ret.peers.OnReceivePullTxRequest(func(from peer.ID, txid ledger.TransactionID) {
		ret.pullTxServer.Push(&pull_tx_server.Input{
			TxID:   txid,
			PeerID: from,
		})
	})

	// hopefully protects against memory leak
	ret.RepeatInBackground("workflow_recreate_map_loop", recreateMapPeriod, func() bool {
		ret.RecreateVertexMap()
		return true
	})
	return ret
}

func StartFromConfig(env environment, peers *peering.Peers) *Workflow {
	opts := make([]ConfigOption, 0)
	if viper.GetBool("workflow.do_not_start_pruner") {
		opts = append(opts, OptionDisableMemDAGGC)
	}
	if viper.GetBool("workflow.sync_manager.enable") {
		opts = append(opts, OptionEnableSyncManager)
	}
	return Start(env, peers, opts...)
}
