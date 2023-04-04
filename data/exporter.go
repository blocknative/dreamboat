package data

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/blocknative/dreamboat/structs"
	"github.com/lthibault/log"
)

type ExportService struct {
	logger   log.Logger
	requests chan exportRequest
	datadir  string
}

type exportRequest struct {
	bbt structs.BlockBidAndTrace
	err chan error
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
		filename := fmt.Sprintf("%s/output_%d.json", datadir, i)
		file, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}

		go func(ctx context.Context, file *os.File) {
			defer file.Close()

			bufWriter := bufio.NewWriterSize(file, 10*2060) // TODO: 2060B = 20Kb is the expected capella payload size. Bufio is a performance optimization for reducing disk writes
			defer bufWriter.Flush()

			encoder := json.NewEncoder(file)
			encoder.SetIndent("", "") //  the output JSON will be written on a single line without any whitespace: '{"field1":"value1","field2":{"nested1":"value2","nested2":"value3"}}'

			s.Run(ctx, logger.WithField("file", filename), encoder)
		}(ctx, file)
	}

	return nil
}

func (s ExportService) Run(ctx context.Context, logger log.Logger, encoder *json.Encoder) {
	logger.Info("started")
	defer logger.Info("stopped")

	for {
		select {
		case req := <-s.requests:
			select {
			case req.err <- encoder.Encode(req): // does not block because it is buffered (1) channel, but better safe than sorry
			case <-ctx.Done():
				logger.
					WithField("blockHash", req.bbt.ExecutionPayload().BlockHash()).
					Error("failed to export request")
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s ExportService) SubmitBlockBidAndTrace(ctx context.Context, bbt structs.BlockBidAndTrace) error {
	request := exportRequest{bbt: bbt, err: make(chan error, 1)}

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
