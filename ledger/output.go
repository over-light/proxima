package ledger

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/lunfardo314/easyfl/easyfl_util"
	"github.com/lunfardo314/easyfl/tuples"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/lines"
	"golang.org/x/crypto/blake2b"
)

type (
	Output struct {
		*tuples.Tuple
	}

	OutputBuilder struct {
		*tuples.TupleEditable
	}

	OutputWithID struct {
		ID     base.OutputID
		Output *Output
	}

	OutputDataWithID struct {
		ID   base.OutputID
		Data []byte
	}

	OutputDataWithChainID struct {
		OutputDataWithID
		ChainID base.ChainID
	}

	OutputWithChainID struct {
		OutputWithID
		ChainID                    base.ChainID
		PredecessorConstraintIndex byte
	}

	SequencerOutputData struct {
		SequencerConstraint      *SequencerConstraint
		ChainConstraint          *ChainConstraint
		AmountOnChain            uint64
		SequencerConstraintIndex byte
		MilestoneData            *MilestoneData
	}
)

func NewOutput(buildFun func(o *OutputBuilder)) *Output {
	arr := tuples.EmptyTupleEditable(256)
	builder := &OutputBuilder{arr}
	buildFun(builder)
	return &Output{arr.Tuple()}
}

func OutputBasic(amount uint64, lock Lock) *Output {
	return NewOutput(func(o *OutputBuilder) {
		o.WithLock(lock).WithAmount(amount)
	})
}

func OutputBuilderFromBytes(data []byte) (*OutputBuilder, error) {
	ret, err := tuples.TupleFromBytesEditable(data, 256)
	if err != nil {
		return nil, fmt.Errorf("OutputBuilderFromBytes: %v", err)
	}
	return &OutputBuilder{ret}, nil
}

func OutputFromBytesReadOnly(data []byte, validateOpt ...func(*Output) error) (*Output, error) {
	ret, _, _, err := OutputFromBytesMain(data)
	if err != nil {
		return nil, err
	}
	for _, validate := range validateOpt {
		if err := validate(ret); err != nil {
			return nil, err
		}
	}
	return ret, nil
}

func OutputFromHexString(hexStr string, validateOpt ...func(*Output) error) (*Output, error) {
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	ret, _, _, err := OutputFromBytesMain(data)
	if err != nil {
		return nil, err
	}
	for _, validate := range validateOpt {
		if err := validate(ret); err != nil {
			return nil, err
		}
	}
	return ret, nil
}

func OutputFromBytesMain(data []byte) (*Output, Amount, Lock, error) {
	arr, err := tuples.TupleFromBytes(bytes.Clone(data), 256)
	if err != nil {
		return nil, 0, nil, err
	}
	ret := &Output{arr}

	var amount Amount
	var lock Lock
	if ret.NumElements() < 2 {
		return nil, 0, nil, fmt.Errorf("at least 2 constraints expected")
	}
	amountBin, err := ret.At(int(ConstraintIndexAmount))
	if err != nil {
		return nil, 0, nil, err
	}
	if amount, err = AmountFromBytes(amountBin); err != nil {
		return nil, 0, nil, err
	}
	lockBin, err := ret.At(int(ConstraintIndexLock))
	if err != nil {
		return nil, 0, nil, err
	}
	if lock, err = LockFromBytes(lockBin); err != nil {
		return nil, 0, nil, err
	}
	return ret, amount, lock, nil
}

func (o *Output) ConstraintsRawBytes() [][]byte {
	ret := make([][]byte, o.NumConstraints())
	o.ForEach(func(i int, data []byte) bool {
		ret[i] = data
		return true
	})
	return ret
}

func (o *Output) StemLock() (*StemLock, bool) {
	ret, ok := o.Lock().(*StemLock)
	return ret, ok
}

func (o *Output) MustStemLock() *StemLock {
	ret, ok := o.StemLock()
	util.Assertf(ok, "can't get stem output")
	return ret
}

// WithAmount can only be used inside r/o override closure
func (o *OutputBuilder) WithAmount(amount uint64) *OutputBuilder {
	o.MustPutAtIdxWithPadding(ConstraintIndexAmount, NewAmount(amount).Bytes())
	return o
}

func (o *Output) Amount() uint64 {
	bin, err := o.At(int(ConstraintIndexAmount))
	util.AssertNoError(err)
	ret, err := AmountFromBytes(bin)
	util.AssertNoError(err)
	return uint64(ret)
}

// WithLock can only be used inside r/o override closure
func (o *OutputBuilder) WithLock(lock Lock) *OutputBuilder {
	o.PutConstraint(lock.Bytes(), ConstraintIndexLock)
	return o
}

func (o *Output) Hex() string {
	return hex.EncodeToString(o.Bytes())
}

