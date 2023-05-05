package warehouse

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/blocknative/dreamboat/structs"
	"github.com/lthibault/log"
)

var (
	fileIdleTime   = structs.DurationPerSlot
	filesCloseTick = fileIdleTime + (fileIdleTime / 2) // x 1.5
	ErrClosed      = errors.New("closed")
)

type Warehouse struct {
	logger   log.Logger
	requests chan StoreRequest

	cleanShutdown  chan struct{}
	runningWorkers *structs.TimeoutWaitGroup

	m WarehouseMetrics
}

func NewWarehouse(logger log.Logger, bufSize int) *Warehouse {
	wh := &Warehouse{logger: logger.WithField("service", "warehouse"), requests: make(chan StoreRequest, bufSize), runningWorkers: structs.NewTimeoutWaitGroup()} // channel must be unbuffered, for not accepting requests that will not be handled
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
	s.runningWorkers.Add(1)
	defer s.runningWorkers.Done()

	logger := s.logger.With(log.F{
		"workerId": id,
		"datadir":  datadir,
	})

	logger.Info("started")
	defer logger.Info("stopped")

	w := newWorker(id, datadir, logger)
	pruneTicker := time.NewTicker(filesCloseTick)
WorkerLoop:
	for {
		select {
		case req := <-s.requests:
			s.handleRequest(ctx, logger, w, req)
			continue
		case <-pruneTicker.C:
			w.closeIdleFiles()
			continue
		case <-ctx.Done():
			// if context is cancelled, then stop without processing remaining requests in buffered channel
			w.closeAllFiles()
			return
		case <-s.cleanShutdown:
			break WorkerLoop
		}
	}

	// 'cleanShutdown' was called, so loop over all the requests in the buffered 's.requests' channel
	for {
		select {
		case req := <-s.requests:
			s.handleRequest(ctx, logger, w, req)
		default:
			w.closeAllFiles()
			return
		}
	}
}

func (s *Warehouse) handleRequest(ctx context.Context, logger log.Logger, w *worker, req StoreRequest) {
	logger = logger.With(req)

	file, err := w.getOrCreateFile(req)
	if err != nil {
		s.m.FailedWrites.WithLabelValues(req.DataType).Add(1)
		logger.WithError(err).Error("failed to get/create file")
		return
	}

	err = writeToFile(file, req)
	if err != nil {
		s.m.FailedWrites.WithLabelValues(req.DataType).Add(1)
		logger.WithError(err).Error("failed to write")

		file.Write([]byte("\n"))
		w.closeFile(file)
		logger.WithField("filename", file.Name()).Debug("corrupted file closed")
		return
	}

	s.m.Writes.WithLabelValues(req.DataType).Add(1)
}

func (s *Warehouse) Close(ctx context.Context) {
	close(s.cleanShutdown) // signal workers to process remaining request in buffered channel and stop

	// wait for inflight requests to complete
	select {
	case <-s.runningWorkers.C():
	case <-ctx.Done():
	}
}

func (s *Warehouse) StoreAsync(ctx context.Context, req StoreRequest) error {
	// pre-calculate header of the data
	var header bytes.Buffer
	header.WriteString("\n")
	header.WriteString(strconv.Itoa(int(req.Timestamp.UnixNano())))
	header.WriteString(";")
	header.WriteString(req.Id)
	header.WriteString(";")
	req.header = header.Bytes()

	select {
	case s.requests <- req:
		return nil
	case <-ctx.Done():
		s.logger.With(req).WithError(ctx.Err()).Warn("failed to store")
		return ctx.Err()
	}
}

func writeToFile(file *os.File, req StoreRequest) error {
	// Encode struct as JSON and write to base64 encoder
	_, err := file.Write(req.header)
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
	filename := fmt.Sprintf("%s/%s/output_%d_%d.json", w.datadir, req.DataType, req.Slot, w.id)
	if fileWithTs, ok := w.files[filename]; ok {
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
	for _, file := range w.files {
		if time.Since(file.ts) > fileIdleTime {
			w.closeFile(file.File)
		}
	}
}

func (w *worker) closeAllFiles() {
	for _, file := range w.files {
		w.closeFile(file.File)
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

	// extract the filedir from the file path
	filedir := filepath.Dir(filename)
	if _, err := os.Stat(filedir); os.IsNotExist(err) {
		err = os.MkdirAll(filedir, 0755)
		if err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// open or create file
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	return file, nil
}
