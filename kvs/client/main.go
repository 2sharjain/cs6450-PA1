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
	return &Client{rpcClients}
}

func (client *Client) Get(key string, target_idx int, txn_id string, phase1 bool) (string, bool) {
	if phase1 {
		request := kvs.GetRequest{
			Key:   key,
			Commit: false,
			TxnID: txn_id,
		}
		response := kvs.GetResponse{}
		cxn := client.rpcClients[target_idx]
		err := cxn.Call("KVService.Get", &request, &response)
		if err != nil {
			log.Fatal(err)
		}

		return response.Value, response.Vote

	} else {
		request := kvs.GetRequest{
			Key:   key,
			Commit: true,
			TxnID: txn_id,
		}
		response := kvs.GetResponse{}
		cxn := client.rpcClients[target_idx]
		err := cxn.Call("KVService.Get", &request, &response)
		if err != nil {
			log.Fatal(err)
		}

		return response.Value, response.Vote
	}
	
}

func (client *Client) Put(key string, value string, target_idx int, txn_id string, phase1 bool) (bool, bool) {


	if phase1 {

		request := kvs.PutRequest{
			Key:   key,
			Value: value,
			Commit: false,
			TxnID: txn_id,
		}
		response := kvs.PutResponse{}
		cxn := client.rpcClients[target_idx]
		err := cxn.Call("KVService.Put", &request, &response)
		if err != nil {
			log.Fatal(err)
		}
		return response.Vote, response.Ack
	} else{
		request := kvs.PutRequest{
			Key:   key,
			Value: value,
			Commit: true,
			TxnID: txn_id,
		}
		response := kvs.PutResponse{}
		cxn := client.rpcClients[target_idx]
		err := cxn.Call("KVService.Put", &request, &response)
		if err != nil {
			log.Fatal(err)
		}
		return response.Vote, response.Ack
	}
}


func (client *Client) Abort(key string, target_idx int, txn_id string, is_read bool) bool {
	request := kvs.AbortRequest{
		Key:   key,
		TxnID: txn_id,
		IsRead: is_read,

	}
	response := kvs.AbortResponse{}
	cxn := client.rpcClients[target_idx]
	err := cxn.Call("KVService.Abort", &request, &response)
	if err != nil {
		log.Fatal(err)
	}

	return response.Ack

	
}


func sendTransaction(client *Client, txn kvs.Transaction, addrs []string, txnstate kvs.TransactionState) {
	//Phase1
	value := strings.Repeat("x", 128)
	if txnstate.States[0]==0 && txnstate.States[1]==0 && txnstate.States[2]==0 {
		for i := 0; i < 3; i++ {
			var vote bool
			key := fmt.Sprintf("%d", txn.Ops[i].Key)
			target_id := kvs.HashKeyMod(key, len(addrs))

			if txn.Ops[i].IsRead {
					_, vote = client.Get(key, target_id, txn.Transaction_id, true)
			} else {
					vote, _ = client.Put(key, value, target_id, txn.Transaction_id, true)
			}
			if vote {
				txnstate.States[i]= 1
			} else {
				txnstate.States[i]= 2
			}
		}

	}
	//abort
	if txnstate.States[0]== 2 || txnstate.States[1]== 2 || txnstate.States[2]== 2 {

		//send rpc call to all servers to abort
		for i := 0; i < 3; i++ {
			key := fmt.Sprintf("%d", txn.Ops[i].Key)
			target_id := kvs.HashKeyMod(key, len(addrs))
			if txnstate.States[i] !=2 {
				client.Abort(key, target_id, txn.Transaction_id, txn.Ops[i].IsRead)
			}
		}
		go sendTransaction(client, txn, addrs, kvs.TransactionState{})
		return
	}
	//Phase2
	//var val string
	if txnstate.States[0]== 1 && txnstate.States[1]== 1 && txnstate.States[2]== 1 {
		for i := 0; i < 3; i++ {
			var key = fmt.Sprintf("%d", txn.Ops[i].Key)
			target_id := kvs.HashKeyMod(key, len(addrs))
			if txn.Ops[i].IsRead {
				_, _ = client.Get(key, target_id, txn.Transaction_id, false)
			} else {
				_, _ = client.Put(key, value, target_id, txn.Transaction_id, false)
			}
		}
	}
}
func runClient(id int, addrs []string, done *atomic.Bool, workload *kvs.Workload, resultsCh chan<- uint64) {

	client := Dial(addrs)
	//value := strings.Repeat("x", 128)
	const batchSize = 1024
	opsCompleted := uint64(0)
	var txn = kvs.Transaction{}

	for !done.Load() {
		for j := 0; j < batchSize; j++ {
			for i := 0; i < 3; i++ {
				txn.Ops[i] = workload.Next()
			}
			txn.Transaction_id, _ = kvs.RandString()
			// client.TxnStateMap[txn.Transaction_id] = kvs.TransactionState{}
			var txnstate = kvs.TransactionState{}
			go sendTransaction(client, txn, addrs, txnstate)
			opsCompleted+=3
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
	workload := flag.String("workload", "YCSB-A", "Workload type (YCSB-A, YCSB-B, YCSB-C)")
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
