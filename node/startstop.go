package node

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/lunfardo314/proxima/core"
	"github.com/lunfardo314/proxima/general"
	"github.com/lunfardo314/proxima/genesis"
	"github.com/lunfardo314/proxima/multistate"
	"github.com/lunfardo314/proxima/sequencer"
	"github.com/lunfardo314/proxima/txstore"
	"github.com/lunfardo314/proxima/utangle"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/workflow"
	"github.com/lunfardo314/unitrie/adaptors/badger_adaptor"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type ProximaNode struct {
	log             *zap.SugaredLogger
	multiStateStore *badger_adaptor.DB
	txStoreDB       *badger_adaptor.DB
	txStore         general.TxBytesStore
	uTangle         *utangle.UTXOTangle
	workflow        *workflow.Workflow
	sequencers      []*sequencer.Sequencer
	stopOnce        sync.Once
	ctx             context.Context
}

func New(ctx context.Context) *ProximaNode {
	return &ProximaNode{
		log:        newBootstrapLogger(),
		sequencers: make([]*sequencer.Sequencer, 0),
		ctx:        ctx,
	}
}

func (p *ProximaNode) initConfig() {
	pflag.Parse()
	err := viper.BindPFlags(pflag.CommandLine)
	util.AssertNoError(err)

	viper.SetConfigName("proxima")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	err = viper.ReadInConfig()
	util.AssertNoError(err)

	if viper.GetString(general.ConfigKeyMultiStateDbName) == "" {
		p.log.Errorf("multistate database not specified, cannot start the node")
		os.Exit(1)
	}
}

func (p *ProximaNode) Run() {
	p.log.Info(general.BannerString())
	p.initConfig()

	p.log = newNodeLoggerFromConfig()
	p.log.Info("---------------- starting up Proxima node --------------")

	err := util.CatchPanicOrError(func() error {
		p.startPProfIfEnabled()
		p.startMultiStateDB()
		p.startTxStore()
		p.loadUTXOTangle()
		p.startWorkflow()
		p.startSequencers()
		p.startApiServer()
		return nil
	})
	if err != nil {
		p.log.Errorf("error on startup: %v", err)
		os.Exit(1)
	}
	p.log.Infof("Proxima node has been started successfully")
	p.log.Debug("running in debug mode")
}

func (p *ProximaNode) Stop() {
	p.stopOnce.Do(func() {
		p.log.Info("stopping the node..")
		p.stop()
	})
}

func (p *ProximaNode) stop() {
	//p.stopMultiStateDB()
	//p.stopApiServer()

	if len(p.sequencers) > 0 {
		// stop sequencers
		var wg sync.WaitGroup
		for _, seq := range p.sequencers {
			seqCopy := seq
			wg.Add(1)
			go func() {
				seqCopy.Stop()
				wg.Done()
			}()
		}
		wg.Wait()
		p.log.Infof("all sequencers stopped")
	}

	if p.workflow != nil {
		p.workflow.Stop()
	}
	p.log.Info("node stopped")

}

func (p *ProximaNode) GetMultiStateDBName() string {
	return viper.GetString("multistate.name")
}

func (p *ProximaNode) startMultiStateDB() {
	dbname := p.GetMultiStateDBName()
	var err error
	bdb, err := badger_adaptor.OpenBadgerDB(dbname)
	if err != nil {
		p.log.Fatalf("can't open '%s'", dbname)
	}
	p.multiStateStore = badger_adaptor.New(bdb)
	p.log.Infof("opened multi-state DB '%s", dbname)

	go func() {
		<-p.ctx.Done()
		p.stopMultiStateDB()
	}()
}

func (p *ProximaNode) stopMultiStateDB() {
	if p.multiStateStore != nil {
		_ = p.multiStateStore.Close()
		p.log.Infof("multi-state database has been closed")
	}
	if p.txStoreDB != nil {
		_ = p.txStoreDB.Close()
		p.log.Infof("transaction store database has been closed")
	}
}

