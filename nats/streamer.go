package nats

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/ircop/dpinger/logger"
	"github.com/ircop/dproto"
	nats "github.com/nats-io/go-nats-streaming"
	"github.com/sasha-s/go-deadlock"
	"os"
	"strings"
	"time"
)

type NatsClient struct {
	Conn			nats.Conn
	URL				string
	DbChan			string
	PingChan		string
}

var SendLock deadlock.Mutex
var Nats NatsClient

func Init(url string, dbChan string, pingChan string) error {
	logger.Log("Initializing NATS connection...")
	var err error

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("Cannot discover hostname")
	}
	hostname = strings.Replace(hostname, ".", "-", -1) + "-pinger"

	Nats.URL = url
	Nats.DbChan = dbChan
	Nats.PingChan = pingChan

	if Nats.Conn, err = nats.Connect("test-cluster", hostname,
		nats.ConnectWait(time.Second * 10),
		nats.NatsURL(Nats.URL)); err != nil {
		return err
	}

	// Subscribe to DB channel
	_, err = Nats.Conn.Subscribe(dbChan, func(msg *nats.Msg) {
		// parse packet: packets useful for us are dbd and db update
		go processPacket(msg)
	},
		nats.DurableName(dbChan),
		nats.MaxInflight(200),
		nats.SetManualAckMode(),
		nats.AckWait(time.Minute * 5))
	if err != nil {
		return err
	}

	return nil
}

func processPacket(msg *nats.Msg) {
	defer msg.Ack()

	var packet dproto.DPacket
	err := proto.Unmarshal(msg.Data, &packet)
	if err != nil {
		logger.Err("Cannot unmarshal DPacket: %s", err.Error())
		return
	}

	if packet.PacketType == dproto.PacketType_DB {
		var dbd dproto.DBD
		if err := proto.Unmarshal(packet.Payload.Value, &dbd); err != nil {
			logger.Err("Failed to unmarshal DBD packet: %s", err.Error())
			return
		}
		processDBD(dbd)
		return
	}

	if packet.PacketType == dproto.PacketType_DB_UPDATE {
		var update dproto.DBUpdate
		if err := proto.Unmarshal(packet.Payload.Value, &update); err != nil {
			logger.Err("Failed to unmarshal DBUpdate packet: %s", err.Error())
			return
		}
		processUpdate(update)
		return
	}
}

func RequestSync() {
	packet := dproto.DPacket{
		PacketType:dproto.PacketType_DB_REQUEST,
		Payload:&any.Any{
			Value:[]byte{},
			TypeUrl:"nil",
		},
	}

	packetBts, err := proto.Marshal(&packet)
	if err != nil {
		logger.Err("Cannot marshal DBD Request: %s", err.Error())
		return
	}

	SendLock.Lock()
	defer SendLock.Unlock()
	logger.Debug("Sending DB_REQUEST to %s: %+#v", Nats.DbChan, packetBts)
	_, err = Nats.Conn.PublishAsync(
		Nats.DbChan,
		packetBts,
		func(g string, e error) {
		if e != nil {
			logger.Err("Error recieving NATS ACK: %s", e.Error())
		}
	})
	if err != nil {
		logger.Err("Failed to send NATS DBD request: %s", err.Error())
	}
}