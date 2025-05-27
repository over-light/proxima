package seqdata

import (
	"fmt"

	"github.com/lunfardo314/easyfl/easyfl_util"
	"github.com/lunfardo314/proxima/ledger/base"
	"github.com/lunfardo314/proxima/util/lines"
)

type SequencerData struct {
	Name         string
	MinimumFee   uint64
	ChainHeight  uint32
	BranchHeight uint32
	Pace         byte
}

const NumParameters = 5

const (
	KeyName = byte(iota)
	KeyMinimumFee
	KeyChainHeight
	KeyBranchHeight
	KeyPace
)

func (sd *SequencerData) Bytes() []byte {
	m := base.NewSmallPersistentMap()
	m.Set(KeyName, []byte(sd.Name))
	m.Set(KeyMinimumFee, easyfl_util.TrimmedLeadingZeroUint64(sd.MinimumFee))
	m.Set(KeyChainHeight, easyfl_util.TrimmedLeadingZeroUint32(sd.ChainHeight))
	m.Set(KeyBranchHeight, easyfl_util.TrimmedLeadingZeroUint32(sd.BranchHeight))
	m.Set(KeyPace, []byte{sd.Pace})
	return m.Bytes()
}

func (sd *SequencerData) Lines(prefix ...string) *lines.Lines {
	ln := lines.New(prefix...)
	ln.Add("Name: %s", sd.Name)
	ln.Add("Minimum fee: %d", sd.MinimumFee)
	ln.Add("Chain height: %d", sd.ChainHeight)
	ln.Add("Branch height: %d", sd.BranchHeight)
	ln.Add("Pace: %d", sd.Pace)
	return ln
}

func SequencerDataFromBytes(data []byte) (*SequencerData, error) {
	m, err := base.SmallPersistentMapFromBytes(data)
	if err != nil {
		return nil, err
	}
	if m.Len() > NumParameters {
		return nil, fmt.Errorf("wrong number of parameters in sequencer data")
	}
	ret := &SequencerData{}
	ret.Name = string(m.Get(KeyName))
	ret.MinimumFee, err = easyfl_util.Uint64FromBytes(m.Get(KeyMinimumFee))
	if err != nil {
		return nil, err
	}
	ch, err := easyfl_util.Uint64FromBytes(m.Get(KeyChainHeight))
	if err != nil {
		return nil, err
	}
	ret.ChainHeight = uint32(ch)
	bh, err := easyfl_util.Uint64FromBytes(m.Get(KeyBranchHeight))
	if err != nil {
		return nil, err
	}
	ret.BranchHeight = uint32(bh)
	pace := m.Get(KeyPace)
	if len(pace) > 0 && len(pace) != 1 {
		return nil, fmt.Errorf("wrong 'pace' parameter")
	}
	ret.Pace = pace[0]
	return ret, nil
}
