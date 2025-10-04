## 1. Results
get/s is the number of get calls that are committed per server
put/s is the number of puts calls that are committed per server
abort/s is the number of aborted ops.
ops/s is the sum of get/s and put/s
So, on average number of transactions per second is (ops/s) / 3 * number of servers

### YCSB- B runs

#### theta = 0.99, 2 servers, 2 clients
get/s 75366.41
put/s 666.88
aborts/s 4288.23
ops/s 76033.29
tran/s 50688.43

#### theta = 0.99, 3 servers, 1 clients
get/s 39525.86
put/s 325.95
aborts/s 2845.56
ops/s 39851.81
tran/s 39851.81

#### theta = 0.99, 1 servers, 3 clients
get/s 97664.38
put/s 393.97
aborts/s 4688.68
ops/s 98058.35
tran/s 32686.1


#### theta = 0, 2 servers, 2 clients
get/s 79060.25
put/s 1498.68
aborts/s 508.89
ops/s 80558.94
tran/s 53705.3

#### theta = 0.5, 2 servers, 2 clients
get/s 78817.68
put/s 1296.81
aborts/s 1595.77
ops/s 80114.49
tran/s 53409.6


### Bank test case
Our bank testcase has 10 clients, each starting with $1000. Each transaction moves $100 (if available) across consecutive accounts.
This test case always preserves the total amount of money = $10000, hence the checking the serializability. We do see patterns in the money transfer. Most of the times a bunch of accounts end up with most of the money, but some of the times the money i kind of evenly distributed(rarely).

#### 2 servers, 2 clients
get/s 367.69
put/s 45.96
aborts/s 271.77
ops/s 413.65
tran/s 275.7


#### 3 servers, 1 clients
get/s 130.99
put/s 22.00
aborts/s 111.99
ops/s 152.99
tran/s 153.43

#### 1 servers, 3 clients
get/s 350.98
put/s 247.99
aborts/s 75.00
ops/s 598.96
tran/s 198.2



## 2. Design
We decided to with the 2PC/2PL protocol. To implement 2PC we created a Transaction struct that held the transaction ID (later used for 2PL), the 3 operations which make up the transaction and the clientID. We held the state of the transaction in a TransactionState that held 3 integers for the respective operations. The TransactionState integers can be 3 values: 
0: Put/Get request was not sent (default).
1: the transaction holds the necessary locks across the shards to perform the transaction. (vote yes)
2: the transaction failed to acquire the locks necessary. (vote no)

### 2PC
These states allow for the 2 seperate phases for 2PC. Initially there is a default state before even calling the Get/Put RPCs, after the RPCs are sent in the first phase the server sets the state to either 1 or 2 for it's Get/Put RPC based on whether it can aquire the lock necessary for the operation. 
These values are then used for a vote on whether the client should call the Put/Get RPCs again to commit the transaction or whether it should call the Abort() RPC on the server. 
To signify the commit to the respective Get/Put RPC in the server a flag named commit is set in the Put/Get request structure. 
The Abort() RPC on the server unlocks any locks that were aquired.

### 2PL
To Implement 2-phase locking in the server we had a lock manager that holds the the reader/writer's transaction IDs on a given key. 
The lock manager ensures that a read and write cannot happen at the same time. The lock manager is implemented as a sync map allowing concurrent access to add readers/writters when adding a new key. KVService struct holds the sharded hash map, which is also implemented as a sync map as it allows concurrent access to keys when adding an entry to the hashmap.We allow for an operation to acquire a write lock if one of these is true, ON A PARTICULAR KEY: 
1. There is no writer or reader on that key
2. There is no writer, and there is only one reader on that key, and the reader has the same transaction id as the current transaction id
3. There is only on writer on that key, but has the same transaction id a the current transaction id


## 3. Reproducability
Configure the cloudlab with 4 nodes and run the commands:
 `./run-cluster.sh 2 2` to run the YCSB-B workload
 `./run-cluster.sh 2 2 --banktestcase` to run the bank transaction testcase
 
## 4. Reflection

### YCSB-B case
    From the results reported, we can notice that the aborts/sec falls as the theta reduces. This happens becasue reducing the theta gives generates more unifromly distributed keys, thus resulting in lesser lock contention on the same keys. An interesting observation is that the maximum throughput occurs when there are 2 servers and 2 clients. This configuration is the most balanced. Increasing the number of servers reduces the number of clients, which lowers the total number of transactions generated. Conversely, reducing the number of servers increases contention, which also decreases throughput.

### The Bank testcase
    When we ran the bank testcase we were getting a lot of abort. It was bacause for a small number of keys(0-9 bank clients), they were contending for the same lockmap, which resulted in most of the puts aborting. So we limited the number of active goroutines to ~100, and that solved the problem. Our implementation is not very good for sending out such a large number of put and get request on such a less number of keys. So in this case we had to sacrifice the througput. We see a lot of puts go through at the beginning of the run, but as lock contention becomes an issue, the aborts accumlates and the put/s decreases.

### LockManager

    We learned about the difficulties in implementing latches (the lock manager) in a sharded database. When there are many transactions that share the same keys having a single lock manager lead to a lot of contention because it is necessary to aquire the lock on the key within the lock manager to unlock the lock for a given key, leading to lower throughputs for put operations as they hold exclusive locks and need to unlock before any transactions can read. In future implementations having a custom data structure or having multiple lock managers may be useful, especially when there are many transactions on a small number of keys.

### Initial Implementations

    Initially we only had a single mutex in our Locks structure that makes up the entries in our LockManager. This was an incorrect implementation as it would not allow a transaction to upgrade from a reader/shared lock to a writer/exclusive lock. This causes transactions that have a read on a key before a put on the same key (ex. Get('A'), Put('A', val), ...) to abort without ever completing.

### Further Improvements

    Next time we would add a way to prioritize aquiring the lock to unlock the lockmanager as this would help alleviate the contention issues when a large amount of threads/transactions are accessing the same keys.


Team:

Toshit Jain - U1528089

Hima Mynampaty - U1528521

Dhruv Meduri - U1471195

Devanshu Mantri - U1551687 