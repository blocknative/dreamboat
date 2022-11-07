package relay_test

import (
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	mock_relay "github.com/blocknative/dreamboat/internal/mock/pkg"
	relay "github.com/blocknative/dreamboat/pkg"
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
		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodGet, relay.PathStatus, nil)
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)

		require.EqualValues(t, w.Code, http.StatusOK)
	})

	t.Run("RegisterValidator", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodPost, relay.PathRegisterValidator, nil)
		w := httptest.NewRecorder()

		service.EXPECT().
			RegisterValidator(gomock.Any(), gomock.Any()).
			Times(1)

		server.ServeHTTP(w, req)
	})

	t.Run("GetHeader", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodGet, relay.PathGetHeader, nil)
		w := httptest.NewRecorder()

		service.EXPECT().
			GetHeader(gomock.Any(), gomock.Any()).
			Times(1)

		server.ServeHTTP(w, req)
	})

	t.Run("GetPayload", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodGet, relay.PathGetPayload, nil)
		w := httptest.NewRecorder()

		service.EXPECT().
			GetPayload(gomock.Any(), gomock.Any()).
			Times(1)

		server.ServeHTTP(w, req)
	})

	t.Run("SubmitBlock", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodPost, relay.PathSubmitBlock, nil)
		w := httptest.NewRecorder()

		service.EXPECT().
			SubmitBlock(gomock.Any(), gomock.Any()).
			Times(1)

		server.ServeHTTP(w, req)
	})

	t.Run("GetValidators", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodGet, relay.PathGetValidators, nil)
		w := httptest.NewRecorder()

		service.EXPECT().
			GetValidators().
			Times(1)

		server.ServeHTTP(w, req)
	})

	t.Run("builderBlocksReceived", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodGet, relay.PathBuilderBlocksReceived, nil)
		q := req.URL.Query()
		q.Add("slot", "100")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetBlockReceived(gomock.Any(), relay.TraceQuery{Limit: relay.DataLimit, Slot: 100}).
			Times(1)

		server.ServeHTTP(w, req)
	})
	t.Run("builderBlocksReceived block_hash", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodGet, relay.PathBuilderBlocksReceived, nil)
		q := req.URL.Query()

		blockHash := types.Hash(random32Bytes())
		q.Add("block_hash", blockHash.String())
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetBlockReceived(gomock.Any(), relay.TraceQuery{Limit: relay.DataLimit, BlockHash: blockHash}).
			Times(1)

		server.ServeHTTP(w, req)
	})
	t.Run("builderBlocksReceived block_number", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodGet, relay.PathBuilderBlocksReceived, nil)
		q := req.URL.Query()

		q.Add("block_number", "100")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetBlockReceived(gomock.Any(), relay.TraceQuery{Limit: relay.DataLimit, BlockNum: 100}).
			Times(1)

		server.ServeHTTP(w, req)
	})
	t.Run("builderBlocksReceived limit", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodGet, relay.PathBuilderBlocksReceived, nil)
		q := req.URL.Query()

		q.Add("limit", "50")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetBlockReceived(gomock.Any(), relay.TraceQuery{Limit: 50}).
			Times(1)

		server.ServeHTTP(w, req)
	})
	t.Run("builderBlocksReceived no limit", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodGet, relay.PathBuilderBlocksReceived, nil)
		q := req.URL.Query()

		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetBlockReceived(gomock.Any(), relay.TraceQuery{Limit: relay.DataLimit}).
			Times(1)

		server.ServeHTTP(w, req)
	})

	t.Run("payloadDelivered", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodGet, relay.PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()
		q.Add("slot", "100")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetPayloadDelivered(gomock.Any(), relay.TraceQuery{Limit: relay.DataLimit, Slot: 100}).
			Times(1)

		server.ServeHTTP(w, req)
	})
	t.Run("payloadDelivered block_hash", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodGet, relay.PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()

		blockHash := types.Hash(random32Bytes())
		q.Add("block_hash", blockHash.String())
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetPayloadDelivered(gomock.Any(), relay.TraceQuery{Limit: relay.DataLimit, BlockHash: blockHash}).
			Times(1)

		server.ServeHTTP(w, req)
	})

	t.Run("payloadDelivered block_number", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodGet, relay.PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()

		q.Add("block_number", "100")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetPayloadDelivered(gomock.Any(), relay.TraceQuery{Limit: relay.DataLimit, BlockNum: 100}).
			Times(1)

		server.ServeHTTP(w, req)
	})
	t.Run("payloadDelivered cursor", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodGet, relay.PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()

		q.Add("cursor", "50")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetPayloadDelivered(gomock.Any(), relay.TraceQuery{Limit: relay.DataLimit, Cursor: 50}).
			Times(1)

		server.ServeHTTP(w, req)
	})
	t.Run("payloadDelivered limit", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodGet, relay.PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()

		q.Add("limit", "50")
		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetPayloadDelivered(gomock.Any(), relay.TraceQuery{Limit: 50}).
			Times(1)

		server.ServeHTTP(w, req)
	})
	t.Run("payloadDelivered no limit", func(t *testing.T) {
		t.Parallel()

		service := mock_relay.NewMockRelayService(ctrl)
		server := relay.API{
			Log:     logger,
			Service: service,
		}

		req := httptest.NewRequest(http.MethodGet, relay.PathProposerPayloadsDelivered, nil)
		q := req.URL.Query()

		req.URL.RawQuery = q.Encode()

		w := httptest.NewRecorder()

		service.EXPECT().
			GetPayloadDelivered(gomock.Any(), relay.TraceQuery{Limit: relay.DataLimit}).
			Times(1)

		server.ServeHTTP(w, req)
	})
}

