package node_cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/lunfardo314/proxima/api"
	"github.com/lunfardo314/proxima/api/client"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/transaction"
	"github.com/lunfardo314/proxima/ledger/txbuilder"
	"github.com/lunfardo314/proxima/proxi/glb"
	"github.com/lunfardo314/proxima/sequencer/commands"
	"github.com/lunfardo314/proxima/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const getFundsPath = "/"

type (
	faucetServerConfig struct {
		fromChain          bool
		amount             uint64
		port               uint64
		maxRequestsPerHour int
		maxRequestsPerDay  int
	}

	faucetServer struct {
		cfg                faucetServerConfig
		walletData         glb.WalletData
		mutex              sync.Mutex
		accountRequestList map[string][]time.Time
		addressRequestList map[string][]time.Time
	}
)

var _fromChain bool

const (
	minAmount         = 1_000_000
	defaultFaucetPort = 9500
)

func initFaucetServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "faucet",
		Short: `starts a faucet server on the current wallet`,
		Args:  cobra.NoArgs,
		Run:   runFaucetServerCmd,
	}
	return cmd
}

func readFaucetServerConfigIn(sub *viper.Viper) (ret faucetServerConfig) {
	glb.Assertf(sub != nil, "faucet server configuration has not found")
	ret.fromChain = !sub.GetBool("use_wallet")
	ret.port = sub.GetUint64("port")
	if ret.port == 0 {
		ret.port = defaultFaucetPort
	}
	ret.amount = sub.GetUint64("amount")
	glb.Assertf(ret.amount >= minAmount, "amount must be greater than %s", util.Th(minAmount))

	ret.maxRequestsPerHour = sub.GetInt("max_requests_per_hour")
	glb.Assertf(ret.maxRequestsPerHour > 0, "wrong maximum requests per hour")
	ret.maxRequestsPerDay = sub.GetInt("max_requests_per_hour")
	glb.Assertf(ret.maxRequestsPerDay > 0, "wrong maxium requests per day")
	return
}

func (fct *faucetServer) displayFaucetConfig() {
	glb.Infof("faucet server configuration:")
	if fct.cfg.fromChain {
		glb.Infof("     draw funds from sequencer %s", fct.walletData.Sequencer.String())
	} else {
		glb.Infof("     draw funds from wallet address %s", fct.walletData.Account.String())
	}
	glb.Infof("     amount:          %d", fct.cfg.amount)
	glb.Infof("     port:            %d", fct.cfg.port)
	glb.Infof("     wallet address:  %s", fct.walletData.Account.String())
	glb.Infof("     maximum number of requests per hour: %d, per day: %d", fct.cfg.maxRequestsPerHour, fct.cfg.maxRequestsPerDay)
	if fct.cfg.fromChain {
		glb.Infof("     funds will be drawn from sequencer %s", fct.walletData.Sequencer.String())
	} else {
		glb.Infof("     funds will be drawn from wallet address %s", fct.walletData.Account.String())
	}
}

func runFaucetServerCmd(_ *cobra.Command, _ []string) {
	glb.InitLedgerFromNode()
	walletData := glb.GetWalletData()
	glb.Assertf(walletData.Sequencer != nil, "can't get own sequencer ID")
	glb.Assertf(glb.GetTagAlongFee() > 0, "tag-along amount not specified")
	glb.Assertf(glb.GetTagAlongSequencerID() != nil, "tag-along sequencer not specified")

	cfg := readFaucetServerConfigIn(viper.Sub("faucet"))

	fct := &faucetServer{
		cfg:                cfg,
		walletData:         walletData,
		accountRequestList: make(map[string][]time.Time),
		addressRequestList: make(map[string][]time.Time),
	}

	fct.displayFaucetConfig()

	clnt := glb.GetClient()
	if cfg.fromChain {
		o, _, _, err := clnt.GetChainOutput(*glb.GetOwnSequencerID())
		glb.AssertNoError(err)
		glb.Assertf(o.Output.Amount() > ledger.L().ID.MinimumAmountOnSequencer+cfg.amount,
			"not enough balance on own sequencer %s", fct.walletData.Sequencer.String())
	} else {
		_, _, _, err := clnt.GetOutputsForAmount(walletData.Account, cfg.amount+glb.GetTagAlongFee())
		glb.AssertNoError(err)
	}
	fct.run()
}

func (fct *faucetServer) handler(w http.ResponseWriter, r *http.Request) {
	targetStr, ok := r.URL.Query()["addr"]
	if !ok || len(targetStr) != 1 {
		writeResponse(w, "wrong parameter 'addr' in request 'get_funds'")
		return
	}

	if !fct.checkAndUpdateRequestTime(targetStr[0], r.RemoteAddr) {
		glb.Infof("funds refused to send to %s (remote = %s)", targetStr[0], r.RemoteAddr)
		writeResponse(w, fmt.Sprintf("maximum %d requests per hour and %d per day are allowed", fct.cfg.maxRequestsPerHour, fct.cfg.maxRequestsPerDay))
		return
	}

	targetLock, err := ledger.AccountableFromSource(targetStr[0])
	if err != nil {
		glb.Infof("error from AccountableFromSource: %s", err.Error())
		writeResponse(w, err.Error())
		return
	}
	var txid *ledger.TransactionID
	var fromStr string
	if fct.cfg.fromChain {
		fromStr = "sequencer " + fct.walletData.Sequencer.StringShort()
		txid, err = fct.redrawFromChain(targetLock)
	} else {
		fromStr = "wallet address " + fct.walletData.Account.String()
		txid, err = fct.redrawFromAccount(targetLock)
	}

	if err == nil {
		glb.Infof("requested faucet transfer of %s tokens to %s from %s (remote = %s)",
			util.Th(fct.cfg.amount), targetLock.String(), fromStr, r.RemoteAddr)
		glb.Infof("             transaction %s (hex = %s)", txid.String(), txid.StringHex())
		writeResponse(w, "")
	} else {
		glb.Infof("failed faucet transfer of %s tokens to %s from %s (remote = %s): err = %v",
			util.Th(fct.cfg.amount), targetLock.String(), fromStr, r.RemoteAddr, err)
		writeResponse(w, err.Error())
	}

	logRequest(targetStr[0], r.RemoteAddr, fct.cfg.amount, err)
}

