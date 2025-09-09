## 2. Design
We began by running the baseline code without modifications, achieving a throughput of approximately 16,000 operations per second on four nodes. To improve performance, we introduced parallelism by launching a separate goroutine for each client request, which increased throughput to around 32,000 on four nodes.

A major improvement came from batching RPC calls: requests were buffered and dispatched together once the batch size was reached, with different goroutines handling the requests concurrently. This optimization raised throughput to roughly 2 million operations per second. The batchqueue is copied to each go routines and the main routine keeps on creating new queues while the spawned routines send out the rpc called and then deallocate the memory from the queues.

On the server side, we replaced the global lock with Go’s sync.Map, enabling concurrent reads and updates on existing keys. This change further improved throughput to about 2.5 million.

Next, we parallelized server processing by assigning worker goroutines to handle batches independently. Additionally, we removed locks on statistics and operation counters, replacing them with atomic functions to eliminate contention. With these combined optimizations, our final median throughput reached approximately 6 million operations per second.

## 3. Reproducability
Configure the cloundlab with 8 nodes and run the command `./run-cluster.sh 1 7`

## 4. Reflection 
We picked up a lot about how RPC works and the kinds of techniques you can use to speed things up in distributed systems. We also got more experience with Go, especially the tricky parts of debugging and testing when you’re running across a cluster. Next time, we think we could do better by using stronger profiling tools and keeping a record of results as we go, so it’s easier to see what changes actually help and what sets us back.
We were stuck for a long while on the resultsCh channel variable. The channel was waiting for opscompleted variable and wasn't getting it from the right place. We also feel there is a lot of room for improvement. We currently only have one server which makes the server the bottleneck. To fix this bottleneck, we could add sharding across servers. The idea is that each client would use a simple hash on the operation’s key to figure out which server should handle it. That way, requests get spread out more evenly and no single server becomes a bottleneck. With the request load balanced across shards, multiple servers can work in parallel, which should give us much higher overall throughput. There might also be room for improvement if we use gRPC instead of HTTP.
All of the team members worked together at the same time in the same room. We brainstormed these ideas together and implemented/debugged the code in rotation.


Team:

Toshit Jain - U1528089

Hima Mynampaty - U1528521

Dhruv Meduri - U1471195

Devanshu Mantri - U1551687
