package transaction

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/lunfardo314/easyfl/easyfl_util"
	"github.com/lunfardo314/easyfl/lazybytes"
	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/ledger/multistate"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/lines"
	"github.com/lunfardo314/proxima/util/set"
	"github.com/lunfardo314/unitrie/common"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/ed25519"
)

// Transaction provides access to the tree of transferable transaction
type (
	Transaction struct {
		tree                     *lazybytes.Tree
		txid                     base.TransactionID
		sequencerMilestoneFlag   bool
		sender                   ledger.AddressED25519
		timestamp                base.LedgerTime
		totalAmount              uint64                    // persisted in tx
		totalInflation           uint64                    // calculated
		sequencerTransactionData *SequencerTransactionData // if != nil it is sequencer milestone transaction
	}

	TxValidationOption func(tx *Transaction) error

	// SequencerTransactionData represents sequencer and stem data on the transaction
	SequencerTransactionData struct {
		SequencerOutputData  *ledger.SequencerOutputData
		StemOutputData       *ledger.StemLock // nil if does not contain stem output
		SequencerID          base.ChainID     // adjusted for chain origin
		SequencerOutputIndex byte
		StemOutputIndex      byte // 0xff if not a branch transaction
	}
)

// MainTxValidationOptions is all except Base, time bounds and input context validation. Fastest first
var MainTxValidationOptions = []TxValidationOption{
	ParseSequencerData,
	CheckExplicitBaseline,
	CheckSizeOfInputCommitment,
	CheckSender,
	ScanInputs,
	ScanEndorsements,
	ScanOutputs,
}

func FromBytes(txBytes []byte, opt ...TxValidationOption) (*Transaction, error) {
	ret, err := transactionFromBytes(txBytes, BaseValidation)
	if err != nil {
		return nil, fmt.Errorf("transaction.FromBytes: basic parse failed: '%v'", err)
	}
	if err = ret.Validate(opt...); err != nil {
		return ret, fmt.Errorf("FromBytes: validation failed, txid = %s: '%v'", ret.IDShortString(), err)
	}
	return ret, nil
}

func FromBytesMainChecksWithOpt(txBytes []byte, additional ...TxValidationOption) (*Transaction, error) {
	tx, err := FromBytes(txBytes, MainTxValidationOptions...)
	if err != nil {
		return nil, err
	}
	if err = tx.Validate(additional...); err != nil {
		return nil, err
	}
	return tx, nil
}

func transactionFromBytes(txBytes []byte, opts ...TxValidationOption) (*Transaction, error) {
	ret := &Transaction{
		tree: lazybytes.TreeFromBytesReadOnly(txBytes),
	}
	if err := ret.Validate(opts...); err != nil {
		return nil, err
	}
	return ret, nil
}

func IDAndTimestampFromTransactionBytes(txBytes []byte) (base.TransactionID, base.LedgerTime, error) {
	tx, err := FromBytes(txBytes)
	if err != nil {
		return base.TransactionID{}, base.LedgerTime{}, err
	}
	return tx.ID(), tx.Timestamp(), nil
}

func IDFromTransactionBytes(txBytes []byte) (base.TransactionID, error) {
	tx, err := FromBytes(txBytes)
	if err != nil {
		return base.TransactionID{}, err
	}
	return tx.ID(), nil
}

func (tx *Transaction) Validate(opt ...TxValidationOption) error {
	return util.CatchPanicOrError(func() error {
		for _, fun := range opt {
			if err := fun(tx); err != nil {
				return err
			}
		}
		return nil
	})
}

func (tx *Transaction) SignatureBytes() []byte {
	return tx.tree.BytesAtPath(Path(ledger.TxSignature))
}

// BaseValidation is a checking of being able to extract id. If not, bytes are not identifiable as a transaction
func BaseValidation(tx *Transaction) error {
	var tsBin []byte
	tsBin = tx.tree.BytesAtPath(Path(ledger.TxTimestamp))
	var err error
	outputIndexData := tx.tree.BytesAtPath(Path(ledger.TxSequencerAndStemOutputIndices))

	// check sequencer output index data. Enforce exactly 2 bytes
	if len(outputIndexData) != 2 {
		return fmt.Errorf("wrong sequencer output index data, must be 2 bytes")
	}
	// determine the sequencer flag
	tx.sequencerMilestoneFlag = outputIndexData[0] != 0xff
	// parse and validate timestamp
	if tx.timestamp, err = base.TimeFromBytes(tsBin); err != nil {
		return err
	}
	// validate stem output index. Must be 0xff if it is not a branch transaction
	if tx.sequencerMilestoneFlag && tx.timestamp.Tick == 0 && outputIndexData[1] == 0xff {
		return fmt.Errorf("wrong stem output index")
	}
	// parse the total amount as trimmed-prefix uint68. Validity of the sum is not checked here
	tx.totalAmount, err = easyfl_util.Uint64FromBytes(tx.tree.BytesAtPath(Path(ledger.TxTotalProducedAmount)))
	if err != nil {
		return fmt.Errorf("wrong total amount in transaction: %v", err)
	}
	// check if the number of outputs is valid. Strictly speaking, not necessary, because max 256 outputs are enforced before
	numProducedOutputs := tx.tree.NumElements(Path(ledger.TxOutputs))
	if numProducedOutputs <= 0 || numProducedOutputs > 256 {
		return fmt.Errorf("number of outputs must be positive and not exceed 256")
	}
	// parsing short txid. For full tx ID it will be concatenated with the
	txidShort := base.TransactionIDShortFromTxBytes(tx.tree.Bytes(), byte(numProducedOutputs-1))
	tx.txid = base.NewTransactionID(tx.timestamp, txidShort, tx.sequencerMilestoneFlag)
	return nil
}

