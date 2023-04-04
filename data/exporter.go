package data

import (
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/blocknative/dreamboat/beacon"
	"github.com/blocknative/dreamboat/structs"
	"github.com/lthibault/log"
)

var (
	fileIdleTime = beacon.DurationPerSlot
)

type ExportService struct {
	logger   log.Logger
	requests chan exportRequest
}

func NewExportService(logger log.Logger, bufSize int) *ExportService {
	return &ExportService{logger: logger, requests: make(chan exportRequest, bufSize)}
}

func (s *ExportService) RunParallel(ctx context.Context, datadir string, numWorkers int) error {
	for i := 0; i < numWorkers; i++ {
		go s.Run(ctx, datadir, i)
	}

	return nil
}

func (s *ExportService) Run(ctx context.Context, datadir string, id int) {
	logger := s.logger.WithField("id", id)

	logger.Info("started")
	defer logger.Info("stopped")

	worker := newWorker(id, datadir, logger)
	for {
		select {
		case req := <-s.requests:
			file, err := worker.getOrCreateFile(req)
			if err == nil {
				err = writeToFile(file, req)
				if err != nil {
					logger.WithError(err).With(req).Error("failed to write")

					file.Write([]byte("\n"))
					worker.closeFile(file)
					logger.WithField("filename", file.Name()).Debug("corrupted file closed")
				}
			} else {
				logger.WithError(err).Error("failed to get/create file: %w", err)
			}

			select {
			case req.err <- err: // does not block because it is buffered (1) channel, but better safe than sorry
			case <-ctx.Done():
				logger.WithError(ctx.Err()).Error("failed to export request")
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *ExportService) SubmitGetPayloadRequest(ctx context.Context, gpr structs.SignedBlindedBeaconBlock) error {
	request := exportRequest{dt: BlockBidAndTraceData, data: gpr.Raw(), slot: gpr.Slot(), timestamp: time.Now(), id: gpr.BlockHash().String(), err: make(chan error, 1)}

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

func writeToFile(file *os.File, req exportRequest) error {
	// Encode struct as JSON and write to base64 encoder
	_, err := file.Write([]byte(strconv.Itoa(req.timestamp.Nanosecond())))
	if err != nil {
		return fmt.Errorf("failed to write data timestamp: %w", err)
	}

	_, err = file.Write([]byte(";"))
	if err != nil {
		return fmt.Errorf("failed to write data sep ';': %w", err)
	}

	_, err = file.Write([]byte(req.id))
	if err != nil {
		return fmt.Errorf("failed to write data identifier: %w", err)
	}

	_, err = file.Write([]byte(";"))
	if err != nil {
		return fmt.Errorf("failed to write data sep ';': %w", err)
	}

	// Create base64 encoder that writes directly to gzip writer
	encoder := base64.NewEncoder(base64.StdEncoding, file)
	encoder.Close()

	// Create gzip writer that writes directly to file
	writer := gzip.NewWriter(encoder)
	defer writer.Close()

	_, err = writer.Write(req.data)
	if err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush writer: %w", err)
	}

	// Flush any remaining data from the encoder and writer
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("failed to close encoder: %w", err)
	}

	_, err = file.Write([]byte("\n"))
	if err != nil {
		return fmt.Errorf("failed to write new data sep newline': %w", err)
	}

	return nil
}

type worker struct {
	id      int
	datadir string
	files   map[string]fileWithTimestamp

	logger log.Logger
}

func newWorker(id int, datadir string, logger log.Logger) *worker {
	return &worker{id: id, datadir: datadir, files: make(map[string]fileWithTimestamp), logger: logger}
}

func (w *worker) getOrCreateFile(req exportRequest) (*os.File, error) {
	// get
	filename := fmt.Sprintf("%s/%s/output_%d_%d.json", w.datadir, toString(req.dt), req.slot, w.id)
	fileWithTs, ok := w.files[filename]
	if ok {
		fileWithTs.ts = time.Now()
		return fileWithTs.File, nil
	}

	// create
	file, err := os.Create(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	w.files[filename] = fileWithTimestamp{File: file, ts: time.Now()}
	return file, nil
}

func (w *worker) closeIdleFiles() {
	for filename, file := range w.files {
		if time.Since(file.ts) > fileIdleTime {
			if err := file.Close(); err != nil {
				w.logger.WithError(err).WithField("filename", filename).Error("failed to close file")
			}
			delete(w.files, filename)
		}
	}
}

func (w *worker) closeFile(file *os.File) error {
	if _, ok := w.files[file.Name()]; ok {
		delete(w.files, file.Name())
	}

	return file.Close()
}
