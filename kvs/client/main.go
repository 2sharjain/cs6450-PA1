package main

import (
	"flag"
	"fmt"
	"log"
	"net/rpc"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rstutsman/cs6450-labs/kvs"
)

type Client struct {
	rpcClients  []*rpc.Client
	TxnStateMap map[string]kvs.TransactionState
}

func Dial(addrs []string) *Client {
	rpcClients := make([]*rpc.Client, len(addrs))
	var err error
	for i := 0; i < len(addrs); i++ {
		rpcClients[i], err = rpc.DialHTTP("tcp", addrs[i])
		if err != nil {
			log.Fatal(err)
		}
	}
	return &Client{rpcClients, make(map[string]kvs.TransactionState)}
}

func (client *Client) Get(key string, target_idx int) string {
	request := kvs.GetRequest{
		Key: key,
	}
	response := kvs.GetResponse{}
	cxn := client.rpcClients[target_idx]
	err := cxn.Call("KVService.Get", &request, &response)
	if err != nil {
		log.Fatal(err)
	}

	return response.Value
}

func (client *Client) Put(key string, value string, target_idx int) {
	request := kvs.PutRequest{
		Key:   key,
		Value: value,
	}
	response := kvs.PutResponse{}
	cxn := client.rpcClients[target_idx]
	err := cxn.Call("KVService.Put", &request, &response)
	if err != nil {
		log.Fatal(err)
	}
}

func sendTransaction(client *Client, txn kvs.Transaction, addrs []string) {
	for i := 0; i < 3; i++ {
		var key = fmt.Sprintf("%d", txn.Ops[i].Key)
		target_id := kvs.HashKeyMod(key, len(addrs))
		if txn.Ops[i].IsRead {
			go func() {
				client.Get(key, target_id)
			}()
		} else {
			go func() {
				client.Put(key, value, target_id)
			}()
		}
	}
}
func runClient(id int, addrs []string, done *atomic.Bool, workload *kvs.Workload, resultsCh chan<- uint64) {

	client := Dial(addrs)
	value := strings.Repeat("x", 128)
	const batchSize = 1024
	opsCompleted := uint64(0)
	var txn = kvs.Transaction{}

	for !done.Load() {
		for j := 0; j < batchSize; j++ {
			for i := 0; i < 3; i++ {
				txn.Ops[i] = workload.Next()
			}
			txn.Transaction_id, _ = kvs.RandString()
			client.TxnStateMap[txn.Transaction_id] = kvs.TransactionState{}
			go sendTransaction(client, txn, addrs)
			opsCompleted++
		}
	}

	fmt.Printf("Client %d finished operations.\n", id)

	resultsCh <- opsCompleted
}

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
	//numHosts := len(hosts)
	//host := hosts[0]
	clientId := 0
	go func(clientId int) {
		workload := kvs.NewWorkload(*workload, *theta)
		runClient(clientId, hosts, &done, workload, resultsCh)
	}(clientId)

	time.Sleep(time.Duration(*secs) * time.Second)
	done.Store(true)

	opsCompleted := <-resultsCh

	elapsed := time.Since(start)

	opsPerSec := float64(opsCompleted) / elapsed.Seconds()
	fmt.Printf("throughput %.2f ops/s\n", opsPerSec)
}
