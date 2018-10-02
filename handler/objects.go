package handler

import (
	"github.com/ircop/dpinger/logger"
	"github.com/ircop/dpinger/pinger"
	"github.com/ircop/dproto"
	"github.com/sasha-s/go-deadlock"
	"sync"
	"time"
)

type Object struct {
	DBO		dproto.DBObject
	Timer	*time.Timer
	MX		deadlock.Mutex
	Changed	bool
}

// todo: when finishing ping, CHECK IF OBJECT STILL EXISTS!
var ObjectStorage sync.Map

func GetAllObjects() map[int64]*Object {
	objects := make(map[int64]*Object)

	ObjectStorage.Range(func(key, oInt interface{}) bool {
		obj := oInt.(*Object)

		obj.MX.Lock()
		objects[obj.DBO.ID] = obj
		obj.MX.Unlock()

		return true
	})

	return objects
}

func (o *Object) ShedulePing() {
	o.MX.Lock()
	defer o.MX.Unlock()

	//interval := o.DBO.PingInterval
	interval := 5
	o.Timer = time.AfterFunc(time.Second * time.Duration(interval), func() {
		o.MX.Lock()
		dbo := o.DBO
		o.MX.Unlock()

		//jobs.StartJob(dbo.Addr)
		// send probes
		result, err := pinger.Pinger.SendEchos(dbo.Addr)
		result.LossPercent = 100

		if err != nil {
			logger.Err("Failed to ping %s: %s", dbo.Addr, err.Error())
		} else {
			if result.Recieved == 0 {
				result.Alive = false
			} else {
				result.Alive = true
			}
			if result.Recieved > 0 && len(result.RTTs) > 0 {
				result.Min = 9999
				var sum int = 0
				for i := range result.RTTs {
					if result.Max < result.RTTs[i] {
						result.Max = result.RTTs[i]
					}
					if result.Min > result.RTTs[i] || result.Min != 9999 {
						result.Min = result.RTTs[i]
					}
					sum += int(result.RTTs[i])
				}
				result.Avg = sum / len(result.RTTs)
			}
		}

		result.LossPercent = 100 - (100 / pinger.Pinger.Probes) * int(result.Recieved)

		//logger.Debug("%s: %+#v", dbo.Addr, result)
		if result.Alive != dbo.Alive {
			logger.Log("Updating '%s' to alive = %v", dbo.Addr, result.Alive)
		}

		o.ShedulePing()
	})
}
