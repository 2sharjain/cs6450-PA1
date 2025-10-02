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

var lockMap sync.Map // map from keys to locks

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


func (kv *KVService) Abort(request *kvs.GetRequest, response *kvs.GetResponse) error {


	if request.IsRead {
		lockMap[request.Key].SUnlock(request.TxnID)
	}else{
		if value, found := kv.mp[request.Key]; found {
			lockMap[request.Key].XUnlock()
		}
		else{
			delete(lockMap, request.Key)
			kv.Unlock()

		}
	}
	response.Ack = true
	return nil
}

func (kv *KVService) Get(request *kvs.GetRequest, response *kvs.GetResponse) error {
	response.Vote = false
	if !request.Commit { //phase1
		if value, found := kv.mp[request.Key]; found {
			if lockMap[request.Key].SLock(request.TxnID) {
				response.Vote = true
			}
		}
		else{
			response.Vote = true
		} 
	}
	else{//Phase 2
		response.Value = kv.mp[request.Key]
		lockMap[request.Key].SUnlock(request.TxnID)
	}

	return nil
}

func (kv *KVService) Put(request *kvs.PutRequest, response *kvs.PutResponse) error {
	response.vote=false
	if !request.Commit { //phase1
		if value, found := kv.mp[request.Key]; found {
			if lockMap[request.Key].XLock(request.TxnID){
				response.Vote = true
			}
		}
		else{
			lockMap.Store(request.Key, &Locks{writeTxn: "", readTxns: make(map[string]string)})
			if lockMap[request.Key].XLock(request.TxnID) && kv.TryLock(){
				response.Vote = true
			}
		}
	}
	else{//Phase2
		if value, found := kv.mp[request.Key]; found {
			kv.mp[request.Key] = response.Value
			lockMap[request.Key].XUnlock()
			response.ack = True
		}
		else{
			kv.mp[request.Key] = response.Value
			lockMap[request.Key].XUnlock()
			kv.Unlock()
			response.ack = True
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