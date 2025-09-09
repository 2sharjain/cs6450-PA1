package main

import (
	"flag"
	"fmt"
	"log"
	"net/rpc"
	"strings"
	"sync/atomic"
	"time"
	"sync"

	"github.com/rstutsman/cs6450-labs/kvs"
)

type Client struct {
	rpcClient *rpc.Client
}

func Dial(addr string) *Client {
	rpcClient, err := rpc.DialHTTP("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	return &Client{rpcClient}
}

func (client *Client) Get(getQueue *[]kvs.GetRequest) *[]kvs.GetResponse {
	
	response := make([]kvs.GetResponse, 0)
	err := client.rpcClient.Call("KVService.Get", &getQueue, &response)
	if err != nil {
		log.Fatal(err)
	}

	return &response
}

func (client *Client) Put(putQueue *[]kvs.PutRequest) {
	// request := kvs.PutRequest{
	// 	Key:   key,
	// 	Value: value,
	// }
	response := kvs.PutResponse{}
	err := client.rpcClient.Call("KVService.Put", &putQueue, &response)
	if err != nil {
		log.Fatal(err)
	}
}

func runClient(id int, addr string, done *atomic.Bool, workload *kvs.Workload, resultsCh chan<- uint64) {
	var mu sync.Mutex
	client := Dial(addr)

	value := strings.Repeat("x", 128)
	const numThreads = 1024
	const batchSize = 32

	opsCompleted := uint64(0)

	getQueue := make([]kvs.GetRequest, 0, batchSize)
	putQueue := make([]kvs.PutRequest, 0, batchSize)
	for !done.Load() {
		for j := 0; j < numThreads; j++ {
			op := workload.Next()
			key := fmt.Sprintf("%d", op.Key)
			if op.IsRead {
				getQueue = append(getQueue, kvs.GetRequest{Key: key})
				if(len(getQueue) == batchSize){
					batch := make([]kvs.GetRequest, batchSize)
    				copy(batch, getQueue)
					go func(){
						client.Get(&batch) //Change the parameter to queue
						//getQueue = nil
						mu.Lock()
						opsCompleted+=batchSize
						mu.Unlock()
					}()
					getQueue = make([]kvs.GetRequest, 0, batchSize)			
			} else {
				putQueue = append(putQueue, kvs.PutRequest{Key: key, Value: value})
				if  len(putQueue) == batchSize {
					batch := make([]kvs.PutRequest, batchSize)
					copy(batch, putQueue)
					go func(){
						client.Put(&batch) // fix it
						//putQueue = nil
						mu.Lock()
						opsCompleted+=batchSize
						mu.Unlock()
					}()
					putQueue = make([]kvs.PutRequest, 0, batchSize)	
				}
			}
		}
	}
		fmt.Printf("Client %d finished operations.\n", id)
		resultsCh <- opsCompleted
	}
}

// func runClient(id int, addr string, done *atomic.Bool, workload *kvs.Workload, resultsCh chan<- uint64) {
// 	client := Dial(addr)

// 	value := strings.Repeat("x", 128)
// 	const numThreads = 1024
// 	const batchSize = 64

// 	opsCompleted := uint64(0)
// 	getQueue := [batchSize]GetRequest{}
// 	putQueue := [batchSize]PutRequest{}

//     // item := queue[0]
//     // queue = queue[1:]
// 	for !done.Load() {
// 		for j := 0; j < numThreads; j++ {
// 			op := workload.Next()
// 			key := fmt.Sprintf("%d", op.Key)

// 			if op.IsRead {
// 				getQueue = append(getQueue, GetRequest{Key: key})
// 				if( len(getQueue) == batchSize ) {

// 					go func(){
// 					client.Get(key) //Change the parameter to queue
// 				}()

// 				}
				
// 			} else {
// 				putQueue = append(putQueue, PutRequest{Key: key, Value: value})
// 				go func(){
// 					client.Put(key, value)
// 				}()
// 			}
// 			opsCompleted++
// 		}
// 	}

// 	fmt.Printf("Client %d finished operations.\n", id)

// 	resultsCh <- opsCompleted
// }

type HostList []string

func (h *HostList) String() string {
	return strings.Join(*h, ",")
}

func (h *HostList) Set(value string) error {
	*h = strings.Split(value, ",")
	return nil
}

func main() {
	hosts := HostList{}

	flag.Var(&hosts, "hosts", "Comma-separated list of host:ports to connect to")
	theta := flag.Float64("theta", 0.99, "Zipfian distribution skew parameter")
	workload := flag.String("workload", "YCSB-B", "Workload type (YCSB-A, YCSB-B, YCSB-C)")
	secs := flag.Int("secs", 30, "Duration in seconds for each client to run")
	flag.Parse()

	if len(hosts) == 0 {
		hosts = append(hosts, "localhost:8080")
	}

	fmt.Printf(
		"hosts %v\n"+
			"theta %.2f\n"+
			"workload %s\n"+
			"secs %d\n",
		hosts, *theta, *workload, *secs,
	)

	start := time.Now()

	done := atomic.Bool{}
	resultsCh := make(chan uint64)

	host := hosts[0]
	clientId := 0
	go func(clientId int) {
		workload := kvs.NewWorkload(*workload, *theta)
		runClient(clientId, host, &done, workload, resultsCh)
	}(clientId)

	time.Sleep(time.Duration(*secs) * time.Second)
	done.Store(true)

	opsCompleted := <-resultsCh

	elapsed := time.Since(start)

	opsPerSec := float64(opsCompleted) / elapsed.Seconds()
	fmt.Printf("throughput %.2f ops/s\n", opsPerSec)
}
