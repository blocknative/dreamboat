package warehouse

import (
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/blocknative/dreamboat/beacon"
	"github.com/lthibault/log"
)

var (
	fileIdleTime   = beacon.DurationPerSlot
	filesPruneTick = fileIdleTime * 3
	ErrClosed      = errors.New("closed")
)

type Warehouse struct {
	logger   log.Logger
	requests chan StoreRequest

	shutdown chan struct{}
	wg       sync.WaitGroup

	m WarehouseMetrics
}

func NewWarehouse(logger log.Logger, bufSize int) *Warehouse {
	wh := &Warehouse{logger: logger, requests: make(chan StoreRequest)} // channel must be unbuffered, for not accepting requests that will not be handled
	wh.initMetrics()
	return wh
}

func (s *Warehouse) RunParallel(ctx context.Context, datadir string, numWorkers int) error {
	for i := 0; i < numWorkers; i++ {
		go s.Run(ctx, datadir, i)
	}

	return nil
}

func (s *Warehouse) Run(ctx context.Context, datadir string, id int) {
	s.wg.Add(1)
	defer s.wg.Done()

	logger := s.logger.WithField("id", id)

	logger.Info("started")
	defer logger.Info("stopped")

	w := newWorker(id, datadir, logger)
	ticker := time.NewTicker(filesPruneTick)
	for {
		select {
		case req := <-s.requests:
			s.handleRequest(ctx, logger, w, req)
			continue
		case <-ticker.C:
			w.closeIdleFiles()
			continue
		case <-ctx.Done():
			w.closeAllFiles()
			return
		case <-s.shutdown:
			w.closeAllFiles()
			return
		}
	}
}

func (s *Warehouse) handleRequest(ctx context.Context, logger log.Logger, w *worker, req StoreRequest) {
	file, err := w.getOrCreateFile(req)
	if err == nil {
		err = writeToFile(file, req)
		if err != nil {
			logger.WithError(err).With(req).Error("failed to write")

			file.Write([]byte("\n"))
			w.closeFile(file)
			logger.WithField("filename", file.Name()).Debug("corrupted file closed")
		}
	} else {
		logger.WithError(err).Error("failed to get/create file: %w", err)
	}

	select {
	case req.err <- err: // if does not block, means it is buffered (1) channel
	default:
	}

	select {
	case req.err <- err:
		return
	case <-s.shutdown:
		logger.WithError(ErrClosed).Error("failed to export request")
		return
	case <-ctx.Done():
		logger.WithError(ctx.Err()).Error("failed to export request")
		return
	}
}

func (s *Warehouse) Close() {
	close(s.shutdown) // prevent new requests from being added
	s.wg.Wait()       // wait for inflight requests to complete
}

func (s *Warehouse) Store(ctx context.Context, req StoreRequest) error {
	if err := s.StoreAsync(ctx, req); err != nil {
		return err
	}

	// wait for response
	select {
	case err := <-req.err:
		return err
	case <-s.shutdown:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Warehouse) StoreAsync(ctx context.Context, req StoreRequest) error {
	// check if service is closed
	select {
	case <-s.shutdown:
		return ErrClosed
	default:
	}

	// submit request
	select {
	case s.requests <- req:
		return nil
	case <-s.shutdown:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

func writeToFile(file *os.File, req StoreRequest) error {
	// Encode struct as JSON and write to base64 encoder
	_, err := file.Write(append([]byte(req.Id), byte(';')))
	if err != nil {
		return fmt.Errorf("failed to write data timestamp: %w", err)
	}

	// Create base64 encoder that writes directly to gzip writer
	encoder := base64.NewEncoder(base64.StdEncoding, file)
	defer encoder.Close()

	// Create gzip writer that writes directly to file
	writer := gzip.NewWriter(encoder)
	defer writer.Close()

	_, err = writer.Write(req.Data)
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

func (w *worker) getOrCreateFile(req StoreRequest) (*os.File, error) {
	// get
	filename := fmt.Sprintf("%s/%s/output_%d_%d.json", w.datadir, toString(req.DataType), req.Slot, w.id)
	fileWithTs, ok := w.files[filename]
	if ok {
		fileWithTs.ts = time.Now()
		return fileWithTs.File, nil
	}

	// open or create file
	file, err := openOrCreateFile(filename)
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

func (w *worker) closeAllFiles() {
	for filename, file := range w.files {
		if err := file.Close(); err != nil {
			w.logger.WithError(err).WithField("filename", filename).Error("failed to close file")
		}
		delete(w.files, filename)
	}
}

func (w *worker) closeFile(file *os.File) {
	if _, ok := w.files[file.Name()]; ok {
		delete(w.files, file.Name())
	}

	if err := file.Close(); err != nil {
		w.logger.WithError(err).WithField("filename", file.Name()).Error("failed to close file")
	}
}

func openOrCreateFile(filename string) (*os.File, error) {
	var file *os.File

	// check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		// file doesn't exist, create a new one
		file, err = os.Create(filename)
		if err != nil {
			return nil, err
		}
	} else {
		// file exists, open it for appending
		file, err = os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
	}

	return file, nil
}
