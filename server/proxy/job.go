package proxy

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"github.com/swordlet/xmrig2xdag/logger"
	"math"
)

const (
	initNonceOffset = 56
	initNonceLength = 8

	nonceOffset = 60
	nonceLength = 4 // bytes

	// TODO - worker could supply expected hashes?
	nonceIncrement = 0x02000000 // 32M, not really expected, just plenty of work
	maxNonceValue  = math.MaxUint32 - nonceIncrement
)

var (
	ErrMalformedJob        = errors.New("bad job format from pool")
	ErrUnknownTargetFormat = errors.New("unrecognized format for job target")
)

// Job is a mining job.  Break it up and send chunks to workers.
type Job struct {
	Blob     string `json:"blob"`
	ID       string `json:"job_id"`
	Target   string `json:"target"`
	SeedHash string `json:"seed_hash"`
	Algo     string `json:"algo"`

	//submittedNonces []string `json:"-"`
	initialNonce uint32 `json:"-"`
	currentBlob  []byte `json:"-"`
	currentNonce uint32 `json:"-"`
}

//// NewJobFromServer creates a Job from a pool notice
//func NewJobFromServer(job map[string]interface{}) (*Job, error) {
//	j := &Job{}
//	var ok bool
//	if j.Blob, ok = job["blob"].(string); !ok {
//		return nil, ErrMalformedJob
//	}
//	if j.ID, ok = job["job_id"].(string); !ok {
//		return nil, ErrMalformedJob
//	}
//	if j.Target, ok = job["target"].(string); !ok {
//		return nil, ErrMalformedJob
//	}
//	if j.SeedHash, ok = job["seed_hash"].(string); !ok {
//		return nil, ErrMalformedJob
//	}
//
//	if err := j.init(); err != nil {
//		return nil, err
//	}
//
//	return j, nil
//}

//func (j *Job) init() error {
//	currentNonce, currentBlob, err := j.Nonce()
//	if err != nil {
//		return err
//	}
//	//j.submittedNonces = make([]string, 0)
//	j.currentNonce = currentNonce
//	j.initialNonce = currentNonce
//	j.currentBlob = currentBlob
//
//	return nil
//}

// Next returns the next version of this job for worker distribution
// and increments the nonce
func (j *Job) Next() *Job {

	j.currentNonce += nonceIncrement
	if j.currentNonce >= maxNonceValue {
		j.currentNonce = 1
	}

	nextJob := &Job{
		ID:           j.ID,
		Target:       j.Target,
		SeedHash:     j.SeedHash,
		Algo:         xdagAlgo,
		initialNonce: j.initialNonce,
		currentNonce: j.currentNonce,
		currentBlob:  make([]byte, 64),
	}

	nonceBytes := make([]byte, nonceLength, nonceLength)
	binary.BigEndian.PutUint32(nonceBytes, j.currentNonce)

	copy(nextJob.currentBlob[:], j.currentBlob[:])
	copy(nextJob.currentBlob[nonceOffset:nonceOffset+nonceLength], nonceBytes)
	nextJob.Blob = hex.EncodeToString(nextJob.currentBlob)

	logger.Get().Println("next, job blob: ", nextJob.Blob, ", nonce: ", hex.EncodeToString(nonceBytes))
	return nextJob
}

//
//// NewJob builds a job for distribution to a worker
//func NewJob(blobBytes []byte, nonce uint32, id, target string) *Job {
//	j := &Job{
//		ID:     id,
//		Target: target,
//		//submittedNonces: make([]string, 0),
//	}
//	nonceBytes := make([]byte, nonceLength, nonceLength)
//	binary.BigEndian.PutUint32(nonceBytes, nonce)
//	copy(blobBytes[nonceOffset:nonceOffset+nonceLength], nonceBytes)
//	j.Blob = hex.EncodeToString(blobBytes)
//
//	return j
//}

//// Nonce extracts the nonce from the job blob and returns it.
//func (j *Job) Nonce() (nonce uint32, blobBytes []byte, err error) {
//	blobBytes, err = hex.DecodeString(j.Blob)
//	if err != nil {
//		return
//	}
//
//	nonceBytes := blobBytes[nonceOffset : nonceOffset+nonceLength]
//	nonce = binary.BigEndian.Uint32(nonceBytes)
//
//	return
//}

// can we count on uint32 hex targets?
// NOT WORKING PROPERLY
func (j *Job) getTargetUint64() (uint64, error) {
	target := j.Target
	if len(target) == 8 {
		target = "00000000" + target
	}
	if len(target) != 16 {
		logger.Get().Println("Job target format is : ", target)
		return 0, ErrUnknownTargetFormat
	}
	targetBytes, err := hex.DecodeString(target)
	if err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint64(targetBytes), nil
}

func createFakeJob() *Job {
	b := make([]byte, 64)
	var blob string
	if _, err := rand.Read(b); err != nil {
		blob = "070780e6b9d60586ba419a0c224e3c6c3e134cc45c4fa04d8ee2d91c2595463c57eef0a4f0796c000000002fcc4d62fa6c77e76c30017c768be5c61d83ec9d3a"
	}
	blob = hex.EncodeToString(b)
	//fmt.Println(blob)
	return &Job{ // return a fake job before proxy connect XDAG pool
		ID:       "FFFFFFFFFF" + NewLen(18),
		Target:   "b88d0600", //difficulty = 10000
		Algo:     xdagAlgo,
		Blob:     blob,
		SeedHash: "e1364b8782719d7683e2ccd3d8f724bc59dfa780a9e960e7c0e0046acdb40100",
	}
}
