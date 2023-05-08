package verify

import (
	"context"
	"fmt"

	"github.com/flashbots/go-boost-utils/bls"
	"github.com/lthibault/log"
	"github.com/prometheus/client_golang/prometheus"
)

type VerificationManager struct {
	VerifySubmitBlockCh       chan Request
	VerifyRegisterValidatorCh chan Request
	VerifyOtherCh             chan Request

	l log.Logger

	m ProcessManagerMetrics
}

func NewVerificationManager(l log.Logger, verifySize uint) *VerificationManager {
	rm := &VerificationManager{
		l: l,

		VerifySubmitBlockCh:       make(chan Request, verifySize),
		VerifyRegisterValidatorCh: make(chan Request, verifySize),
		VerifyOtherCh:             make(chan Request, verifySize),
	}
	rm.initMetrics()
	return rm
}

func (rm *VerificationManager) RunVerify(num uint) {
	for i := uint(0); i < num; i++ {
		go rm.VerifyParallel()
	}
}

func (rm *VerificationManager) VerifyChan() chan Request {
	return rm.VerifyOtherCh
}

func (rm *VerificationManager) GetVerifyChan(stack uint) chan Request {
	switch stack {
	case ResponseQueueSubmit:
		return rm.VerifySubmitBlockCh
	case ResponseQueueRegister:
		return rm.VerifyRegisterValidatorCh
	default: // ResponseQueueOther
		return rm.VerifyOtherCh
	}
}

func (rm *VerificationManager) Enqueue(ctx context.Context, sig [96]byte, pubkey [48]byte, msg [32]byte) (err error) {
	respChA := NewRespC(1)
	rm.GetVerifyChan(ResponseQueueSubmit) <- Request{
		Signature: sig,    //submitBlockRequest.Signature,
		Pubkey:    pubkey, // submitBlockRequest.Message.BuilderPubkey,
		Msg:       msg,
		Response:  respChA}

	select {
	case err = <-respChA.Done():
	case <-ctx.Done():
		err = ctx.Err()
		respChA.Close(0, err)

	}
	return err
}

func (rm *VerificationManager) VerifyParallel() {
	rm.m.RunningWorkers.WithLabelValues("VerifyParallel").Inc()
	defer rm.m.RunningWorkers.WithLabelValues("VerifyParallel").Dec()

	timerA := rm.m.VerifyTiming.WithLabelValues("submitBlock")
	timerB := rm.m.VerifyTiming.WithLabelValues("registerValidator")
	timerC := rm.m.VerifyTiming.WithLabelValues("other")

	// Few words of explanation:
	// Because Relay can only process a limitted number of signatures, it would operate on 3 different channels.
	// This way we'll randomly pick next from different queues and huge number of request
	// would not bring other from being processed.
	//
	// The 3 stacks are meant for (in order of signifficance, however processed IN RANDOM ORDER):
	// submitting blocks, registering validators and others.
	for {
		select {
		case v := <-rm.VerifySubmitBlockCh:
			_ = verifyCheck(timerA, v)
		case v := <-rm.VerifyRegisterValidatorCh:
			_ = verifyCheck(timerB, v)
		case v := <-rm.VerifyOtherCh:
			_ = verifyCheck(timerC, v)
		}
	}
}

func verifyCheck(o prometheus.Observer, v Request) (err error) {
	defer func() { // better safe than sorry
		if r := recover(); r != nil {
			var isErr bool
			err, isErr = r.(error)
			if !isErr {
				err = fmt.Errorf("verify signature bytes panic: %v", r)
			}
		}
	}()

	if v.Response.IsClosed() {
		return nil
	}
	t := prometheus.NewTimer(o)
	defer t.ObserveDuration()

	v.Response.Send(verifyUnit(v.ID, v.Msg, v.Signature, v.Pubkey))
	return err

}
func verifyUnit(id int, msg [32]byte, sigBytes [96]byte, pkBytes [48]byte) Resp {
	ok, err := VerifySignatureBytes(msg, sigBytes[:], pkBytes[:])
	if err == nil && !ok {
		err = bls.ErrInvalidSignature
	}
	return Resp{Err: err, ID: id}
}
