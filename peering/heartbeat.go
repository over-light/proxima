package peering

import (
	"encoding/binary"
	"errors"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/lunfardo314/proxima/util"
)

const traceHeartbeat = false

// clockTolerance is how big the difference between local and remote clocks is tolerated
const clockTolerance = 5 * time.Second // for testing only

func (p *Peer) isAlive() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p._isAlive()
}

func (p *Peer) _isAlive() bool {
	// peer is alive if its last activity is at least 3 heartbeats old
	return time.Now().Sub(p.lastActivity) < aliveDuration
}

func (ps *Peers) logLostConnectionWithPeer(id peer.ID) {
	p := ps.getPeer(id)
	if p == nil {
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p._isAlive() && p.needsLogLostConnection {
		ps.log.Infof("host %s (self) lost connection with peer %s (%s)", shortPeerIDString(ps.host.ID()), shortPeerIDString(id), ps.PeerName(id))
		p.needsLogLostConnection = false
	}
}

func checkRemoteClockTolerance(remoteTime time.Time) (bool, bool) {
	nowis := time.Now() // local clock
	var diff time.Duration

	var behind bool
	if nowis.After(remoteTime) {
		diff = nowis.Sub(remoteTime)
		behind = true
	} else {
		diff = remoteTime.Sub(nowis)
		behind = false
	}
	return diff < clockTolerance, behind
}

// heartbeat protocol is used to monitor if peer is alive and to ensure clocks are synced within tolerance interval

func (ps *Peers) heartbeatStreamHandler(stream network.Stream) {
	defer stream.Close()

	id := stream.Conn().RemotePeer()
	if traceHeartbeat {
		ps.trace("heartbeatStreamHandler invoked in %s from %s", ps.host.ID().String(), id.String())
	}

	p := ps.getPeer(id)
	if p == nil {
		// peer not found
		ps.log.Warnf("unknown peer %s", id.String())
		return
	}

	msgData, err := readFrame(stream)
	if err != nil || len(msgData) != 8 {
		if err == nil {
			err = errors.New("exactly 8 bytes of clock value expected")
		}
		ps.log.Errorf("error while reading message from peer %s: %v", id.String(), err)
		return
	}

	remoteClock := time.Unix(0, int64(binary.BigEndian.Uint64(msgData)))
	if clockOk, behind := checkRemoteClockTolerance(remoteClock); !clockOk {
		b := "ahead"
		if behind {
			b = "behind"
		}
		ps.log.Warnf("clock of the peer %s is %s of the local clock more than tolerance interval %v", id.String(), b, clockTolerance)
		// TODO do something with remote peer with unsynced clock
		// for example mark unworkable and then retry after 1 min or so
		return
	}

	if !p.isAlive() {
		ps.log.Infof("libp2p host %s (self) connected to peer %s (%s)", shortPeerIDString(ps.host.ID()), shortPeerIDString(id), ps.PeerName(id))
	}

	p.mutex.Lock()
	p.lastActivity = time.Now()
	p.needsLogLostConnection = true
	p.mutex.Unlock()

	util.Assertf(p.isAlive(), "isAlive")
}

func (ps *Peers) sendHeartbeatToPeer(id peer.ID) {
	if traceHeartbeat {
		ps.trace("sendHeartbeatToPeer from %s to %s", ps.host.ID().String(), id.String())
	}

	stream, err := ps.host.NewStream(ps.ctx, id, lppProtocolHeartbeat)
	if err != nil {
		return
	}
	defer stream.Close()

	var timeBuf [8]byte
	binary.BigEndian.PutUint64(timeBuf[:], uint64(time.Now().UnixNano()))

	_ = writeFrame(stream, timeBuf[:])
}

const (
	heartbeatRate      = time.Second
	aliveNumHeartbeats = 2
	aliveDuration      = time.Duration(aliveNumHeartbeats) * heartbeatRate
)

func (ps *Peers) heartbeatLoop() {
	for !ps.stopHeartbeat.Load() {
		for _, id := range ps.getPeerIDs() {
			ps.logLostConnectionWithPeer(id)
			ps.sendHeartbeatToPeer(id)
		}
		time.Sleep(heartbeatRate)
	}
}