func CheckTimestampLowerBound(lowerBound time.Time) TxValidationOption {
	return func(tx *Transaction) error {
		if ledger.ClockTime(tx.timestamp).Before(lowerBound) {
			return fmt.Errorf("transaction is too old")
		}
		return nil
	}
}

func CheckTimestampUpperBound(upperBound time.Time) TxValidationOption {
	return func(tx *Transaction) error {
		ts := ledger.ClockTime(tx.timestamp)
		if ts.After(upperBound) {
			return fmt.Errorf("transaction is %d msec too far in the future", int64(ts.Sub(upperBound))/int64(time.Millisecond))
		}
		return nil
	}
}

// ParseSequencerData validates and parses sequencer data if relevant. Data is cached for frequent extraction
func ParseSequencerData(tx *Transaction) error {
	if !tx.sequencerMilestoneFlag {
		return nil
	}
	outputIndexData := tx.tree.BytesAtPath(Path(ledger.TxSequencerAndStemOutputIndices))
	util.Assertf(len(outputIndexData) == 2, "len(outputIndexData) == 2")
	sequencerOutputIndex, stemOutputIndex := outputIndexData[0], outputIndexData[1]

	// check sequencer output
	if int(sequencerOutputIndex) >= tx.NumProducedOutputs() {
		return fmt.Errorf("wrong sequencer output index")
	}
	out, err := tx.ProducedOutputWithIDAt(sequencerOutputIndex)
	if err != nil {
		return fmt.Errorf("ParseSequencerData: '%v' at produced output %d", err, sequencerOutputIndex)
	}
	seqOutputData, valid := out.Output.SequencerOutputData()
	if !valid {
		return fmt.Errorf("ParseSequencerData: invalid sequencer output data")
	}

	var sequencerID base.ChainID
	if seqOutputData.ChainConstraint.IsOrigin() {
		sequencerID = base.MakeOriginChainID(out.ID)
	} else {
		sequencerID = seqOutputData.ChainConstraint.ID
	}

	// it is a sequencer milestone transaction
	tx.sequencerTransactionData = &SequencerTransactionData{
		SequencerOutputData: seqOutputData,
		SequencerID:         sequencerID,
		StemOutputIndex:     stemOutputIndex,
		StemOutputData:      nil,
	}

	// ---  check stem output data
	if tx.timestamp.Tick != 0 {
		// not a branch transaction
		return nil
	}
	if stemOutputIndex == sequencerOutputIndex || int(stemOutputIndex) >= tx.NumProducedOutputs() {
		return fmt.Errorf("ParseSequencerData: wrong stem output index")
	}
	outStem, err := tx.ProducedOutputWithIDAt(stemOutputIndex)
	if err != nil {
		return fmt.Errorf("ParseSequencerData stem: %v", err)
	}
	lock := outStem.Output.Lock()
	if lock.Name() != ledger.StemLockName {
		return fmt.Errorf("ParseSequencerData: not a stem lock")
	}
	tx.sequencerTransactionData.StemOutputData = lock.(*ledger.StemLock)
	return nil
}