var reqs = []*http.Request{
	httptest.NewRequest(http.MethodGet, relay.PathStatus, nil),
	httptest.NewRequest(http.MethodPost, relay.PathRegisterValidator, nil),
	httptest.NewRequest(http.MethodGet, relay.PathGetHeader, nil),
	httptest.NewRequest(http.MethodGet, relay.PathGetPayload, nil),
	httptest.NewRequest(http.MethodPost, relay.PathSubmitBlock, nil),
	httptest.NewRequest(http.MethodGet, relay.PathGetValidators, nil),
}

func BenchmarkAPISequential(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	service := mock_relay.NewMockRelayService(ctrl)
	api := relay.API{
		Service: service,
		Log:     log.New(log.WithWriter(ioutil.Discard)),
	}

	service.EXPECT().GetHeader(gomock.Any(), gomock.Any()).AnyTimes()
	service.EXPECT().GetPayload(gomock.Any(), gomock.Any()).AnyTimes()
	service.EXPECT().GetValidators().AnyTimes()
	service.EXPECT().RegisterValidator(gomock.Any(), gomock.Any()).AnyTimes()
	service.EXPECT().SubmitBlock(gomock.Any(), gomock.Any()).AnyTimes()

	w := httptest.NewRecorder()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		api.ServeHTTP(w, reqs[rand.Intn(len(reqs))])
	}
}

func BenchmarkAPIParallel(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	var wg sync.WaitGroup
	defer wg.Wait()

	service := mock_relay.NewMockRelayService(ctrl)
	api := relay.API{
		Service: service,
		Log:     log.New(log.WithWriter(ioutil.Discard)),
	}

	service.EXPECT().GetHeader(gomock.Any(), gomock.Any()).AnyTimes()
	service.EXPECT().GetPayload(gomock.Any(), gomock.Any()).AnyTimes()
	service.EXPECT().GetValidators().AnyTimes()
	service.EXPECT().RegisterValidator(gomock.Any(), gomock.Any()).AnyTimes()
	service.EXPECT().SubmitBlock(gomock.Any(), gomock.Any()).AnyTimes()

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
			api.ServeHTTP(ws[i], randReqs[i])
			wg.Done()
		}(i)
	}
}
