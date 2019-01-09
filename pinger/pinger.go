package pinger

import (
	"encoding/binary"
	"fmt"
	"github.com/ircop/dpinger/logger"
	"github.com/sasha-s/go-deadlock"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"net"
	"runtime/debug"
	"sync"
	"time"
)

type PingResult struct {
	MX          deadlock.Mutex
	Received    int64
	Alive       bool
	RTTs        []int64
	Min         int64
	Max         int64
	Avg         int
	LossPercent int
}

var runningPings sync.Map

type PingReply struct {
	Payload		[]byte
	Peer		net.Addr
	BytesRead	int
}

type PingerDaemon struct {
	Listener		*icmp.PacketConn
	ListenerLock	deadlock.Mutex
	ReplyChan		chan PingReply
	Probes			int
}
var Pinger PingerDaemon

func (p *PingerDaemon) Init() error {
	logger.Log("Initializing icmp listener")

	var err error
	p.Listener, err = icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return err
	}
	p.Probes = 4
	p.ReplyChan = make(chan PingReply, 1)

	go p.listen()

	return nil
}

func (p *PingerDaemon) SendEchos(host string) (PingResult, error) {
	pr := PingResult{}
	runningPings.Store(host, &pr)

	//if host == "10.170.2.6" {
	//	logger.Debug("Pinging %s", host)
	//}
	ip := net.ParseIP(host)
	if ip == nil {
		return pr, fmt.Errorf("%s is not valid address", host)
	}

	// send N probes
	for i := 0; i < p.Probes; i++ {
		nsec := time.Now().UnixNano()
		cn := make([]byte, 8)	// 8 is std size of int64
		binary.LittleEndian.PutUint64(cn, uint64(nsec))

		msg := icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{
				Seq:  1000 + i,
				ID:   1000 + i,
				Data: cn,
			},
		}

		wbuf, err := msg.Marshal(nil)
		if err != nil {
			logger.Err("Cannot marshal icmp message: %s", err.Error())
			continue
		}

		if _, err := p.Listener.WriteTo(wbuf, &net.IPAddr{IP: ip}); err != nil {
			logger.Err("Failed to write net: %s", err.Error())
		}
		time.Sleep(time.Second)
	}

	prInt, ok := runningPings.Load(host)
	if !ok {
		return PingResult{}, fmt.Errorf("no running job for %s", host)
	}

	time.Sleep(time.Second * time.Duration(p.Probes))

	result := prInt.(*PingResult)
	result.MX.Lock()
	runningPings.Delete(host)
	result.MX.Unlock()

	return *result, nil
}

func (p *PingerDaemon) listen() {
	for {
		buf := make([]byte, 512)
		//var buf []byte
		n, peer, err := p.Listener.ReadFrom(buf)
		if err != nil {
			logger.Err("Failed to read: %s", err.Error())
			continue
		}

		go func(n int, peer net.Addr, payload []byte) {
			now := time.Now().UnixNano()
			defer func() {
				if r := recover(); r != nil {
					logger.Panic("Recovered in listen(): %+v\n%s", r, debug.Stack())
				}
			}()

			parsed, err := icmp.ParseMessage(1, payload[:n])
			if err != nil {
				logger.Err("Unable to parse ICMP message: %s", err.Error())
				return
			}

			if parsed.Type != ipv4.ICMPTypeEchoReply {
				//logger.Err("Unknown message type %v", parsed.Type)
				return
			}

			// we send sequences 1k+
			if parsed.Body.(*icmp.Echo).Seq < 1000 {
				return
			}

			dt := parsed.Body.(*icmp.Echo).Data
			sent := int64(binary.LittleEndian.Uint64(dt))
			delta := (now - sent) / int64(time.Millisecond)
			//logger.Debug("reply from %s (%d ms)", peer.String(), delta)

			// pass results......
			if prInt, ok := runningPings.Load(peer.String()); ok {
				pr := prInt.(*PingResult)
				pr.MX.Lock()
				if !pr.Alive {
					pr.Alive = true
				}
				pr.Received++
				pr.RTTs = append(pr.RTTs, delta)
				pr.MX.Unlock()
			}

		}(n, peer, buf)
	}
}
