package nats

import (
	"github.com/ircop/dpinger/handler"
	"github.com/ircop/dpinger/logger"
	"github.com/ircop/dproto"
)

const OpTypeSYNC = 1
const OpTypeUPDATE = 2

func processDBD(dbd dproto.DBD) {
	// compare given objects with ones in local in-memory DB
	newObjects := dbd.Objects
	newMap := make(map[int64]dproto.DBObject)
	for i := range newObjects {
		newMap[newObjects[i].ID] = *newObjects[i]
	}

	processObjects(newMap, OpTypeSYNC)
}

func processUpdate(update dproto.DBUpdate) {
	newObjects := update.Objects
	objects := make(map[int64]dproto.DBObject)

	for i := range newObjects {
		objects[newObjects[i].ID] = *newObjects[i]
	}

	processObjects(objects, OpTypeUPDATE)
}


func processObjects(newMap map[int64]dproto.DBObject, OpType int64) {
	// remove objects that are in mem, but not in update
	memObjects := make(map[int64]*handler.Object)
	if OpType == OpTypeSYNC {
		memObjects = handler.GetAllObjects()
		for id, old := range memObjects {
			if _, ok := newMap[id]; !ok {
				// we should remove this object from memory
				//_, ok := handler.ObjectStorage.Load(id)
				//if ok {
				old.MX.Lock()
				if old.Timer != nil {
					old.Timer.Stop()
					old.Timer = nil
				}
				handler.ObjectStorage.Delete(id)
				old.MX.Unlock()
				//}
				logger.Debug("Deleted object #%d", id)
			}
		}
	}
	if OpType == OpTypeUPDATE {
		// get object IDs
		for id := range newMap {
			oInt, ok := handler.ObjectStorage.Load(id)
			if ok {
				o := oInt.(*handler.Object)
				memObjects[id] = o
			}
		}
	}

	// add non-existing objects ; compare stored object fields with new ones
	for id, newObj := range newMap {
		old, ok := memObjects[id]
		if !ok {
			// store new object
			obj := handler.Object{
				DBO:newObj,
				Changed:false,
			}
			// shore in syncmap
			handler.ObjectStorage.Store(newObj.ID, &obj)

			// todo: shedule ping for this object
			obj.ShedulePing()

			logger.Debug("Stored object #%d (%s)", newObj.ID, newObj.Addr)
		} else {
			// compare old/new objects
			old.MX.Lock()
			memDbo := old.DBO
			old.MX.Unlock()

			//logger.Debug("newobj: %+#v", newObj)

			if newObj.Removed == true {
				// remove from memory
				old.MX.Lock()
				if old.Timer != nil {
					old.Timer.Stop()
					old.Timer = nil
				}
				handler.ObjectStorage.Delete(id)
				old.MX.Unlock()
				logger.Debug("Deleted object #%d (%s)", id, newObj.Addr)
				continue
			}

			if memDbo.Alive != newObj.Alive || memDbo.PingInterval != newObj.PingInterval || memDbo.Addr != newObj.Addr {
				// update this
				old.MX.Lock()
				old.DBO = newObj
				old.MX.Unlock()
				logger.Debug("Updated object #%d (%s)", newObj.ID, newObj.Addr)

				if newObj.PingInterval != memDbo.PingInterval {
					// todo: shedule ping for this object
					old.ShedulePing()
				}
			}
		}
	}
}
