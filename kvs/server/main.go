package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"sync"
	"time"

	"github.com/rstutsman/cs6450-labs/kvs"
)

type Stats struct {
	puts uint64
	gets uint64
}

func (s *Stats) Sub(prev *Stats) Stats {
	r := Stats{}
	r.puts = s.puts - prev.puts
	r.gets = s.gets - prev.gets
	return r
}


type KVService struct {
	sync.Mutex
	mp        map[string]string
	stats     Stats
	prevStats Stats
	lastPrint time.Time
}

func NewKVService() *KVService {
	kvs := &KVService{}
	kvs.mp = make(map[string]string)
	kvs.lastPrint = time.Now()
	return kvs
}

type Locks struct {
    mu    sync.Mutex
    writeTxn string
	readTxns map[string]string
}

//var lockMap sync.Map // map from keys to locks
type LockMap struct {
    mu sync.Mutex
    mp  map[string]*Locks
} 
var lockMap = &LockMap{mp: make(map[string]*Locks)}

func (l *Locks) SLock(txn_id string) bool {

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.writeTxn == "" {
		l.readTxns[txn_id] = "0"
		return true
	}
	return false
}

func (l *Locks) SUnlock(txn_id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.readTxns, txn_id)
	return true
}

func (l *Locks) XLock(txn_id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.writeTxn == "" && len(l.readTxns) == 0 {
		l.writeTxn = txn_id
		return true
	}
	return false
}
func (l *Locks)XUnlock() bool{
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writeTxn = ""
	return true
}


func (kv *KVService) Abort(request *kvs.AbortRequest, response *kvs.AbortResponse) error {
	kv.Lock()
	_, found := kv.mp[request.Key];
	kv.Unlock()
	if request.IsRead {
		if found {
			lockMap.mp[request.Key].SUnlock(request.TxnID)
		}
	} else {
		if found {
			lockMap.mp[request.Key].XUnlock()
		} else {
			delete(lockMap.mp, request.Key)
			kv.Unlock()
		}
	}
	response.Ack = true
	return nil
}

func (kv *KVService) Get(request *kvs.GetRequest, response *kvs.GetResponse) error {
	response.Vote = false
	kv.Lock()
	_, found := kv.mp[request.Key];
	kv.Unlock()
	if !request.Commit { //phase1
		
		if found {
			if lockMap.mp[request.Key].SLock(request.TxnID) {
				response.Vote = true
			}
		} else {

			response.Vote = true //may need to handle this case differently
		} 
	} else {//Phase 2
		if found {
			response.Value = kv.mp[request.Key]
			lockMap.mp[request.Key].SUnlock(request.TxnID)

		} else{
			response.Value = ""
		}
		
	}
	return nil
}

func (kv *KVService) Put(request *kvs.PutRequest, response *kvs.PutResponse) error {
	response.Vote=false
	kv.Lock()
	_, found := kv.mp[request.Key];
	kv.Unlock()
	if !request.Commit { //phase1
		fmt.Println("In Put at the begin rpc call")
		if found {
			fmt.Println("found the key")

			if lockMap.mp[request.Key].XLock(request.TxnID){
				response.Vote = true
			}
		} else {
			fmt.Println("found not the key")

			if lockMap.mu.TryLock(){
				lockMap.mp[request.Key] = &Locks{readTxns: make(map[string]string)}
				lockMap.mu.Unlock()

				if lockMap.mp[request.Key].XLock(request.TxnID){
					if kv.TryLock(){
						fmt.Println("got the threelock")
						kv.mp[request.Key] = ""
						kv.Unlock()	
						response.Vote = true
					} else {
						lockMap.mp[request.Key].XUnlock()
						lockMap.mu.Lock()
						delete(lockMap.mp, request.Key)
						lockMap.mu.Unlock()

					}
				} else {
					lockMap.mu.Lock()
					delete(lockMap.mp, request.Key)
					lockMap.mu.Unlock()

				}
			}
		}
	} else {//Phase2
		if found {
			kv.mp[request.Key] = request.Value
			lockMap.mp[request.Key].XUnlock()
			response.Ack = true
		} else {
			kv.mp[request.Key] = request.Value
			lockMap.mp[request.Key].XUnlock()
			fmt.Println("In Get, length of map is", len(kv.mp))
			response.Ack = true
		}
		
	}

	return nil
}

func (kv *KVService) printStats() {
	kv.Lock()
	stats := kv.stats
	prevStats := kv.prevStats
	kv.prevStats = stats
	now := time.Now()
	lastPrint := kv.lastPrint
	kv.lastPrint = now
	kv.Unlock()

	diff := stats.Sub(&prevStats)
	deltaS := now.Sub(lastPrint).Seconds()

	fmt.Printf("get/s %0.2f\nput/s %0.2f\nops/s %0.2f\n\n",
		float64(diff.gets)/deltaS,
		float64(diff.puts)/deltaS,
		float64(diff.gets+diff.puts)/deltaS)
}

func main() {
	port := flag.String("port", "8080", "Port to run the server on")
	flag.Parse()

	kvs := NewKVService()
	rpc.Register(kvs)
	rpc.HandleHTTP()

	l, e := net.Listen("tcp", fmt.Sprintf(":%v", *port))
	if e != nil {
		log.Fatal("listen error:", e)
	}

	fmt.Printf("Starting KVS server on :%s\n", *port)

	go func() {
		for {
			kvs.printStats()
			time.Sleep(1 * time.Second)
		}
	}()

	http.Serve(l, nil)
}