package api_test

import (
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	api "github.com/blocknative/dreamboat/pkg/api"
	mock_relay "github.com/blocknative/dreamboat/pkg/api/mocks"
	"github.com/blocknative/dreamboat/pkg/structs"
	"github.com/flashbots/go-boost-utils/types"
	"github.com/golang/mock/gomock"
	"github.com/lthibault/log"
	"github.com/stretchr/testify/require"
)

var (
	logger = log.New(log.WithWriter(io.Discard))
)

func TestServerRouting(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("Status", func(t *testing.T) {
		t.Parallel()
		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, nil)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodGet, api.PathStatus, nil)
		w := httptest.NewRecorder()
		m.ServeHTTP(w, req)

		require.EqualValues(t, w.Code, http.StatusOK)
	})

	t.Run("RegisterValidator", func(t *testing.T) {
		t.Parallel()
		register := mock_relay.NewMockRegistrations(ctrl)
		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, register)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodPost, api.PathRegisterValidator, nil)
		w := httptest.NewRecorder()

		register.EXPECT().
			RegisterValidator(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(1)

		m.ServeHTTP(w, req)
	})

	t.Run("GetHeader", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, nil)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodGet, api.PathGetHeader, nil)
		w := httptest.NewRecorder()

		service.EXPECT().
			GetHeader(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(1)

		m.ServeHTTP(w, req)
	})

	t.Run("GetPayload", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, nil)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodGet, api.PathGetPayload, nil)
		w := httptest.NewRecorder()

		service.EXPECT().
			GetPayload(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(1)

		m.ServeHTTP(w, req)
	})

	t.Run("SubmitBlock", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, nil)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodPost, api.PathSubmitBlock, nil)
		w := httptest.NewRecorder()

		service.EXPECT().
			SubmitBlock(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(1)

		m.ServeHTTP(w, req)
	})

	t.Run("GetValidators", func(t *testing.T) {
		t.Parallel()
		register := mock_relay.NewMockRegistrations(ctrl)
		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, nil)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodGet, api.PathGetValidators, nil)
		w := httptest.NewRecorder()

		register.EXPECT().
			GetValidators(gomock.Any()).
			Times(1)

		m.ServeHTTP(w, req)
	})

	t.Run("builderBlocksReceived", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, nil)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodGet, api.PathBuilderBlocksReceived, nil)
		q := req.URL.Query()
		q.Add("slot", "100")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetBlockReceived(gomock.Any(), structs.SubmissionTraceQuery{Limit: api.DataLimit, Slot: 100}).
			Times(1)

		m.ServeHTTP(w, req)
	})
	t.Run("builderBlocksReceived block_hash", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, nil)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodGet, api.PathBuilderBlocksReceived, nil)
		q := req.URL.Query()

		blockHash := types.Hash(random32Bytes())
		q.Add("block_hash", blockHash.String())
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetBlockReceived(gomock.Any(), structs.SubmissionTraceQuery{Limit: api.DataLimit, BlockHash: blockHash}).
			Times(1)

		m.ServeHTTP(w, req)
	})
	t.Run("builderBlocksReceived block_number", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, nil)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodGet, api.PathBuilderBlocksReceived, nil)
		q := req.URL.Query()

		q.Add("block_number", "100")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetBlockReceived(gomock.Any(), structs.SubmissionTraceQuery{Limit: api.DataLimit, BlockNum: 100}).
			Times(1)

		m.ServeHTTP(w, req)
	})
	t.Run("builderBlocksReceived limit", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, nil)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodGet, api.PathBuilderBlocksReceived, nil)
		q := req.URL.Query()

		q.Add("limit", "50")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetBlockReceived(gomock.Any(), structs.SubmissionTraceQuery{Limit: 50}).
			Times(1)

		m.ServeHTTP(w, req)
	})
	t.Run("builderBlocksReceived no limit", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, nil)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodGet, api.PathBuilderBlocksReceived, nil)
		q := req.URL.Query()

		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetBlockReceived(gomock.Any(), structs.SubmissionTraceQuery{Limit: api.DataLimit}).
			Times(1)

		m.ServeHTTP(w, req)
	})

	t.Run("payloadDelivered", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, nil)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodGet, api.PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()
		q.Add("slot", "100")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetPayloadDelivered(gomock.Any(), structs.PayloadTraceQuery{Limit: api.DataLimit, Slot: 100}).
			Times(1)

		m.ServeHTTP(w, req)
	})
	t.Run("payloadDelivered block_hash", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, nil)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodGet, api.PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()

		blockHash := types.Hash(random32Bytes())
		q.Add("block_hash", blockHash.String())
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetPayloadDelivered(gomock.Any(), structs.PayloadTraceQuery{Limit: api.DataLimit, BlockHash: blockHash}).
			Times(1)

		m.ServeHTTP(w, req)
	})

	t.Run("payloadDelivered block_number", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, nil)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodGet, api.PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()

		q.Add("block_number", "100")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetPayloadDelivered(gomock.Any(), structs.PayloadTraceQuery{Limit: api.DataLimit, BlockNum: 100}).
			Times(1)

		m.ServeHTTP(w, req)
	})
	t.Run("payloadDelivered cursor", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, nil)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodGet, api.PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()

		q.Add("cursor", "50")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetPayloadDelivered(gomock.Any(), structs.PayloadTraceQuery{Limit: api.DataLimit, Cursor: 50}).
			Times(1)

		m.ServeHTTP(w, req)
	})
	t.Run("payloadDelivered limit", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, nil)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodGet, api.PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()

		q.Add("limit", "50")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetPayloadDelivered(gomock.Any(), structs.PayloadTraceQuery{Limit: 50}).
			Times(1)

		m.ServeHTTP(w, req)
	})
	t.Run("payloadDelivered no limit", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelay(ctrl)
		server := api.NewApi(logger, service, nil)
		m := http.NewServeMux()
		server.AttachToHandler(m)

		req := httptest.NewRequest(http.MethodGet, api.PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()

		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetPayloadDelivered(gomock.Any(), structs.PayloadTraceQuery{Limit: api.DataLimit}).
			Times(1)

		m.ServeHTTP(w, req)
	})
}

