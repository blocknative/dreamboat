package data

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/blocknative/dreamboat/beacon"
	"github.com/blocknative/dreamboat/structs"
	"github.com/lthibault/log"
)

type ExportService struct {
	logger   log.Logger
	requests chan exportRequest
	datadir  string
}

func NewExportService(logger log.Logger, datadir string, bufSize int) ExportService {
	return ExportService{logger: logger, datadir: datadir, requests: make(chan exportRequest, bufSize)}
}

func (s ExportService) RunParallel(ctx context.Context, numWorkers int) error {
	logger := s.logger.WithField("service", "data-exporter")

	datadir := fmt.Sprintf("%s/blockBidAndTrace", s.datadir)
	if err := os.MkdirAll(datadir, 0755); err != nil {
		return fmt.Errorf("failed to create datadir: %w", err)
	}

	for i := 0; i < numWorkers; i++ {
		files := exportFiles{}
		// create files
		// // BlockBidAndTrace
		filename := fmt.Sprintf("%s/blockBidAndTrace/output_%d.json", s.datadir, i)
		file, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}
		files.BlockBidAndTrace = file

		go func(ctx context.Context, i int, files exportFiles) {
			defer files.BlockBidAndTrace.Close()

			encs := exportEncoders{}
			buffers := exportBuffers{}

			// init buffers
			buffers.BlockBidAndTrace = bufio.NewWriterSize(files.BlockBidAndTrace, 10*2060) // TODO: 2060B = 20Kb is the expected capella payload size. Bufio is a performance optimization for reducing disk writes
			go func(ctx context.Context, buffers exportBuffers) {
				ticker := time.NewTicker(beacon.DurationPerSlot)
				for {
					select {
					case <-ticker.C:
						buffers.BlockBidAndTrace.Flush()
					case <-ctx.Done():
						buffers.BlockBidAndTrace.Flush()
						return
					}
				}
			}(ctx, buffers)

			// init encoders
			encs.BlockBidAndTrace = json.NewEncoder(buffers.BlockBidAndTrace)
			encs.BlockBidAndTrace.SetIndent("", "") //  the output JSON will be written on a single line without any whitespace: '{"field1":"value1","field2":{"nested1":"value2","nested2":"value3"}}'

			s.Run(ctx, logger.WithField("worker", i), encs)
		}(ctx, i, files)
	}

	return nil
}

func (s ExportService) Run(ctx context.Context, logger log.Logger, encs exportEncoders) {
	logger.Info("started")
	defer logger.Info("stopped")

	for {
		select {
		case req := <-s.requests:
			enc, err := selectEncoder(req, encs)
			if err != nil {
				logger.WithError(err).Error("failed to export request")
			}

			data := dataWithCaller{Data: req.data, Caller: req.caller}
			select {
			case req.err <- enc.Encode(data): // does not block because it is buffered (1) channel, but better safe than sorry
			case <-ctx.Done():
				logger.WithError(ctx.Err()).Error("failed to export request")
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s ExportService) SubmitBlockBidAndTrace(ctx context.Context, bbt structs.BlockBidAndTrace, caller string) error {
	request := exportRequest{dt: BlockBidAndTraceData, data: bbt, caller: caller, err: make(chan error, 1)}

	// submit request
	select {
	case s.requests <- request:
	case <-ctx.Done():
		return ctx.Err()
	}

	// wait for response
	select {
	case err := <-request.err:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
