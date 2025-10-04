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
	"strconv"
	"os"
    "os/signal"
	"syscall"
	"github.com/rstutsman/cs6450-labs/kvs"
)

type Stats struct {
	puts uint64
	gets uint64
	aborts uint64
}

func (s *Stats) Sub(prev *Stats) Stats {
	r := Stats{}
	r.puts = s.puts - prev.puts
	r.gets = s.gets - prev.gets
	r.aborts = s.aborts - prev.aborts
	return r
}


type KVService struct {
	sync.Mutex
	mp        sync.Map
	stats     Stats
	prevStats Stats
	lastPrint time.Time
}

func NewKVService() *KVService {
	kvs := &KVService{}
	kvs.mp = sync.Map{}
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
	if l.mu.TryLock(){
		defer l.mu.Unlock()
	// l.mu.Lock()
	// defer l.mu.Unlock()
		if l.writeTxn == "" {
			l.readTxns[txn_id] = "0"
			//fmt.Println("Read lock granted to txn:", txn_id)
			return true
		}
	}

	return false
}

func (l *Locks) SUnlock(txn_id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.readTxns, txn_id)
	//fmt.Println("Read unlock granted to txn:", txn_id)

	return true
}

func (l *Locks) XLock(txn_id string) bool {
	if l.mu.TryLock(){
		defer l.mu.Unlock()

		if l.writeTxn == "" && len(l.readTxns) == 0 {
		l.writeTxn = txn_id
		//fmt.Println("write lock granted to txn:", txn_id)

		return true
		}

		if l.writeTxn == "" && len(l.readTxns) == 1 && l.readTxns[txn_id] == "0" {
			delete(l.readTxns, txn_id)
			l.writeTxn = txn_id
			//fmt.Println("write lock granted to txn:", txn_id)

			return true
		}
		if l.writeTxn == txn_id {
			//fmt.Println("write lock granted to txn:", txn_id)

			return true
		}
	}
	return false
}
func (l *Locks)XUnlock() bool{
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writeTxn = ""
	//fmt.Println("writeunlock")
	return true
}


func (kv *KVService) Abort(request *kvs.AbortRequest, response *kvs.AbortResponse) error {
	_, found := kv.mp.Load(request.Key);
	if request.IsRead {
		if found {
			l_val, l_ok := lockMap.Load(request.Key)
			if l_ok {
				lock := l_val.(*Locks)
				lock.SUnlock(request.TxnID)
			}
		}
	} else {
		if found {
			l_val, l_ok := lockMap.Load(request.Key)
			if l_ok {
				lock := l_val.(*Locks)
				lock.XUnlock()
			}
		} else {
			lockMap.Delete(request.Key)
		}
	}
	response.Ack = true
	kv.Lock()
	kv.stats.aborts++
	kv.Unlock()
	return nil
}

func (kv *KVService) Get(request *kvs.GetRequest, response *kvs.GetResponse) error {
	response.Vote = false
	val, found := kv.mp.Load(request.Key);

	if !request.Commit { //phase1
		
		if found {
			l_val, l_ok := lockMap.Load(request.Key)
			if l_ok {
				lock := l_val.(*Locks)
				if lock.SLock(request.TxnID){
					response.Value = val.(string)
					response.Vote = true
				}
			}
		} else {
			response.Value = ""
			response.Vote = true
		} 
	} else {//Phase 2
		if found {
			val, ok := kv.mp.Load(request.Key)
			if ok {
				response.Value = val.(string)
			}
			//lockMap.mp[request.Key].SUnlock(request.TxnID)
			l_val, l_ok := lockMap.Load(request.Key)
			if l_ok {
				lock := l_val.(*Locks)
				lock.SUnlock(request.TxnID)
			}

		} else{
			response.Value = ""
		}
	}
	kv.Lock()
	kv.stats.gets++
	kv.Unlock()
	return nil
}