var reqs = []*http.Request{
	httptest.NewRequest(http.MethodGet, api.PathStatus, nil),
	httptest.NewRequest(http.MethodPost, api.PathRegisterValidator, nil),
	httptest.NewRequest(http.MethodGet, api.PathGetHeader, nil),
	httptest.NewRequest(http.MethodGet, api.PathGetPayload, nil),
	httptest.NewRequest(http.MethodPost, api.PathSubmitBlock, nil),
	httptest.NewRequest(http.MethodGet, api.PathGetValidators, nil),
}

func BenchmarkAPISequential(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	service := mock_relay.NewMockRelay(ctrl)
	register := mock_relay.NewMockRegistrations(ctrl)
	//Log:     log.New(log.WithWriter(ioutil.Discard)),
	server := api.NewApi(logger, service, register)
	m := http.NewServeMux()
	server.AttachToHandler(m)

	service.EXPECT().GetHeader(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	service.EXPECT().GetPayload(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	register.EXPECT().GetValidators(gomock.Any()).AnyTimes()
	register.EXPECT().RegisterValidator(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	service.EXPECT().SubmitBlock(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	w := httptest.NewRecorder()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		m.ServeHTTP(w, reqs[rand.Intn(len(reqs))])
	}
}

func BenchmarkAPIParallel(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	var wg sync.WaitGroup
	defer wg.Wait()

	service := mock_relay.NewMockRelay(ctrl)

	register := mock_relay.NewMockRegistrations(ctrl)
	//Log:     log.New(log.WithWriter(ioutil.Discard)),
	server := api.NewApi(logger, service, register)
	m := http.NewServeMux()
	server.AttachToHandler(m)

	service.EXPECT().GetHeader(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	service.EXPECT().GetPayload(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	register.EXPECT().GetValidators(gomock.Any()).AnyTimes()
	register.EXPECT().RegisterValidator(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	service.EXPECT().SubmitBlock(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	randReqs := make([]*http.Request, 0)
	ws := make([]*httptest.ResponseRecorder, 0)

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := reqs[rand.Intn(len(reqs))]
		randReq := httptest.NewRequest(req.Method, req.URL.Path, nil)
		randReqs = append(randReqs, randReq)
		ws = append(ws, w)
	}

	wg.Add(b.N)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		go func(i int) {
			m.ServeHTTP(ws[i], randReqs[i])
			wg.Done()
		}(i)
	}
}

func random32Bytes() (b [32]byte) {
	rand.Read(b[:])
	return b
}