// Clone clones output and gives a chance to modify it
func (o *Output) Clone(buildFun ...func(o *OutputBuilder)) *Output {
	if len(buildFun) == 0 {
		ret, err := OutputFromBytesReadOnly(o.Bytes())
		util.AssertNoError(err)
		return ret
	}
	builder, err := OutputBuilderFromBytes(o.Bytes())
	util.AssertNoError(err)
	buildFun[0](builder)
	return &Output{builder.Tuple()}
}

// MustPushConstraint can only be used inside the edit closure
func (o *OutputBuilder) MustPushConstraint(c []byte) byte {
	util.Assertf(o.NumConstraints() < 256, "too many constraints")
	o.MustPush(c)
	return byte(o.NumElements() - 1)
}

// PutConstraint places bytecode at the specific index
func (o *OutputBuilder) PutConstraint(c []byte, idx byte) {
	o.MustPutAtIdxWithPadding(idx, c)
}

func (o *OutputBuilder) PutAmount(amount uint64) {
	o.PutConstraint(NewAmount(amount).Bytes(), ConstraintIndexAmount)
}

func (o *OutputBuilder) PutLock(lock Lock) {
	o.PutConstraint(lock.Bytes(), ConstraintIndexLock)
}

func (o *Output) MustConstraintAt(idx byte) []byte {
	return o.MustAt(int(idx))
}

func (o *OutputBuilder) NumConstraints() int {
	return o.NumElements()
}

func (o *Output) NumConstraints() int {
	return o.NumElements()
}

func (o *Output) ForEachConstraint(fun func(idx byte, constr []byte) bool) {
	o.ForEach(func(i int, data []byte) bool {
		return fun(byte(i), data)
	})
}

func (o *Output) Lock() Lock {
	ret, err := LockFromBytes(o.MustAt(int(ConstraintIndexLock)))
	util.AssertNoError(err)
	return ret
}

func (o *Output) AccountIDs() []AccountID {
	ret := make([]AccountID, 0)
	for _, a := range o.Lock().Accounts() {
		ret = append(ret, a.AccountID())
	}
	return ret
}

func (o *Output) TimeLock() (uint32, bool) {
	var ret Timelock
	var err error
	found := false
	o.ForEachConstraint(func(idx byte, constr []byte) bool {
		if idx < ConstraintIndexFirstOptionalConstraint {
			return true
		}
		if ret, err = TimelockFromBytes(constr); err == nil {
			found = true
			return false
		}
		return true
	})
	if found {
		return uint32(ret), true
	}
	return 0, false
}

// MessageWithED25519Sender return sender address and constraintIndex if found, otherwise nil, 0xff
func (o *Output) MessageWithED25519Sender() (*MessageWithED25519Sender, byte) {
	var ret *MessageWithED25519Sender
	var err error
	foundIdx := byte(0xff)
	o.ForEachConstraint(func(idx byte, constr []byte) bool {
		if idx < ConstraintIndexFirstOptionalConstraint {
			return true
		}
		ret, err = MessageWithSenderED25519FromBytes(constr)
		if err == nil {
			foundIdx = idx
			return false
		}
		return true
	})
	if foundIdx != 0xff {
		return ret, foundIdx
	}
	return nil, 0xff
}

// ChainConstraint finds and parses chain constraint. Returns its constraintIndex or 0xff if not found
func (o *Output) ChainConstraint() (*ChainConstraint, byte) {
	var ret *ChainConstraint
	var err error
	found := byte(0xff)
	o.ForEachConstraint(func(idx byte, constr []byte) bool {
		if idx < ConstraintIndexFirstOptionalConstraint {
			return true
		}
		ret, err = ChainConstraintFromBytes(constr)
		if err == nil {
			found = idx
			return false
		}
		return true
	})
	if found != 0xff {
		return ret, found
	}
	return nil, 0xff
}

// SequencerConstraint finds and parses chain constraint. Returns its constraintIndex or 0xff if not found
func (o *Output) SequencerConstraint() (*SequencerConstraint, byte) {
	var ret *SequencerConstraint
	var err error
	found := byte(0xff)
	o.ForEachConstraint(func(idx byte, constr []byte) bool {
		if idx < ConstraintIndexFirstOptionalConstraint {
			return true
		}
		ret, err = SequencerConstraintFromBytes(constr)
		if err == nil {
			found = idx
			return false
		}
		return true
	})
	if found != 0xff {
		return ret, found
	}
	return nil, 0xff
}

func (o *Output) IsSequencerOutput() bool {
	_, idx := o.SequencerConstraint()
	return idx != 0xff
}