func (kv *KVService) Put(request *kvs.PutRequest, response *kvs.PutResponse) error {
	response.Vote=false
	value, found := kv.mp.Load(request.Key);
	if !request.Commit { //phase1
		if found {
			l_val, l_ok := lockMap.Load(request.Key)
			if l_ok {
				lock := l_val.(*Locks)
				if lock.XLock(request.TxnID){
					response.Vote = true
					response.Value = value.(string)
				}
			}

		} else {
			lockMap.Store(request.Key, &Locks{readTxns: make(map[string]string)})
			l_val, l_ok := lockMap.Load(request.Key)
			if l_ok {
				lock := l_val.(*Locks)
				if lock.XLock(request.TxnID) {
					kv.mp.Store(request.Key, "")
					response.Vote = true
				} else {
					lock.XUnlock()
					lockMap.Delete(request.Key)
				}
			}
		}
	} else {//Phase2
		kv.mp.Store(request.Key, request.Value)
		l_val, l_ok := lockMap.Load(request.Key)
			if l_ok {
				lock := l_val.(*Locks)
				lock.XUnlock()
			}
		response.Value = request.Value
		//fmt.Println("PutComitted: ", "Key:", request.Key, "Value:", request.Value)
		kv.Lock()
		kv.stats.puts++
		kv.Unlock()
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
	// mp := kv.mp
	// total, _ := kv.totalValues()
	//kv.printAll()
	kv.Unlock()

	diff := stats.Sub(&prevStats)
	deltaS := now.Sub(lastPrint).Seconds()

	fmt.Printf("get/s %0.2f\nput/s %0.2f\naborts/s %0.2f\nops/s %0.2f\n\n",
		float64(diff.gets)/deltaS,
		float64(diff.puts)/deltaS,
		float64(diff.aborts)/deltaS,
		float64(diff.gets+diff.puts)/deltaS)
	// fmt.Println("Total amount:", total)
}


func (kv *KVService)totalValues() (int, error) {
    total := 0
    var err error

    kv.mp.Range(func(key, value any) bool {
        strVal, ok := value.(string)
        if !ok {
            // not a string, skip or stop
            return true
        }

        num, convErr := strconv.Atoi(strVal)
        if convErr != nil {
            err = convErr
            return false // stop iteration on error
        }

        total += num
		fmt.Println("key:", key, "value:", strVal, "total so far:", total)
        return true
    })

    return total, err
}

func (kv *KVService) printAll() {
    kv.mp.Range(func(key, value any) bool {
        fmt.Printf("key: %v, value: %v\n", key, value)
        return true // keep iterating
    })
}

// func main() {
// 	port := flag.String("port", "8080", "Port to run the server on")
// 	flag.Parse()

// 	kvs := NewKVService()
// 	rpc.Register(kvs)
// 	rpc.HandleHTTP()

// 	l, e := net.Listen("tcp", fmt.Sprintf(":%v", *port))
// 	if e != nil {
// 		log.Fatal("listen error:", e)
// 	}

// 	fmt.Printf("Starting KVS server on :%s\n", *port)
	
// 	go func() {
// 		for {
// 			kvs.printStats()
// 			time.Sleep(1 * time.Second)
// 		}
// 	}()
	
// 	http.Serve(l, nil)
// }

func main() {
	var bankTestCase bool
    port := flag.String("port", "8080", "Port to run the server on")
    flag.BoolVar(&bankTestCase, "banktestcase", false, "Run bank test case")
    flag.Parse()

    kvs := NewKVService()
    rpc.Register(kvs)
    rpc.HandleHTTP()

    l, e := net.Listen("tcp", fmt.Sprintf(":%v", *port))
    if e != nil {
        log.Fatal("listen error:", e)
    }

    fmt.Printf("Starting KVS server on :%s\n", *port)

    // goroutine to print stats every second
    go func() {
        for {
            kvs.printStats()
            time.Sleep(1 * time.Second)
        }
    }()

    // channel to catch OS signals (Ctrl+C, SIGTERM, etc.)
    stop := make(chan os.Signal, 1)
    signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

    // run server in a goroutine
    go func() {
        if err := http.Serve(l, nil); err != nil {
            log.Println("server error:", err)
        }
    }()

    // block until a signal is received
    <-stop
    fmt.Println("\nShutting down server...")

    // call your printAll before exiting
    kvs.printAll()
}


