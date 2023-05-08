package verify

import (
	"sync"
	"sync/atomic"
)

const (
	ResponseQueueSubmit = iota
	ResponseQueueRegister
	ResponseQueueOther
)

// VerifyReq is a request structure used in communication
// between api calls and fixed set of worker goroutines
// it's using return channel pattern, meaning that after
// sent the sender locks on that channel to get the response
type Request struct {
	Signature [96]byte
	Pubkey    [48]byte
	Msg       [32]byte
	// Unique identifier of payload
	// if needed to be passed back in response
	ID       int
	Response *StoreResp
}

// Resp response structure
// - potential candidate for structure pool
// as it's almost constant size
type Resp struct {
	ID     int
	Type   int8
	Commit bool
	Err    error
}

type StoreResp struct {
	nonErrors []int64
	numAll    int

	rLock    sync.Mutex
	isClosed int32
	err      error

	done chan error
}

func NewRespC(numAll int) (s *StoreResp) {
	return &StoreResp{
		numAll: numAll,
		done:   make(chan error, 1),
	}
}

func (s *StoreResp) SuccessfullIndexes() []int64 {
	return s.nonErrors
}

func (s *StoreResp) Done() chan error {
	return s.done
}

func (s *StoreResp) IsClosed() bool {
	return atomic.LoadInt32(&(s.isClosed)) != 0
}

func (s *StoreResp) Send(r Resp) {
	s.rLock.Lock()
	defer s.rLock.Unlock()

	if s.IsClosed() {
		return
	}

	if r.Err != nil {
		s.err = r.Err
		s.close()
		return
	}

	s.nonErrors = append(s.nonErrors, int64(r.ID))
	if s.numAll == len(s.nonErrors) {
		s.close()
		return
	}
}

func (s *StoreResp) Error() (err error) {
	return s.err
}

func (s *StoreResp) Close(id int, err error) {
	s.rLock.Lock()
	defer s.rLock.Unlock()

	if err != nil {
		s.err = err
	}
	s.close()
}

func (s *StoreResp) SkipOne() {
	s.rLock.Lock()
	defer s.rLock.Unlock()

	if s.IsClosed() {
		return
	}

	s.numAll -= 1
	if s.numAll == len(s.nonErrors) {
		s.close()
		return
	}
}

func (s *StoreResp) close() {
	if !s.IsClosed() {
		atomic.StoreInt32(&(s.isClosed), int32(1))
		close(s.done)
	}
}
