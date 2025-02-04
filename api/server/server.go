package server

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/lunfardo314/proxima/api"
	"github.com/lunfardo314/proxima/core/work_process/tippool"
	"github.com/lunfardo314/proxima/global"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/util"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/slices"
)

type (
	environment interface {
		global.Logging
		global.Metrics
		GetNodeInfo() *global.NodeInfo
		GetSyncInfo() *api.SyncInfo
		GetPeersInfo() *api.PeersInfo
		LatestReliableState() (multistate.SugaredStateReader, error)
		CheckTransactionInLRB(txid ledger.TransactionID, maxDepth int) (lrbid ledger.TransactionID, foundAtDepth int)
		SubmitTxBytesFromAPI(txBytes []byte)
		GetLatestReliableBranch() *multistate.BranchData
		StateStore() multistate.StateStore
		TxBytesStore() global.TxBytesStore
		GetKnownLatestMilestonesJSONAble() map[string]tippool.LatestSequencerTipDataJSONAble
	}

	server struct {
		*http.Server
		environment
		metrics
	}

	metrics struct {
		totalRequests prometheus.Counter
	}
)

const TraceTag = "apiServer"

func (srv *server) registerHandlers() {
	// GET request format: '/api/v1/get_ledger_id'
	srv.addHandler(api.PathGetLedgerID, srv.getLedgerID)
	// GET request format: '/api/v1/get_account_outputs?accountable=<EasyFL source form of the accountable lock constraint>'
	srv.addHandler(api.PathGetAccountOutputs, srv.getAccountOutputs)
	// GET request format: '/api/v1/get_account_simple_siglocked?addr=<a(0x....)>'
	srv.addHandler(api.PathGetAccountSimpleSiglockedOutputs, srv.getAccountSimpleSigLockedOutputs)
	// GET request format: '/api/v1/get_outputs_for_amount?addr=<a(0x....)>&amount=<amount>'
	srv.addHandler(api.PathGetOutputsForAmount, srv.getOutputsForAmount)
	// GET request format: '/api/v1/get_chained_outputs?accountable=<EasyFL source form of the accountable lock constraint>'
	srv.addHandler(api.PathGetChainedOutputs, srv.getChainedOutputs)
	// GET request format: '/api/v1/get_chain_output?chainid=<hex-encoded chain ID>'
	srv.addHandler(api.PathGetChainOutput, srv.getChainOutput)
	// GET request format: '/api/v1/get_output?id=<hex-encoded output ID>'
	srv.addHandler(api.PathGetOutput, srv.getOutput)
	// POST request format '/api/v1/submit_tx'. Feedback only on parsing error, otherwise async posting
	srv.addHandler(api.PathSubmitTransaction, srv.submitTx)
	// GET sync info from the node '/api/v1/sync_info'
	srv.addHandler(api.PathGetSyncInfo, srv.getSyncInfo)
	// GET node info from the node '/api/v1/node_info'
	srv.addHandler(api.PathGetNodeInfo, srv.getNodeInfo)
	// GET peers info from the node '/api/v1/peers_info'
	srv.addHandler(api.PathGetPeersInfo, srv.getPeersInfo)
	// GET latest reliable branch '/api/v1/get_latest_reliable_branch'
	srv.addHandler(api.PathGetLatestReliableBranch, srv.getLatestReliableBranch)
	// GET latest reliable branch and check if transaction ID is in it '/check_txid_in_lrb?txid=<hex-encoded transaction ID>[&max_depth=<max depth in LRB>]'
	srv.addHandler(api.PathCheckTxIDInLRB, srv.checkTxIDIncludedInLRB)
	// GET last milestone list
	srv.addHandler(api.PathGetLastKnownSequencerMilestones, srv.getMilestoneList)
	// GET main chain of branches /get_mainchain?[max=]
	srv.addHandler(api.PathGetMainChain, srv.getMainChain)
	// GET all chains in the LRB /get_all_chains
	srv.addHandler(api.PathGetAllChains, srv.getAllChains)
	// GET dashboard for node
	srv.addHandler(api.PathGetDashboard, srv.getDashboard)

	// register handlers of tx API
	srv.registerTxAPIHandlers()
}