// CheckSender parses and checks signature, sets the sender field
func CheckSender(tx *Transaction) error {
	// mandatory sender signature
	sigData := tx.SignatureBytes()
	senderPubKey := ed25519.PublicKey(sigData[64:])
	tx.sender = ledger.AddressED25519FromPublicKey(senderPubKey)
	if !ed25519.Verify(senderPubKey, tx.EssenceBytes(), sigData[0:64]) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

// ScanInputs validation option scans all inputs, enforces existence of mandatory constrains,
// computes total of outputs and total inflation
func ScanInputs(tx *Transaction) error {
	numInputs := tx.tree.NumElements(Path(ledger.TxInputIDs))
	var err error
	var oid base.OutputID

	// enforce non-empty input set
	if numInputs <= 0 {
		return fmt.Errorf("number of inputs can't be 0")
	}
	// enforce exactly one unlock data for one input
	if numInputs != tx.tree.NumElements(Path(ledger.TxUnlockData)) {
		return fmt.Errorf("number of unlock datas must be equal to the number of inputs")
	}

	ts := tx.Timestamp()
	isSequencer := tx.IsSequencerTransaction()
	path := []byte{ledger.TxInputIDs, 0}
	inps := set.New[base.OutputID]()

	for i := 0; i < numInputs; i++ {
		path[1] = byte(i)
		// parse output ID
		oid, err = base.OutputIDFromBytes(tx.tree.BytesAtPath(path))
		if err != nil {
			return fmt.Errorf("parsing input #%d: '%v'", i, err)
		}
		// check uniqueness
		if inps.Contains(oid) {
			return fmt.Errorf("repeating input #%d: %s", i, oid.StringShort())
		}
		inps.Insert(oid)
		// check time pace constraint
		if isSequencer {
			if !ledger.ValidSequencerPace(oid.Timestamp(), ts) {
				return fmt.Errorf("input #%d violates sequencer time pace constraint: %s", i, oid.StringShort())
			}
		} else {
			if !ledger.ValidTransactionPace(oid.Timestamp(), ts) {
				return fmt.Errorf("input #%d violates transaction time pace constraint: %s", i, oid.StringShort())
			}
		}
	}
	return nil
}

// ScanEndorsements parses and checks validity of each endorsement
func ScanEndorsements(tx *Transaction) error {
	numEndorsements := tx.tree.NumElements(Path(ledger.TxEndorsements))
	if numEndorsements == 0 {
		return nil
	}
	// check max number of endorsements
	if numEndorsements > int(ledger.L().ID.MaxNumberOfEndorsements) {
		return fmt.Errorf("number of endorsements should not exceed %d", ledger.L().ID.MaxNumberOfEndorsements)
	}
	// enforce only sequencer transaction can endorse
	if !tx.IsSequencerTransaction() {
		return fmt.Errorf("non-sequencer transaction cannot contain endorsements")
	}

	var err error
	var endorsementID base.TransactionID

	unique := set.New[base.TransactionID]()
	txTs := tx.Timestamp()

	path := []byte{ledger.TxEndorsements, 0}
	for i := 0; i < numEndorsements; i++ {
		path[1] = byte(i)
		// parse transaction ID
		endorsementID, err = base.TransactionIDFromBytes(tx.tree.BytesAtPath(path))
		if err != nil {
			return fmt.Errorf("parsing endorsement #%d: '%v'", i, err)
		}
		// check uniqueness
		if unique.Contains(endorsementID) {
			return fmt.Errorf("repeating endorsement #%d: %s", i, endorsementID.StringShort())
		}
		unique.Insert(endorsementID)
		// check cross-slot endorsements
		if txTs.Slot != endorsementID.Slot() {
			return fmt.Errorf("cross-slot endorsements are not allowed:  %s ->  %s", tx.IDShortString(), endorsementID.StringShort())
		}
		// check time pace
		if !ledger.ValidSequencerPace(endorsementID.Timestamp(), txTs) {
			return fmt.Errorf("endorsement #%d violates sequencer time pace constraint: %s -> %s", i, txTs.String(), endorsementID.StringShort())
		}
	}
	return nil
}

// ScanOutputs validation option scans all outputs, enforces existence of mandatory constrains,
// computes total of outputs and total inflation
func ScanOutputs(tx *Transaction) error {
	numOutputs := tx.tree.NumElements(Path(ledger.TxOutputs))

	var err error
	var totalAmount uint64
	var amount ledger.Amount

	var o *ledger.Output
	path := []byte{ledger.TxOutputs, 0}
	for i := 0; i < numOutputs; i++ {
		path[1] = byte(i)
		o, amount, _, err = ledger.OutputFromBytesMain(tx.tree.BytesAtPath(path))
		if err != nil {
			return fmt.Errorf("scanning output #%d: '%v'", i, err)
		}
		if uint64(amount) > math.MaxUint64-totalAmount {
			return fmt.Errorf("scanning output #%d: 'arithmetic overflow while calculating total of outputs'", i)
		}
		totalAmount += uint64(amount)
		tx.totalInflation += o.Inflation()
	}
	if tx.totalAmount != totalAmount {
		return fmt.Errorf("wrong total produced amount")
	}
	return nil
}

func CheckSizeOfInputCommitment(tx *Transaction) error {
	data := tx.tree.BytesAtPath(Path(ledger.TxInputCommitment))
	if len(data) != 32 {
		return fmt.Errorf("input commitment must be 32-bytes long")
	}
	return nil
}

func CheckExplicitBaseline(tx *Transaction) error {
	data := tx.tree.BytesAtPath(Path(ledger.TxExplicitBaseline))
	if len(data) == 0 {
		return nil
	}
	if !tx.IsSequencerTransaction() {
		return fmt.Errorf("checking explicit baseline: can't only be set on a sequencer transaction")
	}
	txid, err := base.TransactionIDFromBytes(data)
	if err != nil {
		return fmt.Errorf("checking explicit baseline: %v", err)
	}
	if !txid.IsBranchTransaction() {
		return fmt.Errorf("explicit baseline must be a branch transaction ID, got %s", txid.String())
	}
	if !ledger.ValidSequencerPace(txid.Timestamp(), tx.timestamp) {
		return fmt.Errorf("explicit baseline violates sequencer pace constraint: %s", txid.String())
	}
	return nil
}

func ValidateOptionWithFullContext(inputLoaderByIndex func(i byte) (*ledger.Output, error)) TxValidationOption {
	return func(tx *Transaction) error {
		var ctx *TxContext
		var err error
		if __printLogOnFail.Load() {
			ctx, err = TxContextFromTransaction(tx, inputLoaderByIndex, TraceOptionAll)
		} else {
			ctx, err = TxContextFromTransaction(tx, inputLoaderByIndex)
		}
		if err != nil {
			return err
		}
		return ctx.Validate()
	}
}

func (tx *Transaction) ID() base.TransactionID {
	return tx.txid
}

func (tx *Transaction) IDString() string {
	return base.TransactionIDString(tx.timestamp, tx.txid.ShortID(), tx.sequencerMilestoneFlag)
}

func (tx *Transaction) IDShortString() string {
	return base.TransactionIDStringShort(tx.timestamp, tx.txid.ShortID(), tx.sequencerMilestoneFlag)
}

func (tx *Transaction) IDVeryShortString() string {
	return base.TransactionIDStringVeryShort(tx.timestamp, tx.txid.ShortID(), tx.sequencerMilestoneFlag)
}

func (tx *Transaction) IDStringHex() string {
	id := tx.ID()
	return id.StringHex()
}

func (tx *Transaction) Slot() base.Slot {
	return tx.timestamp.Slot
}

func (tx *Transaction) Hash() base.TransactionIDShort {
	return tx.txid.ShortID()
}

// SequencerTransactionData returns nil it is not a sequencer milestone
func (tx *Transaction) SequencerTransactionData() *SequencerTransactionData {
	return tx.sequencerTransactionData
}

func (tx *Transaction) ExplicitBaseline() (base.TransactionID, bool) {
	data := tx.tree.BytesAtPath(Path(ledger.TxExplicitBaseline))
	if len(data) == 0 {
		return base.TransactionID{}, false
	}
	ret, err := base.TransactionIDFromBytes(data)
	util.AssertNoError(err)
	return ret, true
}

func (tx *Transaction) IsSequencerTransaction() bool {
	return tx.sequencerMilestoneFlag
}

func (tx *Transaction) IsBranchTransaction() bool {
	return tx.sequencerMilestoneFlag && tx.timestamp.Tick == 0
}

func (tx *Transaction) StemOutputData() *ledger.StemLock {
	if tx.sequencerTransactionData != nil {
		return tx.sequencerTransactionData.StemOutputData
	}
	return nil
}

func (m *SequencerTransactionData) Short() string {
	return fmt.Sprintf("SEQ(%s)", m.SequencerID.StringVeryShort())
}

func (tx *Transaction) SequencerOutput() *ledger.OutputWithID {
	util.Assertf(tx.IsSequencerTransaction(), "tx.IsSequencerTransaction()")
	return tx.MustProducedOutputWithIDAt(tx.SequencerTransactionData().SequencerOutputIndex)
}

func (tx *Transaction) StemOutput() *ledger.OutputWithID {
	util.Assertf(tx.IsBranchTransaction(), "tx.IsBranchTransaction()")
	return tx.MustProducedOutputWithIDAt(tx.SequencerTransactionData().StemOutputIndex)
}

func (tx *Transaction) SenderAddress() ledger.AddressED25519 {
	return tx.sender
}

func (tx *Transaction) Timestamp() base.LedgerTime {
	return tx.timestamp
}

func (tx *Transaction) TimestampTime() time.Time {
	return ledger.ClockTime(tx.timestamp)
}

func (tx *Transaction) TotalAmount() uint64 {
	return tx.totalAmount
}

func EssenceBytesFromTransactionDataTree(txTree *lazybytes.Tree) []byte {
	return common.Concat(
		txTree.BytesAtPath([]byte{ledger.TxInputIDs}),
		txTree.BytesAtPath([]byte{ledger.TxOutputs}),
		txTree.BytesAtPath([]byte{ledger.TxTimestamp}),
		txTree.BytesAtPath([]byte{ledger.TxSequencerAndStemOutputIndices}),
		txTree.BytesAtPath([]byte{ledger.TxInputCommitment}),
		txTree.BytesAtPath([]byte{ledger.TxEndorsements}),
	)
}

func (tx *Transaction) Bytes() []byte {
	return tx.tree.Bytes()
}

func (tx *Transaction) EssenceBytes() []byte {
	return EssenceBytesFromTransactionDataTree(tx.tree)
}

func (tx *Transaction) NumProducedOutputs() int {
	return tx.tree.NumElements(Path(ledger.TxOutputs))
}

func (tx *Transaction) NumInputs() int {
	return tx.tree.NumElements(Path(ledger.TxInputIDs))
}

func (tx *Transaction) NumEndorsements() int {
	return tx.tree.NumElements(Path(ledger.TxEndorsements))
}

func (tx *Transaction) MustOutputDataAt(idx byte) []byte {
	return tx.tree.BytesAtPath(common.Concat(ledger.TxOutputs, idx))
}

func (tx *Transaction) MustProducedOutputAt(idx byte) *ledger.Output {
	ret, err := ledger.OutputFromBytesReadOnly(tx.MustOutputDataAt(idx))
	util.AssertNoError(err)
	return ret
}

func (tx *Transaction) ProducedOutputAt(idx byte) (*ledger.Output, error) {
	if int(idx) >= tx.NumProducedOutputs() {
		return nil, fmt.Errorf("wrong output index")
	}
	out, err := ledger.OutputFromBytesReadOnly(tx.MustOutputDataAt(idx))
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (tx *Transaction) ProducedOutputWithIDAt(idx byte) (*ledger.OutputWithID, error) {
	ret, err := tx.ProducedOutputAt(idx)
	if err != nil {
		return nil, err
	}
	return &ledger.OutputWithID{
		ID:     tx.OutputID(idx),
		Output: ret,
	}, nil
}

func (tx *Transaction) MustProducedOutputWithIDAt(idx byte) *ledger.OutputWithID {
	ret, err := tx.ProducedOutputWithIDAt(idx)
	util.AssertNoError(err)
	return ret
}

func (tx *Transaction) ProducedOutputs() []*ledger.OutputWithID {
	ret := make([]*ledger.OutputWithID, tx.NumProducedOutputs())
	for i := range ret {
		ret[i] = tx.MustProducedOutputWithIDAt(byte(i))
	}
	return ret
}

func (tx *Transaction) InputAt(idx byte) (ret base.OutputID, err error) {
	if int(idx) >= tx.NumInputs() {
		return [33]byte{}, fmt.Errorf("InputAt: wrong input index")
	}
	data := tx.tree.BytesAtPath(common.Concat(ledger.TxInputIDs, idx))
	ret, err = base.OutputIDFromBytes(data)
	return
}

func (tx *Transaction) MustInputAt(idx byte) base.OutputID {
	ret, err := tx.InputAt(idx)
	util.AssertNoError(err)
	return ret
}

func (tx *Transaction) MustOutputIndexOfTheInput(inputIdx byte) byte {
	return base.MustOutputIndexFromIDBytes(tx.tree.BytesAtPath(common.Concat(ledger.TxInputIDs, inputIdx)))
}

func (tx *Transaction) InputAtString(idx byte) string {
	ret, err := tx.InputAt(idx)
	if err != nil {
		return err.Error()
	}
	return ret.String()
}

func (tx *Transaction) InputAtShort(idx byte) string {
	ret, err := tx.InputAt(idx)
	if err != nil {
		return err.Error()
	}
	return ret.StringShort()
}

func (tx *Transaction) Inputs() []base.OutputID {
	ret := make([]base.OutputID, tx.NumInputs())
	for i := range ret {
		ret[i] = tx.MustInputAt(byte(i))
	}
	return ret
}

func (tx *Transaction) MustUnlockDataAt(idx byte) []byte {
	return tx.tree.BytesAtPath(common.Concat(ledger.TxUnlockData, idx))
}

func (tx *Transaction) ConsumedOutputAt(idx byte, fetchOutput func(id *base.OutputID) ([]byte, bool)) (*ledger.OutputDataWithID, error) {
	oid, err := tx.InputAt(idx)
	if err != nil {
		return nil, err
	}
	ret, ok := fetchOutput(&oid)
	if !ok {
		return nil, fmt.Errorf("can't fetch output %s", oid.StringShort())
	}
	return &ledger.OutputDataWithID{
		ID:   oid,
		Data: ret,
	}, nil
}

func (tx *Transaction) EndorsementAt(idx byte) base.TransactionID {
	data := tx.tree.BytesAtPath(common.Concat(ledger.TxEndorsements, idx))
	ret, err := base.TransactionIDFromBytes(data)
	util.AssertNoError(err)
	return ret
}

// HashInputsAndEndorsements blake2b of concatenated input IDs and endorsements
// independent on any other tz data but inputs
func (tx *Transaction) HashInputsAndEndorsements() [32]byte {
	var buf bytes.Buffer

	buf.Write(tx.tree.BytesAtPath(Path(ledger.TxInputIDs)))
	buf.Write(tx.tree.BytesAtPath(Path(ledger.TxEndorsements)))

	return blake2b.Sum256(buf.Bytes())
}

func (tx *Transaction) ForEachInput(fun func(i byte, oid base.OutputID) bool) {
	tx.tree.ForEach(func(i byte, data []byte) bool {
		oid, err := base.OutputIDFromBytes(data)
		util.Assertf(err == nil, "ForEachInput @ %d: %v", i, err)
		return fun(i, oid)
	}, Path(ledger.TxInputIDs))
}

func (tx *Transaction) ForEachEndorsement(fun func(idx byte, txid base.TransactionID) bool) {
	tx.tree.ForEach(func(i byte, data []byte) bool {
		txid, err := base.TransactionIDFromBytes(data)
		util.Assertf(err == nil, "ForEachEndorsement @ %d: %v", i, err)
		return fun(i, txid)
	}, Path(ledger.TxEndorsements))
}

func (tx *Transaction) ForEachOutputData(fun func(idx byte, oData []byte) bool) {
	tx.tree.ForEach(func(i byte, data []byte) bool {
		return fun(i, data)
	}, Path(ledger.TxOutputs))
}

// ForEachProducedOutput traverses all produced outputs
// Inside callback function the correct outputID must be obtained with OutputID(idx byte) ledger.OutputID
// because stem output ID has a special form
func (tx *Transaction) ForEachProducedOutput(fun func(idx byte, o *ledger.Output, oid base.OutputID) bool) {
	tx.ForEachOutputData(func(idx byte, oData []byte) bool {
		o, _ := ledger.OutputFromBytesReadOnly(oData)
		oid := tx.OutputID(idx)
		if !fun(idx, o, oid) {
			return false
		}
		return true
	})
}

func (tx *Transaction) PredecessorTransactionIDs() set.Set[base.TransactionID] {
	ret := set.New[base.TransactionID]()
	tx.ForEachInput(func(_ byte, oid base.OutputID) bool {
		ret.Insert(oid.TransactionID())
		return true
	})
	tx.ForEachEndorsement(func(_ byte, txid base.TransactionID) bool {
		ret.Insert(txid)
		return true
	})
	return ret
}

// SequencerAndStemOutputIndices return seq output index and stem output index
func (tx *Transaction) SequencerAndStemOutputIndices() (byte, byte) {
	ret := tx.tree.BytesAtPath([]byte{ledger.TxSequencerAndStemOutputIndices})
	util.Assertf(len(ret) == 2, "len(ret)==2")
	return ret[0], ret[1]
}

func (tx *Transaction) OutputID(idx byte) base.OutputID {
	return base.MustNewOutputID(tx.ID(), idx)
}

func (tx *Transaction) InflationAmount() uint64 {
	return tx.totalInflation
}

func OutputWithIDFromTransactionBytes(txBytes []byte, idx byte) (*ledger.OutputWithID, error) {
	tx, err := FromBytes(txBytes)
	if err != nil {
		return nil, err
	}
	if int(idx) >= tx.NumProducedOutputs() {
		return nil, fmt.Errorf("wrong output index")
	}
	return tx.ProducedOutputWithIDAt(idx)
}

func OutputsWithIDFromTransactionBytes(txBytes []byte) ([]*ledger.OutputWithID, error) {
	tx, err := FromBytes(txBytes)
	if err != nil {
		return nil, err
	}

	ret := make([]*ledger.OutputWithID, tx.NumProducedOutputs())
	for idx := 0; idx < tx.NumProducedOutputs(); idx++ {
		ret[idx], err = tx.ProducedOutputWithIDAt(byte(idx))
		if err != nil {
			return nil, err
		}
	}
	return ret, nil
}

func (tx *Transaction) ToString(fetchOutput func(oid base.OutputID) ([]byte, bool)) string {
	ctx, err := TxContextFromTransaction(tx, func(i byte) (*ledger.Output, error) {
		oid, err1 := tx.InputAt(i)
		if err1 != nil {
			return nil, err1
		}
		oData, ok := fetchOutput(oid)
		if !ok {
			return nil, fmt.Errorf("output %s has not been found", oid.StringShort())
		}
		o, err1 := ledger.OutputFromBytesReadOnly(oData)
		if err1 != nil {
			return nil, err1
		}
		return o, nil
	})
	if err != nil {
		return err.Error()
	}
	return ctx.String()
}

func (tx *Transaction) ToStringWithInputLoaderByIndex(fetchOutput func(i byte) (*ledger.Output, error)) string {
	ctx, err := TxContextFromTransaction(tx, fetchOutput)
	if err != nil {
		return err.Error()
	}
	return ctx.String()
}

func (tx *Transaction) InputLoaderByIndex(fetchOutput func(oid base.OutputID) ([]byte, bool)) func(byte) (*ledger.Output, error) {
	return func(idx byte) (*ledger.Output, error) {
		inp := tx.MustInputAt(idx)
		odata, ok := fetchOutput(inp)
		if !ok {
			return nil, fmt.Errorf("can't load input #%d: %s", idx, inp.String())
		}
		o, err := ledger.OutputFromBytesReadOnly(odata)
		if err != nil {
			return nil, fmt.Errorf("can't load input #%d: %s, '%v'", idx, inp.String(), err)
		}
		return o, nil
	}
}

func (tx *Transaction) InputLoaderFromState(rdr multistate.StateReader) func(idx byte) (*ledger.Output, error) {
	return tx.InputLoaderByIndex(func(oid base.OutputID) ([]byte, bool) {
		return rdr.GetUTXO(oid)
	})
}

func (tx *Transaction) SequencerAndStemInputData() (seqInputIdx *byte, stemInputIdx *byte, seqID *base.ChainID) {
	if !tx.IsSequencerTransaction() {
		return
	}
	seqMeta := tx.SequencerTransactionData()
	if !seqMeta.SequencerOutputData.ChainConstraint.IsOrigin() {
		seqInputIdx = util.Ref(seqMeta.SequencerOutputData.ChainConstraint.PredecessorInputIndex)
	}
	seqID = util.Ref(seqMeta.SequencerID)

	if tx.IsBranchTransaction() {
		tx.ForEachInput(func(i byte, oid base.OutputID) bool {
			if oid == seqMeta.StemOutputData.PredecessorOutputID {
				stemInputIdx = util.Ref(i)
			}
			return true
		})
	}
	return
}

// SequencerChainPredecessor returns chain predecessor output ID
// If it is chain origin, it returns nil. Otherwise, it may or may not be a sequencer ID
// It also returns index of the inout
func (tx *Transaction) SequencerChainPredecessor() (base.OutputID, byte) {
	seqMeta := tx.SequencerTransactionData()
	util.Assertf(seqMeta != nil, "SequencerChainPredecessor: must be a sequencer transaction")

	if seqMeta.SequencerOutputData.ChainConstraint.IsOrigin() {
		return base.OutputID{}, 0xff
	}
	ret, err := tx.InputAt(seqMeta.SequencerOutputData.ChainConstraint.PredecessorInputIndex)
	util.AssertNoError(err)
	// The following is ensured by the 'chain' and 'sequencer' constraints on the transaction
	// Returned predecessor outputID must be:
	// - if the transaction is branch tx, then it returns tx id which may or may not be a sequencer transaction id
	// - if the transaction is not a branch tx, it must always return sequencer tx id (which may or may not be a branch)
	return ret, seqMeta.SequencerOutputData.ChainConstraint.PredecessorInputIndex
}

func (tx *Transaction) FindChainOutput(chainID base.ChainID) *ledger.OutputWithID {
	var ret *ledger.OutputWithID
	tx.ForEachProducedOutput(func(idx byte, o *ledger.Output, oid base.OutputID) bool {
		cc, idx := o.ChainConstraint()
		if idx == 0xff {
			return true
		}
		cID := cc.ID
		if cc.IsOrigin() {
			cID = base.MakeOriginChainID(oid)
		}
		if cID == chainID {
			ret = &ledger.OutputWithID{
				ID:     oid,
				Output: o,
			}
			return false
		}
		return true
	})
	return ret
}

func (tx *Transaction) FindStemProducedOutput() *ledger.OutputWithID {
	if !tx.IsBranchTransaction() {
		return nil
	}
	return tx.MustProducedOutputWithIDAt(tx.SequencerTransactionData().StemOutputIndex)
}

func (tx *Transaction) EndorsementsVeryShort() string {
	ret := make([]string, tx.NumEndorsements())
	tx.ForEachEndorsement(func(idx byte, txid base.TransactionID) bool {
		ret[idx] = txid.StringVeryShort()
		return true
	})
	return strings.Join(ret, ", ")
}

func (tx *Transaction) ProducedOutputsToString() string {
	ret := make([]string, 0)
	tx.ForEachProducedOutput(func(idx byte, o *ledger.Output, oid base.OutputID) bool {
		ret = append(ret, fmt.Sprintf("  %d :", idx), o.ToString("    "))
		return true
	})
	return strings.Join(ret, "\n")
}

func (tx *Transaction) StateMutations() *multistate.Mutations {
	ret := multistate.NewMutations()
	tx.ForEachInput(func(i byte, oid base.OutputID) bool {
		ret.InsertDelOutputMutation(oid)
		return true
	})
	tx.ForEachProducedOutput(func(_ byte, o *ledger.Output, oid base.OutputID) bool {
		ret.InsertAddOutputMutation(oid, o)
		return true
	})
	ret.InsertAddTxMutation(tx.ID(), tx.Slot(), byte(tx.NumProducedOutputs()-1))

	// TODO not correct. ChainIDs of discontinued chains must be deleted. We leave it as is because tx.StateMutations is not used
	//  in the UTXO tangle but mostly in tests or at the DB inception
	return ret
}

func (tx *Transaction) Lines(inputLoaderByIndex func(i byte) (*ledger.Output, error), prefix ...string) *lines.Lines {
	ctx, err := TxContextFromTransaction(tx, inputLoaderByIndex)
	if err != nil {
		ret := lines.New(prefix...)
		ret.Add("can't create context of transaction %s: '%v'", tx.IDShortString(), err)
		return ret
	}
	return ctx.Lines(prefix...)
}

func (tx *Transaction) ProducedOutputsWithTargetLock(lock ledger.Lock) []*ledger.OutputWithID {
	ret := make([]*ledger.OutputWithID, 0)
	tx.ForEachProducedOutput(func(_ byte, o *ledger.Output, oid base.OutputID) bool {
		if ledger.EqualConstraints(lock, o.Lock()) {
			ret = append(ret, &ledger.OutputWithID{
				ID:     oid,
				Output: o,
			})
		}
		return true
	})
	return ret
}

func (tx *Transaction) LinesShort(prefix ...string) *lines.Lines {
	ret := lines.New(prefix...)
	ret.Add("id: %s", tx.IDString())
	ret.Add("Sender address: %s", tx.SenderAddress().String())
	ret.Add("Total: %s", util.Th(tx.TotalAmount()))
	ret.Add("Inflation: %s", util.Th(tx.InflationAmount()))
	if tx.IsSequencerTransaction() {
		ret.Add("Sequencer output index: %d, Stem output index: %d", tx.sequencerTransactionData.SequencerOutputIndex, tx.sequencerTransactionData.StemOutputIndex)
	}
	ret.Add("Endorsements (%d):", tx.NumEndorsements())
	tx.ForEachEndorsement(func(idx byte, txid base.TransactionID) bool {
		ret.Add("    %3d: %s", idx, txid.String())
		return true
	})
	ret.Add("Inputs (%d):", tx.NumInputs())
	tx.ForEachInput(func(i byte, oid base.OutputID) bool {
		ret.Add("    %3d: %s", i, oid.String())
		ret.Add("       Unlock data: %s", UnlockDataToString(tx.MustUnlockDataAt(i)))
		return true
	})
	ret.Add("Outputs (%d):", tx.NumProducedOutputs())
	pref := ""
	if len(prefix) > 0 {
		pref = prefix[0]
	}
	tx.ForEachProducedOutput(func(idx byte, o *ledger.Output, oid base.OutputID) bool {
		ret.Add("%s", oid.StringShort())
		ret.Append(o.Lines(pref + "    "))
		return true
	})
	return ret
}

func (tx *Transaction) String() string {
	return tx.LinesShort().String()
}

func LinesFromTransactionBytes(txBytes []byte, inputLoader func(i byte) (*ledger.Output, error), prefix ...string) *lines.Lines {
	tx, err := FromBytes(txBytes)
	if err != nil {
		return lines.New(prefix...).Add("FromBytes returned: %v", err)
	}
	txCtx, err := TxContextFromTransaction(tx, inputLoader)
	if err != nil {
		return lines.New(prefix...).Add("TxContextFromTransaction returned: %v", err)
	}
	return txCtx.Lines(prefix...)
}

// BaselineDirection is the input, endorsement or explicit baseline of the sequencer transaction where to look for a baseline branch
// It is not a baseline yet (but it can be one).
// It is assumed tx is a sequencer transaction and not the origin of the sequencer chain
func (tx *Transaction) BaselineDirection() (ret base.TransactionID) {
	util.Assertf(tx.IsSequencerTransaction(), "tx.IsSequencerTransaction()")
	var ok bool
	if ret, ok = tx.ExplicitBaseline(); ok {
		return
	}
	predOid, idx := tx.SequencerChainPredecessor()
	util.Assertf(idx != 0xff, "inconsistency: sequencer milestone cannot be a chain origin. %s hex = %s", tx.IDShortString, tx.IDStringHex)

	if tx.IsBranchTransaction() {
		ret = predOid.TransactionID()
		return
	}
	// not branch tx
	if predOid.Slot() != tx.Slot() || !predOid.IsSequencerTransaction() {
		util.Assertf(tx.NumEndorsements() > 0, "tx.NumEndorsements()>0")
		// follow the endorsement if it is cross-slot or the predecessor is not a sequencer tx
		ret = tx.EndorsementAt(0)
	} else {
		ret = predOid.TransactionID()
	}
	return
}
