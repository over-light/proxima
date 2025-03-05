package vertex

import (
	"strings"

	"github.com/lunfardo314/proxima/ledger"
	"github.com/lunfardo314/proxima/util/lines"
)

func (o *WrappedOutput) DecodeID() *ledger.OutputID {
	if o.VID == nil {
		ret := ledger.MustNewOutputID(&ledger.TransactionID{}, o.Index)
		return &ret
	}
	ret := o.VID.OutputID(o.Index)
	return &ret
}

func (o *WrappedOutput) IDShortString() string {
	if o == nil {
		return "<nil>"
	}
	return o.DecodeID().StringShort()
}

func (o *WrappedOutput) Timestamp() ledger.Time {
	return o.VID.Timestamp()
}

func (o *WrappedOutput) Slot() ledger.Slot {
	return o.VID.Slot()
}

func (o *WrappedOutput) IsAvailable() (available bool) {
	o.VID.RUnwrap(UnwrapOptions{
		Vertex: func(v *Vertex) {
			available = int(o.Index) < v.Tx.NumProducedOutputs()
		},
		DetachedVertex: func(v *DetachedVertex) {
			available = int(o.Index) < v.Tx.NumProducedOutputs()
		},
		VirtualTx: func(v *VirtualTransaction) {
			_, available = v.OutputAt(o.Index)
		},
	})
	return
}

func (o *WrappedOutput) Output() (ret *ledger.Output) {
	o.VID.Unwrap(UnwrapOptions{
		Vertex: func(v *Vertex) {
			var err error
			if ret, err = v.Tx.ProducedOutputAt(o.Index); err != nil {
				ret = nil
			}
		},
		DetachedVertex: func(v *DetachedVertex) {
			var err error
			if ret, err = v.Tx.ProducedOutputAt(o.Index); err != nil {
				ret = nil
			}
		},
		VirtualTx: func(v *VirtualTransaction) {
			var available bool
			if ret, available = v.OutputAt(o.Index); !available {
				ret = nil
			}
		},
	})
	return
}

func (o *WrappedOutput) OutputWithID() *ledger.OutputWithID {
	ret := ledger.OutputWithID{
		ID:     *o.DecodeID(),
		Output: o.Output(),
	}
	if ret.Output == nil {
		return nil
	}
	return &ret
}

func (o *WrappedOutput) Lock() ledger.Lock {
	if out := o.Output(); out != nil {
		return out.Lock()
	}
	return nil
}

func (o *WrappedOutput) LockName() string {
	if l := o.Lock(); l != nil {
		return l.Name()
	}
	return ""
}

func (o *WrappedOutput) IDHasFragment(frag string) bool {
	return strings.Contains(o.DecodeID().String(), frag)
}

func WrappedOutputsShortLines(wOuts []WrappedOutput) *lines.Lines {
	ret := lines.New()
	for _, wOut := range wOuts {
		ret.Add(wOut.IDShortString())
	}
	return ret
}

func (o *WrappedOutput) ValidID() bool {
	return int(o.Index) < o.VID.NumProducedOutputs()
}