func (srv *server) getLedgerID(w http.ResponseWriter, _ *http.Request) {
	setHeader(w)

	srv.Tracef(TraceTag, "getLedgerID invoked")

	resp := &api.LedgerID{
		LedgerIDBytes: hex.EncodeToString(ledger.L().ID.Bytes()),
	}
	respBin, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	_, err = w.Write(respBin)
	util.AssertNoError(err)
}

const absoluteMaximumOfReturnedOutputs = 2000

func (srv *server) _getAccountOutputsWithFilter(w http.ResponseWriter, r *http.Request, addr ledger.Accountable, filter func(oid ledger.OutputID, o *ledger.Output) bool) {
	var err error
	maxOutputs := absoluteMaximumOfReturnedOutputs
	lst, ok := r.URL.Query()["max_outputs"]
	if ok {
		if len(lst) != 1 {
			writeErr(w, "wrong parameter 'max_outputs'")
			return
		}
		maxOutputs, err = strconv.Atoi(lst[0])
		if err != nil {
			writeErr(w, err.Error())
			return
		}
		if maxOutputs > absoluteMaximumOfReturnedOutputs {
			maxOutputs = absoluteMaximumOfReturnedOutputs
		}
	}

	doSorting := false
	sortDesc := false
	lst, ok = r.URL.Query()["sort"]
	if ok {
		if len(lst) != 1 || (lst[0] != "asc" && lst[0] != "desc") {
			writeErr(w, "wrong parameter 'sort'")
			return
		}
		doSorting = true
		sortDesc = lst[0] == "desc"
	}

	outs := make([]*ledger.OutputWithID, 0)
	resp := &api.OutputList{
		Outputs: make(map[string]string),
	}

	err = srv.withLRB(func(rdr multistate.SugaredStateReader) (errRet error) {
		lrbid := rdr.GetStemOutput().ID.TransactionID()
		resp.LRBID = lrbid.StringHex()
		err1 := rdr.IterateOutputsForAccount(addr, func(oid ledger.OutputID, o *ledger.Output) bool {
			if filter(oid, o) {
				outs = append(outs, &ledger.OutputWithID{
					ID:     oid,
					Output: o,
				})
			}
			return true
		})
		if err1 != nil {
			return err1
		}
		return
	})
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	if doSorting {
		sort.Slice(outs, func(i, j int) bool {
			if sortDesc {
				return bytes.Compare(outs[i].ID[:], outs[j].ID[:]) > 0
			}
			return bytes.Compare(outs[i].ID[:], outs[j].ID[:]) < 0

		})
	}
	outs = util.TrimSlice(outs, maxOutputs)
	for _, o := range outs {
		resp.Outputs[o.ID.StringHex()] = o.Output.Hex()
	}

	respBin, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	_, err = w.Write(respBin)
	util.AssertNoError(err)
}

// getAccountOutputs returns all outputs from the account because of random ordering and limits
// Lock can be of any type
func (srv *server) getAccountOutputs(w http.ResponseWriter, r *http.Request) {
	setHeader(w)

	lst, ok := r.URL.Query()["accountable"]
	if !ok || len(lst) != 1 {
		writeErr(w, "wrong parameter 'accountable' in request 'get_account_outputs'")
		return
	}
	accountable, err := ledger.AccountableFromSource(lst[0])
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	srv._getAccountOutputsWithFilter(w, r, accountable, func(oid ledger.OutputID, o *ledger.Output) bool {
		return true
	})
}

// getAccountSimpleSigLockedOutputs returns outputs locked with simple AddressED25519 lock
func (srv *server) getAccountSimpleSigLockedOutputs(w http.ResponseWriter, r *http.Request) {
	lst, ok := r.URL.Query()["addr"]
	if !ok || len(lst) != 1 {
		writeErr(w, "wrong parameter 'addr' in request 'get_account_simple_siglocked_outputs'")
		return
	}
	addr, err := ledger.AddressED25519FromSource(lst[0])
	if err != nil {
		writeErr(w, err.Error())
		return
	}

	srv._getAccountOutputsWithFilter(w, r, addr, func(_ ledger.OutputID, o *ledger.Output) bool {
		return o.Lock().Name() == ledger.AddressED25519Name
	})
}

