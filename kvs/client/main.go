package main

import (
	"flag"
	"fmt"
	"log"
	"net/rpc"
	"strings"
	"sync/atomic"
	"time"
	"math/rand"
	"strconv"

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

func (client *Client) Put(key string, value string, target_idx int, txn_id string, phase1 bool) (string, bool) {


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
		return response.Value, response.Vote
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
		return response.Value, response.Vote
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
	if txnstate.States[0]==0 && txnstate.States[1]==0 && txnstate.States[2]==0 {
		for i := 0; i < 3; i++ {
			var vote bool
			key := fmt.Sprintf("%d", txn.Ops[i].Key)
			target_id := kvs.HashKeyMod(key, len(addrs))

			if txn.Ops[i].IsRead {
					_, vote = client.Get(key, target_id, txn.Transaction_id, true)
			} else {
					_, vote = client.Put(key, txn.Ops[i].Value, target_id, txn.Transaction_id, true)
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
	var val string
	if txnstate.States[0]== 1 && txnstate.States[1]== 1 && txnstate.States[2]== 1 {
		for i := 0; i < 3; i++ {
			var key = fmt.Sprintf("%d", txn.Ops[i].Key)
			target_id := kvs.HashKeyMod(key, len(addrs))
			if txn.Ops[i].IsRead {
				_, _ = client.Get(key, target_id, txn.Transaction_id, false)
			} else {
				val, _ = client.Put(key, txn.Ops[i].Value, target_id, txn.Transaction_id, false)
			}
			fmt.Println("key:", key, "value:", val)
		}

	}
}





func sendTransaction_bankLoad(client *Client, txn kvs.Transaction, addrs []string, txnstate kvs.TransactionState) {
	//Phase1
	if txnstate.States[0]==0 && txnstate.States[1]==0 && txnstate.States[2]==0 {
		fmt.Println("Entering phase 1, we have", txn.Ops, txnstate )

		var vote bool
		var credit_val string
		var debit_val string
		key := fmt.Sprintf("%d", txn.Ops[0].Key)
		target_id := kvs.HashKeyMod(key, len(addrs))
		debit_val, vote = client.Get(key, target_id, txn.Transaction_id, true)
		debit_int, _ := strconv.ParseUint(debit_val, 10, 64)
		if vote && debit_int >= uint64(100) {
			txnstate.States[0]= 1 
			txn.Ops[1].Value = fmt.Sprintf("%d", uint64(debit_int)-uint64(100))
		}else{
			txnstate.States[0]= 2
		}

		key = fmt.Sprintf("%d", txn.Ops[1].Key)
		target_id = kvs.HashKeyMod(key, len(addrs))
		_ , vote = client.Put(key, txn.Ops[1].Value , target_id, txn.Transaction_id, true)
		if vote{
			txnstate.States[1]= 1 
		}else{
			txnstate.States[1]= 2
		}

		key = fmt.Sprintf("%d", txn.Ops[2].Key)
		target_id = kvs.HashKeyMod(key, len(addrs))
		credit_val, vote = client.Put(key, txn.Ops[2].Value, target_id, txn.Transaction_id, true)
		credit_int, _ := strconv.ParseUint(credit_val, 10, 64)
		txn.Ops[2].Value = fmt.Sprintf("%d", uint64(credit_int)+uint64(100))


		if vote{
			txnstate.States[2]= 1 
		}else{
			txnstate.States[2]= 2
		}
		fmt.Println("exiting phase 1, we have", txn.Ops, txnstate)

	}
	//fmt.Println("after phase 1, we have", txn, txnstate)

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
	if txnstate.States[0]== 1 && txnstate.States[1]== 1 && txnstate.States[2]== 1 {
		//println("Committing transaction")
		fmt.Println("Entering phase 2, we have", txn.Ops, txnstate)
		for i := 0; i < 3; i++ {
			var key = fmt.Sprintf("%d", txn.Ops[i].Key)
			target_id := kvs.HashKeyMod(key, len(addrs))
			if txn.Ops[i].IsRead {
				_, _ = client.Get(key, target_id, txn.Transaction_id, false)
			} else {
				_, _ = client.Put(key, txn.Ops[i].Value, target_id, txn.Transaction_id, false)
			}
		}
		fmt.Println("exiting phase 2, we have", txn.Ops, txnstate)

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



func runBankClient(id int, addrs []string, done *atomic.Bool, resultsCh chan<- uint64) {

	client := Dial(addrs)
	const batchSize = 1024
	opsCompleted := uint64(0)
	var txn = kvs.Transaction{}

// putting the 10 bank accounts

	for i := 0; i < 4; i++ {
		txn.Transaction_id, _ = kvs.RandString()
		txn.Client_id = id
		for j := 0; j < 3; j++ {
			key := uint64((i*3 + j) % 10)
			txn.Ops[j] = kvs.WorkloadOp{Key: key, IsRead: false, Value: "1000" }
		}
		
		sendTransaction(client, txn, addrs, kvs.TransactionState{})
	}
	// our banks are setup
	for !done.Load() {
		for j := 0; j < batchSize; j++ {
			txn.Transaction_id, _ = kvs.RandString()
			txn.Client_id = id
			debitId := uint64(rand.Intn(10))
			txn.Ops[0] = kvs.WorkloadOp{Key: debitId, IsRead: true, Value: "" }
			creditId := (debitId + 1) % 10
			txn.Ops[1] = kvs.WorkloadOp{Key: debitId, IsRead: false, Value: "" }
			txn.Ops[2] = kvs.WorkloadOp{Key: creditId, IsRead: false, Value: "" }
			// client.TxnStateMap[txn.Transaction_id] = kvs.TransactionState{}
			var txnstate = kvs.TransactionState{}
			go sendTransaction_bankLoad(client, txn, addrs, txnstate)
			opsCompleted+=3
		}
	}




	fmt.Printf("Client %d finished operations.\n", id)

	resultsCh <- opsCompleted


	return


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

	banktestcase := true
	if banktestcase {
		hosts := HostList{}

		flag.Var(&hosts, "hosts", "Comma-separated list of host:ports to connect to")
		theta := flag.Float64("theta", 0.99, "Zipfian distribution skew parameter")
		workload := flag.String("workload", "YCSB-A", "Workload type (YCSB-A, YCSB-B, YCSB-C)") //new workload
		secs := flag.Int("secs", 10, "Duration in seconds for each client to run")
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
			runBankClient(clientId, hosts, &done, resultsCh) //new client
		}(clientId)

		time.Sleep(time.Duration(*secs) * time.Second)
		done.Store(true)

		opsCompleted := <-resultsCh

		elapsed := time.Since(start)

		opsPerSec := float64(opsCompleted) / elapsed.Seconds()
		fmt.Printf("throughput %.2f ops/s\n", opsPerSec)
		return
	}







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
