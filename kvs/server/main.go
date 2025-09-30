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

type LockMap struct {
    mu    sync.Mutex
    locks map[string]*sync.Mutex
}
type KVService struct {
	sync.Mutex
	mp        map[string]string
	stats     Stats
	prevStats Stats
	lastPrint time.Time
	lockMap   LockMap
}

func NewKVService() *KVService {
	kvs := &KVService{}
	kvs.mp = make(map[string]string)
	kvs.lastPrint = time.Now()
	return kvs
}



func (kv *KVService) Get(request *kvs.GetRequest, response *kvs.GetResponse) error {

	if !request.Commit { //phase1
		if value, found := kv.mp[request.Key]; found {
			if kv.lockMap.locks[request.Key].TryLock() {
				response.Vote = true
			}
			else{
				response.Vote = false
			}
		}
		else{
			response.Vote = true
		}
	}
	else{
		response.Value = kv.mp[request.key]
		kv.lockMap.locks[request.Key].Unlock()
	}

	return nil
}

func (kv *KVService) Put(request *kvs.PutRequest, response *kvs.PutResponse) error {
	response.vote=false
	if !request.Commit { //phase1
		if value, found := kv.mp[request.Key]; found {
			if kv.lockMap.locks[request.Key].TryLock() {
				response.Vote = true
			}
		}
		else{
			if kv.TryLock(){
				if kv.lockMap.mu.TryLock(){
					kv.lockMap.locks[request.Key] = &sync.Mutex 
					if kv.lockMap.locks[request.Key].TryLock(){
						response.Vote=true
					}
				}
			}

			
		}
	}
	else{//Phase2
		if value, found := kv.mp[request.Key]; found {
			kv.mp[request.Key] = response.Value
			kv.lockMap.locks[request.Key].Unlock()
			response.ack = True
		}
		else{
			kv.mp[request.Key] = response.Value
			kv.lockMap.locks[request.Key].Unlock()
			kv.lockMap.mu.UnLock()
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