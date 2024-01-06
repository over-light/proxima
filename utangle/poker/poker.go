package poker

import (
	"context"
	"time"

	"github.com/lunfardo314/proxima/utangle/vertex"
	"github.com/lunfardo314/proxima/util"
	"github.com/lunfardo314/proxima/util/consumer"
)

type (
	Poker struct {
		consumer.Consumer[Cmd]
		m map[*vertex.WrappedTx]waitingList
	}

	waitingList struct {
		waiting   []*vertex.WrappedTx
		keepUntil time.Time
	}

	Cmd struct {
		Wanted       *vertex.WrappedTx
		WhoIsWaiting *vertex.WrappedTx
		Cmd          Command
	}

	Command byte
)

const (
	CommandAdd = Command(iota)
	CommandPokeAll
	CommandPeriodicCleanup
)

func NewPoker(ctx context.Context) *Poker {
	ret := &Poker{m: make(map[*vertex.WrappedTx]waitingList)}
	ret.AddOnConsume(ret.consume)
	go ret.periodicLoop(ctx)
	return ret
}

const (
	loopPeriod = 1 * time.Second
	ttlWanted  = 1 * time.Minute
)

func (c *Poker) consume(inp Cmd) {
	switch inp.Cmd {
	case CommandAdd:
		util.Assertf(inp.Wanted != nil, "inp.Wanted != nil")
		util.Assertf(inp.WhoIsWaiting != nil, "inp.WhoIsWaiting != nil")
		c.addCmd(inp.Wanted, inp.WhoIsWaiting)

	case CommandPokeAll:
		util.Assertf(inp.Wanted != nil, "inp.Wanted != nil")
		util.Assertf(inp.WhoIsWaiting == nil, "inp.WhoIsWaiting == nil")
		c.pokeAllCmd(inp.Wanted)

	case CommandPeriodicCleanup:
		util.Assertf(inp.Wanted == nil, "inp.Wanted == nil")
		util.Assertf(inp.WhoIsWaiting == nil, "inp.WhoIsWaiting == nil")
		c.periodicCleanup()
	}
}

func (c *Poker) addCmd(wanted, whoIsWaiting *vertex.WrappedTx) {
	lst := c.m[wanted]
	if len(lst.waiting) == 0 {
		lst.waiting = []*vertex.WrappedTx{whoIsWaiting}
	} else {
		lst.waiting = util.AppendUnique(lst.waiting, whoIsWaiting)
	}
	lst.keepUntil = time.Now().Add(ttlWanted)
	c.m[wanted] = lst
}

func (c *Poker) pokeAllCmd(wanted *vertex.WrappedTx) {
	lst := c.m[wanted]
	if len(lst.waiting) > 0 {
		for _, vid := range lst.waiting {
			vid.PokeWith(wanted)
		}
		delete(c.m, wanted)
	}
}

func (c *Poker) periodicCleanup() {
	toDelete := make([]*vertex.WrappedTx, 0)
	nowis := time.Now()
	for wanted, lst := range c.m {
		if nowis.After(lst.keepUntil) {
			toDelete = append(toDelete, wanted)
		}
	}
	for _, vid := range toDelete {
		delete(c.m, vid)
	}
}

func (c *Poker) periodicLoop(ctx context.Context) {
	defer c.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(loopPeriod):
		}
		c.Push(Cmd{Cmd: CommandPeriodicCleanup})
	}
}

func (c *Poker) PokeMe(me, waitingFor *vertex.WrappedTx) {
	c.Push(Cmd{
		Wanted:       waitingFor,
		WhoIsWaiting: me,
		Cmd:          CommandAdd,
	})
}

func (c *Poker) PokeAllWith(vid *vertex.WrappedTx) {
	c.Push(Cmd{
		Wanted: vid,
		Cmd:    CommandPokeAll,
	})
}