// InflationConstraint finds and parses inflation constraint. Returns its constraintIndex or 0xff if not found
func (o *Output) InflationConstraint() (*InflationConstraint, byte) {
	var ret *InflationConstraint
	var err error
	found := byte(0xff)
	o.ForEachConstraint(func(idx byte, constr []byte) bool {
		if idx < ConstraintIndexFirstOptionalConstraint {
			return true
		}
		ret, err = InflationConstraintFromBytes(constr)
		if err == nil {
			found = idx
			return false
		}
		return true
	})
	if found != 0xff {
		return ret, found
	}
	return nil, 0xff
}

func (o *Output) Inflation() uint64 {
	if inflationConstraint, idx := o.InflationConstraint(); idx != 0xff {
		return inflationConstraint.InflationAmount
	}
	return 0
}

func (o *Output) SequencerOutputData() (*SequencerOutputData, bool) {
	chainConstraint, chainConstraintIndex := o.ChainConstraint()
	if chainConstraintIndex == 0xff {
		return nil, false
	}
	var err error
	seqConstraintIndex := byte(0xff)
	var seqConstraint *SequencerConstraint

	o.ForEachConstraint(func(idx byte, constr []byte) bool {
		if idx < ConstraintIndexFirstOptionalConstraint || idx == chainConstraintIndex {
			return true
		}
		seqConstraint, err = SequencerConstraintFromBytes(constr)
		if err == nil {
			seqConstraintIndex = idx
			return false
		}
		return true
	})
	if seqConstraintIndex == 0xff {
		return nil, false
	}
	if seqConstraint.ChainConstraintIndex != chainConstraintIndex {
		return nil, false
	}

	return &SequencerOutputData{
		SequencerConstraintIndex: seqConstraintIndex,
		SequencerConstraint:      seqConstraint,
		ChainConstraint:          chainConstraint,
		AmountOnChain:            o.Amount(),
		MilestoneData:            ParseMilestoneData(o),
	}, true
}

func (o *Output) DelegationLock() *DelegationLock {
	lock := o.Lock()
	if lock.Name() != DelegationLockName {
		return nil
	}
	return lock.(*DelegationLock)
}

func (o *Output) ToString(prefix ...string) string {
	return o.Lines(prefix...).String()
}

func (o *Output) Lines(prefix ...string) *lines.Lines {
	pref := ""
	if len(prefix) > 0 {
		pref = prefix[0]
	}
	return o._lines(pref, false)
}

func (o *Output) LinesVerbose(prefix ...string) *lines.Lines {
	pref := ""
	if len(prefix) > 0 {
		pref = prefix[0]
	}
	return o._lines(pref, true)
}

func (o *Output) String() string {
	return o.Lines().String()
}

func (o *Output) _lines(prefix string, verbose bool) *lines.Lines {
	ret := lines.New()
	o.ForEachConstraint(func(i byte, data []byte) bool {
		bc := ""
		if verbose {
			bc = fmt.Sprintf(prefix+"   bytecode: %s", easyfl_util.Fmt(data))
		}
		c, err := ConstraintFromBytes(data)
		if err != nil {
			ret.Add("%s%d: %v%s", prefix, i, err, bc)
		} else {
			ret.Add("%s%d: %s%s", prefix, i, c.String(), bc)
		}
		return true
	})
	return ret
}

func (o *Output) LinesPlain() *lines.Lines {
	ret := lines.New()
	o.ForEachConstraint(func(i byte, data []byte) bool {
		c, err := ConstraintFromBytes(data)
		if err != nil {
			ret.Add(err.Error())
		} else {
			ret.Add(c.Source())
		}
		return true
	})
	return ret
}

func (o *OutputDataWithID) Parse(validOpt ...func(o *Output) error) (*OutputWithID, error) {
	ret, err := OutputFromBytesReadOnly(o.Data, validOpt...)
	if err != nil {
		return nil, err
	}
	return &OutputWithID{
		ID:     o.ID,
		Output: ret,
	}, nil
}