func (p *ProximaNode) startTxStore() {
	switch viper.GetString(general.ConfigKeyTxStoreType) {
	case "dummy":
		p.log.Infof("transaction store is 'dummy'")
		p.txStore = txstore.NewDummyTxBytesStore()

	case "db":
		name := viper.GetString(general.ConfigKeyTxStoreName)
		p.log.Infof("transaction store database name is '%s'", name)
		if name == "" {
			p.log.Errorf("transaction store database name not specified. Cannot start the node")
			p.Stop()
			os.Exit(1)
		}
		p.txStoreDB = badger_adaptor.New(badger_adaptor.MustCreateOrOpenBadgerDB(name))
		p.txStore = txstore.NewSimpleTxBytesStore(p.txStoreDB)
		p.log.Infof("opened DB '%s' as transaction store", name)

	case "url":
		panic("'url' type of transaction store is not supported yet")

	default:
		p.log.Errorf("transaction store type '%s' is wrong", viper.GetString(general.ConfigKeyTxStoreType))
		p.Stop()
		os.Exit(1)
	}
}

// MustCompatibleStateBootstrapData branches of the latest slot sorted by coverage descending
func mustReadStateIdentity(store general.StateStore) {
	rootRecords := multistate.FetchRootRecords(store, multistate.FetchLatestSlot(store))
	util.Assertf(len(rootRecords) > 0, "at least on root record expected")
	stateReader, err := multistate.NewSugaredReadableState(store, rootRecords[0].Root)
	util.AssertNoError(err)

	// it will panic if constraint libraries are incompatible
	genesis.MustStateIdentityDataFromBytes(stateReader.MustStateIdentityBytes())
}

func (p *ProximaNode) loadUTXOTangle() {
	mustReadStateIdentity(p.multiStateStore)

	p.uTangle = utangle.Load(p.multiStateStore, p.txStore)
	latestSlot := p.uTangle.LatestTimeSlot()
	currentSlot := core.LogicalTimeNow().TimeSlot()
	p.log.Infof("current time slot: %d, latest time slot in the multi-state: %d, lagging behind: %d slots",
		currentSlot, latestSlot, currentSlot-latestSlot)

	branches := multistate.FetchLatestBranches(p.multiStateStore)
	p.log.Infof("latest time slot %d contains %d branches", latestSlot, len(branches))
	for _, br := range branches {
		txid := br.Stem.ID.TransactionID()
		p.log.Infof("    branch %s : sequencer: %s, coverage: %s", txid.Short(), br.SequencerID.Short(), br.LedgerCoverage.String())
	}
	p.log.Infof("UTXO tangle has been created successfully")
}

func (p *ProximaNode) startWorkflow() {
	p.workflow = workflow.New(p.uTangle, workflow.WithGlobalConfigOptions)
	p.workflow.Start()
	p.workflow.StartPruner()
}

func (p *ProximaNode) startSequencers() {
	traceProposers := viper.GetStringMap("trace_proposers")

	for pname := range traceProposers {
		if viper.GetBool("trace_proposers." + pname) {
			sequencer.SetTraceProposer(pname, true)
			p.log.Infof("will be tracing proposer '%s'", pname)
		}
	}

	sequencers := viper.GetStringMap("sequencers")
	if len(sequencers) == 0 {
		p.log.Infof("No sequencers will be started")
		return
	}
	p.log.Infof("%d sequencer config profiles has been found", len(sequencers))

	seqNames := util.SortKeys(sequencers, func(k1, k2 string) bool {
		return k1 < k2
	})
	for _, name := range seqNames {
		seq, err := sequencer.NewFromConfig(p.workflow, name)
		if err != nil {
			p.log.Errorf("can't start sequencer '%s': '%v'", name, err)
			continue
		}
		if seq == nil {
			p.log.Infof("skipping sequencer '%s'", name)
			continue
		}
		seq.Run(p.ctx)

		p.log.Infof("started sequencer '%s', seqID: %s", name, seq.ID().String())
		p.sequencers = append(p.sequencers, seq)
		time.Sleep(500 * time.Millisecond)
	}
}
