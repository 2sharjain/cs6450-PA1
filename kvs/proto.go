package kvs

import (
	"crypto/rand"
	"hash/fnv"
	"math/big"
)

// Transaction represents a client transaction. Fields are kept package-private
// (lowercase) to match the existing code's intent.
type Transaction struct {
	Transaction_id string
	Ops            [3]WorkloadOp
	Client_id      int
}

// 0 unsent/waiting
// 1 yes
// 2 no
// 3 waiting 2
type TransactionState struct {
	States [3]int
}

type AbortRequest struct {
	Key string
	TxnID string
	IsRead bool // transaction id
}

type AbortResponse struct {
	Ack bool // true for yes, false for no
}

type PutRequest struct {
	Key   string
	Value string
	Commit bool 
	TxnID string// true for commit, false for abort
}

type PutResponse struct {
	Value string // we populate this only for phase 1
	Vote bool // true for yes, false for no
}

type GetRequest struct {
	Key string
	Commit bool // true for commit, false for abort
	TxnID string // transaction id
}

type GetResponse struct {
	Value string //This is the value
	Vote bool   // true for yes, false for no
}

// HashKey returns the 64-bit FNV-1a hash of the supplied string.
// Use this for consistent hashing of keys.
func HashKey(s string) uint64 {
	h := fnv.New64a()
	// Write can never fail for bytes.Buffer-like hash implementations, ignore error.
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// HashKeyMod returns HashKey(s) modulo n as an int. If n <= 0 it returns 0.
// Useful for mapping keys to a fixed number of partitions.
func HashKeyMod(s string, n int) int {
	if n <= 0 {
		return 0
	}
	return int(HashKey(s) % uint64(n))
}

func RandString() (string, error) {
	n := 16
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	out := make([]byte, n)
	max := big.NewInt(int64(len(letters)))
	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = letters[num.Int64()]
	}
	return string(out), nil
}