func (fct *faucetServer) redrawFromChain(targetLock ledger.Accountable) (*ledger.TransactionID, error) {
	clnt := glb.GetClient()
	o, _, _, err := clnt.GetChainOutput(*glb.GetOwnSequencerID())
	if err != nil {
		return nil, err
	}
	if o.Output.Amount() < ledger.L().ID.MinimumAmountOnSequencer+fct.cfg.amount {
		return nil, fmt.Errorf("not enough tokens on the sequencer %s", glb.GetOwnSequencerID().String())
	}
	walletOutputs, _, _, err := clnt.GetOutputsForAmount(fct.walletData.Account, glb.GetTagAlongFee())
	if err != nil {
		return nil, err
	}
	withdrawCmd, err := commands.MakeSequencerWithdrawCommand(fct.cfg.amount, targetLock.AsLock())
	if err != nil {
		return nil, err
	}
	// sending command to sequencer
	transferData := txbuilder.NewTransferData(fct.walletData.PrivateKey, fct.walletData.Account, ledger.TimeNow()).
		WithChainOutput(o).
		WithAmount(glb.GetTagAlongFee()).
		WithTargetLock(fct.walletData.Sequencer.AsChainLock()).
		MustWithInputs(walletOutputs...).
		WithSender().
		WithConstraint(withdrawCmd)

	txBytes, err := txbuilder.MakeChainTransferTransaction(transferData)
	if err != nil {
		return nil, err
	}
	tx, err := transaction.FromBytes(txBytes, transaction.MainTxValidationOptions...)
	if err != nil {
		return nil, err
	}
	err = clnt.SubmitTransaction(txBytes)
	if err != nil {
		return nil, err
	}
	return util.Ref(tx.ID()), nil
}

func (fct *faucetServer) redrawFromAccount(targetLock ledger.Accountable) (*ledger.TransactionID, error) {
	txCtx, err := glb.GetClient().TransferFromED25519Wallet(client.TransferFromED25519WalletParams{
		WalletPrivateKey: fct.walletData.PrivateKey,
		TagAlongSeqID:    glb.GetTagAlongSequencerID(),
		TagAlongFee:      glb.GetTagAlongFee(),
		Amount:           fct.cfg.amount,
		Target:           targetLock.AsLock(),
	})

	if err != nil {
		return nil, err
	}
	return txCtx.TransactionID(), nil
}

func _trimToLastDay(lst []time.Time) ([]time.Time, int) {
	ret := util.PurgeSlice(lst, func(when time.Time) bool {
		return time.Since(when) <= 24*time.Hour
	})
	lastHour := 0
	for _, when := range ret {
		if time.Since(when) <= time.Hour {
			lastHour++
		}
	}
	return ret, lastHour
}

func (fct *faucetServer) checkAndUpdateRequestTime(account string, addr string) bool {
	fct.mutex.Lock()
	defer fct.mutex.Unlock()

	var lastHour int

	lst, ok := fct.accountRequestList[account]
	if ok {
		lst, lastHour = _trimToLastDay(lst)
		if len(lst) >= fct.cfg.maxRequestsPerDay || lastHour >= fct.cfg.maxRequestsPerHour {
			return false
		}
		lst = append(lst, time.Now())
	} else {
		lst = []time.Time{time.Now()}
	}
	fct.accountRequestList[account] = lst

	lst, ok = fct.addressRequestList[addr]
	if ok {
		lst, lastHour = _trimToLastDay(lst)
		if len(lst) >= fct.cfg.maxRequestsPerDay || lastHour >= fct.cfg.maxRequestsPerHour {
			return false
		}
		lst = append(lst, time.Now())
	} else {
		lst = []time.Time{time.Now()}
	}
	fct.addressRequestList[addr] = lst
	return true
}

const faucetLogName = "faucet_requests.log"

func logRequest(account string, ipAddress string, funds uint64, err error) {
	// Open the log file in append mode, creating it if it doesn't exist
	file, err := os.OpenFile(faucetLogName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	glb.AssertNoError(err)
	defer file.Close()

	// Create a logger
	logger := log.New(file, "", log.LstdFlags)

	// Log the request
	if err == nil {
		logger.Printf("time: %s, to: %s, funds: %d, IP: %s, \n", time.Now().Format(time.RFC3339), account, funds, ipAddress)
	} else {
		logger.Printf("time: %s, to: %s, funds: %d, IP: %s, , err: %v\n", time.Now().Format(time.RFC3339), account, funds, ipAddress, err)
	}
}

func writeResponse(w http.ResponseWriter, respStr string) {
	var respBytes []byte
	var err error
	if len(respStr) > 0 {
		respBytes, err = json.Marshal(&api.Error{Error: respStr})
	} else {
		respBytes, err = json.Marshal(&api.Error{})
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = w.Write(respBytes)
	util.AssertNoError(err)
}

func (fct *faucetServer) run() {
	http.HandleFunc(getFundsPath, fct.handler) // Route for the handler function
	sport := fmt.Sprintf(":%d", fct.cfg.port)
	glb.Infof("running proxi faucet server on %s. Press Ctrl-C to stop..", sport)
	glb.AssertNoError(http.ListenAndServe(sport, nil))
}