func (srv *server) getOutputsForAmount(w http.ResponseWriter, r *http.Request) {
	lst, ok := r.URL.Query()["addr"]
	if !ok || len(lst) != 1 {
		writeErr(w, "wrong parameter 'addr' in request 'get_outputs_for_amount'")
		return
	}
	targetAddr, err := ledger.AddressED25519FromSource(lst[0])
	if err != nil {
		writeErr(w, err.Error())
		return
	}

	lst, ok = r.URL.Query()["amount"]
	if !ok || len(lst) != 1 {
		writeErr(w, "wrong parameter 'amount' in request 'get_outputs_for_amount'")
		return
	}
	amount, err := strconv.Atoi(lst[0])
	if err != nil {
		writeErr(w, err.Error())
		return
	}

	resp := &api.OutputList{
		Outputs: make(map[string]string),
	}
	sum := uint64(0)
	err = srv.withLRB(func(rdr multistate.SugaredStateReader) error {
		lrbid := rdr.GetStemOutput().ID.TransactionID()
		resp.LRBID = lrbid.StringHex()
		err1 := rdr.IterateOutputsForAccount(targetAddr, func(oid ledger.OutputID, o *ledger.Output) bool {
			if o.Lock().Name() != ledger.AddressED25519Name {
				return true
			}
			if !ledger.EqualAccountables(targetAddr, o.Lock().(ledger.AddressED25519)) {
				return true
			}
			resp.Outputs[oid.StringHex()] = o.Hex()
			sum += o.Amount()
			return sum < uint64(amount)
		})
		if err1 != nil {
			return err1
		}
		return nil
	})

	if sum < uint64(amount) {
		writeErr(w, fmt.Sprintf("not enough tokens: < than requested %s", util.Th(amount)))
		return
	}

	respBin, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	_, err = w.Write(respBin)
	util.AssertNoError(err)

}

func (srv *server) getChainOutput(w http.ResponseWriter, r *http.Request) {
	setHeader(w)

	lst, ok := r.URL.Query()["chainid"]
	if !ok || len(lst) != 1 {
		writeErr(w, "wrong parameters in request 'get_chain_output'")
		return
	}
	chainID, err := ledger.ChainIDFromHexString(lst[0])
	if err != nil {
		writeErr(w, err.Error())
		return
	}

	resp := &api.ChainOutput{}
	err = srv.withLRB(func(rdr multistate.SugaredStateReader) error {
		o, err1 := rdr.GetChainOutput(&chainID)
		if err1 != nil {
			return err1
		}
		resp.ID = o.ID.StringHex()
		resp.Data = hex.EncodeToString(o.Output.Bytes())
		lrbid := rdr.GetStemOutput().ID.TransactionID()
		resp.LRBID = lrbid.StringHex()
		return nil
	})
	if err != nil {
		writeErr(w, err.Error())
		return
	}

	respBin, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	_, err = w.Write(respBin)
	util.AssertNoError(err)
}

func (srv *server) getChainedOutputs(w http.ResponseWriter, r *http.Request) {
	setHeader(w)

	lst, ok := r.URL.Query()["accountable"]
	if !ok || len(lst) != 1 {
		writeErr(w, "wrong parameter 'accountable' in request 'get_chained_outputs'")
		return
	}
	accountable, err := ledger.AccountableFromSource(lst[0])
	if err != nil {
		writeErr(w, err.Error())
		return
	}

	resp := api.ChainedOutputs{
		Outputs: make(map[string]string),
	}
	var err1 error
	err = srv.withLRB(func(rdr multistate.SugaredStateReader) error {
		lrbid := rdr.GetStemOutput().ID.TransactionID()
		resp.LRBID = lrbid.StringHex()

		err1 = rdr.IterateChainsInAccount(accountable, func(oid ledger.OutputID, o *ledger.Output, _ ledger.ChainID) bool {
			resp.Outputs[oid.StringHex()] = hex.EncodeToString(o.Bytes())
			return true
		})
		if err1 != nil {
			return err1
		}
		return nil
	})
	if err != nil {
		writeErr(w, err.Error())
		return
	}

	respBin, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	_, err = w.Write(respBin)
	util.AssertNoError(err)
}

