package txutils

import (
	"fmt"
	"sort"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/ledger/base"
	"golang.org/x/crypto/blake2b"
)

func ParseAndSortOutputData(outs []*ledger.OutputDataWithID, filter func(oid *ledger.OutputID, o *ledger.Output) bool, desc ...bool) ([]*ledger.OutputWithID, error) {
	ret, err := ParseOutputDataAndFilter(outs, filter)
	if err != nil {
		return nil, err
	}
	if len(desc) > 0 && desc[0] {
		sort.Slice(ret, func(i, j int) bool {
			return ret[i].Output.Amount() > ret[j].Output.Amount()
		})
	} else {
		sort.Slice(ret, func(i, j int) bool {
			return ret[i].Output.Amount() < ret[j].Output.Amount()
		})
	}
	return ret, nil
}

func ParseOutputDataAndFilter(outs []*ledger.OutputDataWithID, filter func(oid *ledger.OutputID, o *ledger.Output) bool) ([]*ledger.OutputWithID, error) {
	ret := make([]*ledger.OutputWithID, 0, len(outs))
	for _, od := range outs {
		out, err := ledger.OutputFromBytesReadOnly(od.Data)
		if err != nil {
			return nil, err
		}
		if filter != nil && !filter(&od.ID, out) {
			continue
		}
		ret = append(ret, &ledger.OutputWithID{
			ID:     od.ID,
			Output: out,
		})
	}
	return ret, nil
}

func FilterOutputsSortByAmount(outs []*ledger.OutputWithID, filter func(o *ledger.Output) bool, desc ...bool) []*ledger.OutputWithID {
	ret := make([]*ledger.OutputWithID, 0, len(outs))
	for _, out := range outs {
		if filter != nil && !filter(out.Output) {
			continue
		}
		ret = append(ret, out)
	}
	if len(desc) > 0 && desc[0] {
		sort.Slice(ret, func(i, j int) bool {
			return ret[i].Output.Amount() > ret[j].Output.Amount()
		})
	} else {
		sort.Slice(ret, func(i, j int) bool {
			return ret[i].Output.Amount() < ret[j].Output.Amount()
		})
	}
	return ret
}

func ParseAndSortOutputDataUpToAmount(outs []*ledger.OutputDataWithID, amount uint64, filter func(oid *ledger.OutputID, o *ledger.Output) bool, desc ...bool) ([]*ledger.OutputWithID, uint64, base.LedgerTime, error) {
	outsWitID, err := ParseAndSortOutputData(outs, filter, desc...)
	if err != nil {
		return nil, 0, base.NilLedgerTime, err
	}
	retTs := base.NilLedgerTime
	retSum := uint64(0)
	retOuts := make([]*ledger.OutputWithID, 0, len(outs))
	for _, o := range outsWitID {
		retSum += o.Output.Amount()
		retTs = base.MaximumTime(retTs, o.Timestamp())
		retOuts = append(retOuts, o)
		if retSum >= amount {
			break
		}
	}
	if retSum < amount {
		return nil, 0, base.NilLedgerTime, fmt.Errorf("not enough tokens")
	}
	return retOuts, retSum, retTs, nil
}

func FilterChainOutputs(outs []*ledger.OutputWithID) ([]*ledger.OutputWithChainID, error) {
	ret := make([]*ledger.OutputWithChainID, 0)
	for _, o := range outs {
		ch, constraintIndex := o.Output.ChainConstraint()
		if constraintIndex == 0xff {
			continue
		}
		d := &ledger.OutputWithChainID{
			OutputWithID: ledger.OutputWithID{
				ID:     o.ID,
				Output: o.Output,
			},
			PredecessorConstraintIndex: constraintIndex,
		}
		if ch.IsOrigin() {
			h := blake2b.Sum256(o.ID[:])
			d.ChainID = h
		} else {
			d.ChainID = ch.ID
		}
		ret = append(ret, d)
	}
	return ret, nil
}

func forEachOutputReadOnly(outs []*ledger.OutputDataWithID, fun func(o *ledger.Output, odata *ledger.OutputDataWithID) bool) error {
	for _, odata := range outs {
		o, err := ledger.OutputFromBytesReadOnly(odata.Data)
		if err != nil {
			return err
		}
		if !fun(o, odata) {
			return nil
		}
	}
	return nil
}

func ParseChainConstraintsFromData(outs []*ledger.OutputDataWithID) ([]*ledger.OutputWithChainID, error) {
	ret := make([]*ledger.OutputWithChainID, 0)
	err := forEachOutputReadOnly(outs, func(o *ledger.Output, odata *ledger.OutputDataWithID) bool {
		ch, constraintIndex := o.ChainConstraint()
		if constraintIndex == 0xff {
			return true
		}
		d := &ledger.OutputWithChainID{
			OutputWithID: ledger.OutputWithID{
				ID:     odata.ID,
				Output: o,
			},
			PredecessorConstraintIndex: constraintIndex,
		}
		if ch.IsOrigin() {
			h := blake2b.Sum256(odata.ID[:])
			d.ChainID = h
		} else {
			d.ChainID = ch.ID
		}
		ret = append(ret, d)
		return true
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}