// ParseAsChainOutput parses raw output data expecting chain output. Returns parsed output and index of the chain constraint in it
func (o *OutputDataWithID) ParseAsChainOutput() (*OutputWithChainID, byte, error) {
	var chainConstr *ChainConstraint
	var idx byte
	var chainID base.ChainID

	ret, err := o.Parse(func(oParsed *Output) error {
		chainConstr, idx = oParsed.ChainConstraint()
		if idx == 0xff {
			return fmt.Errorf("can't find chain constraint")
		}
		chainID = chainConstr.ID
		if chainID == base.NilChainID {
			chainID = blake2b.Sum256(o.ID[:])
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return &OutputWithChainID{
		OutputWithID:               *ret,
		ChainID:                    chainID,
		PredecessorConstraintIndex: chainConstr.PredecessorInputIndex,
	}, idx, nil
}

func (o *OutputDataWithID) MustParse() *OutputWithID {
	ret, err := o.Parse()
	util.AssertNoError(err)
	return ret
}

func ExtractChainID(o *Output, oid base.OutputID) (chainID base.ChainID, predecessorConstraintIndex byte, ok bool) {
	cc, blockIdx := o.ChainConstraint()
	if blockIdx == 0xff {
		return base.ChainID{}, 0, false
	}
	ret := cc.ID
	if cc.ID == base.NilChainID {
		ret = blake2b.Sum256(oid[:])
	}
	return ret, cc.PredecessorConstraintIndex, true

}

// ExtractChainID return chainID, predecessor constraint index, existence flag
func (o *OutputWithID) ExtractChainID() (chainID base.ChainID, predecessorConstraintIndex byte, ok bool) {
	return ExtractChainID(o.Output, o.ID)
}

func (o *OutputWithID) AsChainOutput() (*OutputWithChainID, error) {
	chainID, predecessorConstraintIdx, ok := o.ExtractChainID()
	if !ok {
		return nil, fmt.Errorf("not a chain output")
	}
	return &OutputWithChainID{
		OutputWithID:               *o,
		ChainID:                    chainID,
		PredecessorConstraintIndex: predecessorConstraintIdx,
	}, nil
}

func (o *OutputWithID) MustAsChainOutput() *OutputWithChainID {
	ret, err := o.AsChainOutput()
	util.AssertNoError(err)
	return ret
}

func (o *OutputWithID) Timestamp() base.LedgerTime {
	return o.ID.Timestamp()
}

func (o *OutputWithID) Clone() *OutputWithID {
	return &OutputWithID{
		ID:     o.ID,
		Output: o.Output.Clone(),
	}
}

func (o *OutputWithID) Lines(prefix ...string) *lines.Lines {
	ret := lines.New(prefix...)
	ret.Add("id: %s, hex: %s", o.ID.String(), o.ID.StringHex())
	if cc, idx := o.Output.ChainConstraint(); idx != 0xff {
		var chainID base.ChainID
		if cc.IsOrigin() {
			chainID = blake2b.Sum256(o.ID[:])
		} else {
			chainID = cc.ID
		}
		ret.Add("ChainID: %s", chainID.String())
	}
	ret.Append(o.Output.Lines(prefix...))
	return ret
}

func (o *OutputWithID) String() string {
	return o.Lines().String()
}

func (o *OutputWithID) Short() string {
	return fmt.Sprintf("%s\n%s", o.ID.StringShort(), o.Output.ToString("   "))
}

func (o *OutputWithID) IDShort() string {
	return o.ID.StringShort()
}

func OutputsWithIDToString(outs ...*OutputWithID) string {
	ret := lines.New()
	for i, o := range outs {
		ret.Add("%d : %s", i, o.ID.StringShort()).
			Add("      bytecode: %s", o.Output.Hex()).
			Append(o.Output.Lines("      "))
	}
	return ret.String()
}

func (o *Output) hasConstraintAt(pos byte, constraintName string) bool {
	constr, err := ConstraintFromBytes(o.MustConstraintAt(pos))
	util.AssertNoError(err)

	return constr.Name() == constraintName
}

func (o *Output) MustHaveConstraintAnyOfAt(pos byte, names ...string) {
	util.Assertf(o.NumConstraints() >= int(pos), "no constraint at position %d", pos)

	constr, err := ConstraintFromBytes(o.MustConstraintAt(pos))
	util.AssertNoError(err)

	for _, n := range names {
		if constr.Name() == n {
			return
		}
	}
	util.Panicf("any of %+v was expected at the position %d, got '%s' instead", names, pos, constr.Name())
}

// MustValidOutput checks if amount and lock constraints are as expected
func (o *Output) MustValidOutput() {
	o.MustHaveConstraintAnyOfAt(0, AmountConstraintName)
	_, err := LockFromBytes(o.MustConstraintAt(1))
	util.AssertNoError(err)
}

func (o *Output) EnoughAmountForStorageDeposit() error {
	if o.Amount() >= o.MinimumStorageDeposit(0) {
		return nil
	}
	return fmt.Errorf("not enough tokens (%s) for the minimum storage deposit (%s)",
		util.Th(o.Amount()), util.Th(o.MinimumStorageDeposit(0)))
}

func (o *Output) MinimumStorageDeposit(extraWeight uint32) uint64 {
	if _, isStem := o.StemLock(); isStem {
		return 0
	}
	return StorageDeposit(len(o.Bytes()))
}

// HashOutputs calculates input commitment from outputs: the hash of lazyarray composed of output data
func HashOutputs(outs ...*Output) [32]byte {
	arr := tuples.EmptyTupleEditable(256)
	for _, o := range outs {
		arr.MustPush(o.Bytes())
	}
	return blake2b.Sum256(arr.Bytes())
}