func (srv *server) getOutput(w http.ResponseWriter, r *http.Request) {
	setHeader(w)

	lst, ok := r.URL.Query()["id"]
	if !ok || len(lst) != 1 {
		writeErr(w, "wrong parameter in request 'get_output'")
		return
	}
	oid, err := ledger.OutputIDFromHexString(lst[0])
	if err != nil {
		writeErr(w, err.Error())
		return
	}

	resp := &api.OutputData{}
	err = srv.withLRB(func(rdr multistate.SugaredStateReader) error {
		oData, found := rdr.GetUTXO(&oid)
		if !found {
			return errors.New(api.ErrGetOutputNotFound)
		}
		resp.OutputData = hex.EncodeToString(oData)
		lrbid := rdr.GetStemOutput().ID.TransactionID()
		resp.LRBID = lrbid.StringHex()
		return nil
	})
	if err != nil {
		writeErr(w, api.ErrGetOutputNotFound)
		return
	}

	respBin, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	_, err = w.Write(respBin)
	util.AssertNoError(err)
}

const (
	maxTxUploadSize            = 64 * (1 << 10)
	defaultTxAppendWaitTimeout = 10 * time.Second
	maxTxAppendWaitTimeout     = 2 * time.Minute
)

func (srv *server) submitTx(w http.ResponseWriter, r *http.Request) {
	setHeader(w)

	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	timeout := defaultTxAppendWaitTimeout
	lst, ok := r.URL.Query()["timeout"]
	if ok {
		wrong := len(lst) != 1
		var timeoutSec int
		var err error
		if !wrong {
			timeoutSec, err = strconv.Atoi(lst[0])
			wrong = err != nil || timeoutSec < 0
		}
		if wrong {
			writeErr(w, "wrong 'timeout' parameter in request 'submit_wait'")
			return
		}
		timeout = time.Duration(timeoutSec) * time.Second
		if timeout > maxTxAppendWaitTimeout {
			timeout = maxTxAppendWaitTimeout
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxTxUploadSize)
	txBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// tx tracing on server parameter
	err = util.CatchPanicOrError(func() error {
		srv.SubmitTxBytesFromAPI(slices.Clip(txBytes))
		return nil
	})
	if err != nil {
		writeErr(w, fmt.Sprintf("submit_tx: %v", err))
		srv.Tracef(TraceTag, "submit transaction: '%v'", err)
		return
	}
	writeOk(w)
}

func (srv *server) getSyncInfo(w http.ResponseWriter, r *http.Request) {
	setHeader(w)

	syncInfo := srv.GetSyncInfo()
	respBin, err := json.MarshalIndent(syncInfo, "", "  ")
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	_, err = w.Write(respBin)
	util.AssertNoError(err)
}

func (srv *server) getPeersInfo(w http.ResponseWriter, r *http.Request) {
	setHeader(w)

	peersInfo := srv.GetPeersInfo()
	respBin, err := json.MarshalIndent(peersInfo, "", "  ")
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	_, err = w.Write(respBin)
	util.AssertNoError(err)
}

func (srv *server) getNodeInfo(w http.ResponseWriter, r *http.Request) {
	setHeader(w)

	nodeInfo := srv.GetNodeInfo()
	respBin, err := json.MarshalIndent(nodeInfo, "", "  ")
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	_, err = w.Write(respBin)
	util.AssertNoError(err)
}

func (srv *server) getMilestoneList(w http.ResponseWriter, r *http.Request) {
	setHeader(w)

	resp := api.KnownLatestMilestones{
		Sequencers: srv.GetKnownLatestMilestonesJSONAble(),
	}
	respBin, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	_, err = w.Write(respBin)
	util.AssertNoError(err)
}

const defaultMaxMainChainDepth = 20

func (srv *server) getMainChain(w http.ResponseWriter, r *http.Request) {
	setHeader(w)

	var err error
	maxDepth := defaultMaxMainChainDepth
	lst, ok := r.URL.Query()["max"]
	if ok || len(lst) == 1 {
		if maxDepth, err = strconv.Atoi(lst[0]); err != nil {
			writeErr(w, "wrong parameter 'max'")
			return
		}
	}
	if maxDepth <= 0 {
		maxDepth = 1
	}
	main, err := multistate.GetMainChain(srv.StateStore(), global.FractionHealthyBranch, maxDepth)
	if err != nil {
		writeErr(w, err.Error())
		return
	}

	resp := api.MainChain{
		Branches: make([]api.BranchData, 0),
	}

	for _, br := range main {
		txid := br.Stem.ID.TransactionID()
		resp.Branches = append(resp.Branches, api.BranchData{
			ID:   txid.StringHex(),
			Data: *br.JSONAble(),
		})
	}

	respBin, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	_, err = w.Write(respBin)
	util.AssertNoError(err)
}

func (srv *server) getAllChains(w http.ResponseWriter, r *http.Request) {
	setHeader(w)

	var lst map[ledger.ChainID]multistate.ChainRecordInfo
	resp := api.Chains{
		Chains: make(map[string]api.OutputDataWithID),
	}

	err := srv.withLRB(func(rdr multistate.SugaredStateReader) error {
		var err1 error
		lst, err1 = rdr.GetAllChains()
		lrbid := rdr.GetStemOutput().ID.TransactionID()
		resp.LRBID = lrbid.StringHex()
		return err1
	})
	if err != nil {
		writeErr(w, err.Error())
		return
	}

	for chainID, ri := range lst {
		resp.Chains[chainID.StringHex()] = api.OutputDataWithID{
			ID:   ri.Output.ID.StringHex(),
			Data: hex.EncodeToString(ri.Output.Data),
		}
	}
	respBin, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	_, err = w.Write(respBin)
	util.AssertNoError(err)
}

func (srv *server) getLatestReliableBranch(w http.ResponseWriter, r *http.Request) {
	setHeader(w)

	bd := srv.GetLatestReliableBranch()
	if bd == nil {
		writeErr(w, "latest reliable branch (LRB) has not been found")
		return
	}

	resp := &api.LatestReliableBranch{
		RootData: *bd.RootRecord.JSONAble(),
		BranchID: bd.Stem.ID.TransactionID(),
	}
	respBin, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	_, err = w.Write(respBin)
	util.AssertNoError(err)
}

func (srv *server) checkTxIDIncludedInLRB(w http.ResponseWriter, r *http.Request) {
	setHeader(w)

	var txid ledger.TransactionID
	var err error

	// mandatory parameter txid
	lst, ok := r.URL.Query()["txid"]
	if !ok || len(lst) != 1 {
		writeErr(w, "txid expected")
		return
	}
	txid, err = ledger.TransactionIDFromHexString(lst[0])
	if err != nil {
		writeErr(w, err.Error())
		return
	}

	maxDepth := 1 // default max depth is 1
	// optional parameter
	lst, ok = r.URL.Query()["max_depth"]
	if ok && len(lst) == 1 {
		maxDepth, err = strconv.Atoi(lst[0])
		if err != nil {
			writeErr(w, err.Error())
			return
		}
		if maxDepth < 0 {
			// wrong value reset to default
			maxDepth = 1
		}
	}

	lrbid, foundAyDepth := srv.CheckTransactionInLRB(txid, maxDepth)
	resp := api.CheckTxIDInLRB{
		TxID:         txid.StringHex(),
		LRBID:        lrbid.StringHex(),
		FoundAtDepth: foundAyDepth,
	}

	respBin, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	_, err = w.Write(respBin)
	util.AssertNoError(err)
}

func writeErr(w http.ResponseWriter, errStr string) {
	respBytes, err := json.Marshal(&api.Error{Error: errStr})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = w.Write(respBytes)
	util.AssertNoError(err)
}

func writeOk(w http.ResponseWriter) {
	respBytes, err := json.Marshal(&api.Error{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = w.Write(respBytes)
	util.AssertNoError(err)
}

func writeNotImplemented(w http.ResponseWriter) {
	writeErr(w, "not implemented")
}

func setHeader(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
}

func (srv *server) withLRB(fun func(rdr multistate.SugaredStateReader) error) error {
	return util.CatchPanicOrError(func() error {
		rdr, err1 := srv.LatestReliableState()
		if err1 != nil {
			return err1
		}
		return fun(rdr)
	})
}

func Run(addr string, env environment) {
	srv := &server{
		Server: &http.Server{
			Addr:         addr,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  10 * time.Second,
		},
		environment: env,
	}
	srv.registerHandlers()
	srv.registerMetrics()

	err := srv.ListenAndServe()
	util.AssertNoError(err)
}

func (srv *server) registerMetrics() {
	srv.metrics.totalRequests = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "proxima_api_totalRequests",
		Help: "total API requests",
	})
	srv.MetricsRegistry().MustRegister(srv.metrics.totalRequests)
}

func (srv *server) addHandler(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	http.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		srv.Tracef(TraceTag, "API request: %s from %s", r.URL.String(), r.RemoteAddr)
		handler(w, r)
		srv.metrics.totalRequests.Inc()
	})
}
