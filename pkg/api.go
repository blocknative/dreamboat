package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"strconv"
	"strings"
	"sync"

	"github.com/flashbots/go-boost-utils/types"
	"github.com/gorilla/mux"
	"github.com/lthibault/log"
)

// Router paths
const (
	// proposer endpoints
	PathStatus            = "/eth/v1/builder/status"
	PathRegisterValidator = "/eth/v1/builder/validators"
	PathGetHeader         = "/eth/v1/builder/header/{slot:[0-9]+}/{parent_hash:0x[a-fA-F0-9]+}/{pubkey:0x[a-fA-F0-9]+}"
	PathGetPayload        = "/eth/v1/builder/blinded_blocks"

	// builder endpoints
	PathGetValidators = "/relay/v1/builder/validators"
	PathSubmitBlock   = "/relay/v1/builder/blocks"

	// data api
	PathBuilderBlocksReceived     = "/relay/v1/data/bidtraces/builder_blocks_received"
	PathProposerPayloadsDelivered = "/relay/v1/data/bidtraces/proposer_payload_delivered"
	PathSpecificRegistration      = "/relay/v1/data/validator_registration"

	// tracing
	PathPprofIndex   = "/debug/pprof/"
	PathPprofCmdline = "/debug/pprof/cmdline"
	PathPprofSymbol  = "/debug/pprof/symbol"
	PathPprofTrace   = "/debug/pprof/trace"
	PathPprofProfile = "/debug/pprof/profile"
)

var (
	ErrParamNotFound = errors.New("not found")
)

type API struct {
	Service       RelayService
	Log           log.Logger
	EnableProfile bool
	once          sync.Once
	mux           http.Handler
}

func (a *API) init() {
	a.once.Do(func() {
		if a.Log == nil {
			a.Log = log.New()
		}

		router := mux.NewRouter()
		router.Use(
			withDrainBody(),
			mux.CORSMethodMiddleware(router),
			withContentType("application/json"),
			withLogger(a.Log)) // set middleware

		// root returns 200 - nil
		router.HandleFunc("/", succeed(http.StatusOK))

		// proposer related
		router.HandleFunc(PathStatus, succeed(http.StatusOK)).Methods(http.MethodGet)
		router.HandleFunc(PathRegisterValidator, handler(a.registerValidator)).Methods(http.MethodPost)
		router.HandleFunc(PathGetHeader, handler(a.getHeader)).Methods(http.MethodGet)
		router.HandleFunc(PathGetPayload, handler(a.getPayload)).Methods(http.MethodPost)

		// builder related
		router.HandleFunc(PathSubmitBlock, handler(a.submitBlock)).Methods(http.MethodPost)
		router.HandleFunc(PathGetValidators, handler(a.getValidators)).Methods(http.MethodGet)

		// data API related
		router.HandleFunc(PathProposerPayloadsDelivered, handler(a.proposerPayloadsDelivered)).Methods(http.MethodGet)
		router.HandleFunc(PathBuilderBlocksReceived, handler(a.builderBlocksReceived)).Methods(http.MethodGet)
		router.HandleFunc(PathSpecificRegistration, handler(a.specificRegistration)).Methods(http.MethodGet)

		// add tracer if enabled
		if a.EnableProfile {
			router.HandleFunc(PathPprofCmdline, pprof.Cmdline)
			router.HandleFunc(PathPprofSymbol, pprof.Symbol)
			router.HandleFunc(PathPprofTrace, pprof.Trace)
			router.HandleFunc(PathPprofProfile, pprof.Profile)
			router.PathPrefix(PathPprofIndex).HandlerFunc(pprof.Index)
		}

		router.Use(mux.CORSMethodMiddleware(router))

		a.mux = router
	})

}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.init()
	a.mux.ServeHTTP(w, r)
}

func handler(f func(http.ResponseWriter, *http.Request) (int, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := f(w, r)

		if status == 0 {
			status = http.StatusOK
		}

		// NOTE:  will default to http.StatusOK if f wrote any data to the
		//        response body.
		w.WriteHeader(status)

		if err != nil {
			_ = json.NewEncoder(w).Encode(jsonError{
				Code:    status,
				Message: err.Error(),
			})
		}
	}
}

func succeed(status int) http.HandlerFunc {
	return handler(func(http.ResponseWriter, *http.Request) (int, error) {
		return status, nil
	})
}

// proposer related handlers

func (a *API) registerValidator(w http.ResponseWriter, r *http.Request) (status int, err error) {
	payload := []types.SignedValidatorRegistration{}
	if err = json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return http.StatusBadRequest, errors.New("invalid payload")
	}

	err = a.Service.RegisterValidator(r.Context(), payload)
	if err != nil {
		status = http.StatusBadRequest
	}

	return
}

func (a *API) getHeader(w http.ResponseWriter, r *http.Request) (int, error) {
	response, err := a.Service.GetHeader(r.Context(), ParseHeaderRequest(r))
	if err != nil {
		return http.StatusBadRequest, err
	}

	if err = json.NewEncoder(w).Encode(response); err != nil {
		a.Log.WithError(err).
			WithField("path", r.URL.Path).
			Debug("failed to write response")
		return http.StatusInternalServerError, err
	}

	return 0, nil
}

func (a *API) getPayload(w http.ResponseWriter, r *http.Request) (int, error) {
	var block types.SignedBlindedBeaconBlock
	if err := json.NewDecoder(r.Body).Decode(&block); err != nil {
		return http.StatusBadRequest, errors.New("invalid payload")
	}

	payload, err := a.Service.GetPayload(r.Context(), &block)
	if err != nil {
		return http.StatusBadRequest, err
	}

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		a.Log.WithError(err).
			WithField("path", r.URL.Path).
			Debug("failed to write response")
		return http.StatusInternalServerError, err
	}

	return 0, nil
}

// builder related handlers

func (a *API) submitBlock(w http.ResponseWriter, r *http.Request) (int, error) {
	var br types.BuilderSubmitBlockRequest
	if err := json.NewDecoder(r.Body).Decode(&br); err != nil {
		return http.StatusBadRequest, err
	}

	if err := a.Service.SubmitBlock(r.Context(), &br); err != nil {
		return http.StatusBadRequest, err
	}

	return 0, nil
}

func (a *API) getValidators(w http.ResponseWriter, r *http.Request) (int, error) {
	vs := a.Service.GetValidators()
	if vs == nil {
		a.Log.Trace("no registered validators for epoch")
	}

	if err := json.NewEncoder(w).Encode(vs); err != nil {
		return http.StatusInternalServerError, err
	}

	return 0, nil
}

// data API related handlers

func (a *API) specificRegistration(w http.ResponseWriter, r *http.Request) (int, error) {
	pkStr := r.URL.Query().Get("pubkey")

	var pk types.PublicKey
	if err := pk.UnmarshalText([]byte(pkStr)); err != nil {
		return http.StatusBadRequest, err
	}

	registration, err := a.Service.Registration(r.Context(), pk)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if err := json.NewEncoder(w).Encode(registration); err != nil {
		return http.StatusInternalServerError, err
	}
	return 0, nil
}

func (a *API) proposerPayloadsDelivered(w http.ResponseWriter, r *http.Request) (int, error) {
	slot, err := specificSlot(r)
	if err == nil {
		blocks, err := a.Service.GetDelivered(r.Context(), slot)
		return a.respond(w, blocks, err)
	} else if err != nil && !errors.Is(err, ErrParamNotFound) {
		return http.StatusBadRequest, err
	}

	bh, err := blockHash(r)
	if err == nil {
		blocks, err := a.Service.GetDeliveredByHash(r.Context(), bh)
		return a.respond(w, blocks, err)
	} else if err != nil && !errors.Is(err, ErrParamNotFound) {
		return http.StatusBadRequest, err
	}

	bn, err := blockNumber(r)
	if err == nil {
		blocks, err := a.Service.GetDeliveredByNum(r.Context(), bn)
		return a.respond(w, blocks, err)
	} else if err != nil && !errors.Is(err, ErrParamNotFound) {
		return http.StatusBadRequest, err
	}

	pk, err := publickKey(r)
	if err == nil {
		blocks, err := a.Service.GetDeliveredByPubKey(r.Context(), pk)
		return a.respond(w, blocks, err)
	} else if err != nil && !errors.Is(err, ErrParamNotFound) {
		return http.StatusBadRequest, err
	}

	limit, err := limit(r)
	if errors.Is(err, ErrParamNotFound) {
		limit = 100
	} else if err != nil {
		return http.StatusBadRequest, err
	}

	cursor, err := cursor(r)
	if err == nil {
		blocks, err := a.Service.GetTailDeliveredCursor(r.Context(), limit, cursor)
		return a.respond(w, blocks, err)
	} else if err != nil && !errors.Is(err, ErrParamNotFound) {
		return http.StatusBadRequest, err
	}

	blocks, err := a.Service.GetTailDelivered(r.Context(), limit)
	return a.respond(w, blocks, err)
}

func (a *API) builderBlocksReceived(w http.ResponseWriter, r *http.Request) (int, error) {
	slot, err := specificSlot(r)
	if err == nil {
		blocks, err := a.Service.GetBlockReceived(r.Context(), slot)
		return a.respond(w, blocks, err)
	} else if err != nil && !errors.Is(err, ErrParamNotFound) {
		return http.StatusBadRequest, err
	}

	bh, err := blockHash(r)
	if err == nil {
		blocks, err := a.Service.GetBlockReceivedByHash(r.Context(), bh)
		return a.respond(w, blocks, err)
	} else if err != nil && !errors.Is(err, ErrParamNotFound) {
		return http.StatusBadRequest, err
	}

	bn, err := blockNumber(r)
	if err == nil {
		blocks, err := a.Service.GetBlockReceivedByNum(r.Context(), bn)
		return a.respond(w, blocks, err)
	} else if err != nil && !errors.Is(err, ErrParamNotFound) {
		return http.StatusBadRequest, err
	}

	limit, err := limit(r)
	if err == nil {
		blocks, err := a.Service.GetTailBlockReceived(r.Context(), limit)
		return a.respond(w, blocks, err)
	} else if err != nil && !errors.Is(err, ErrParamNotFound) {
		return http.StatusBadRequest, err
	}

	blocks, err := a.Service.GetTailBlockReceived(r.Context(), 100)
	return a.respond(w, blocks, err)
}

func (a *API) respond(w http.ResponseWriter, v any, err error) (int, error) {
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return http.StatusInternalServerError, err
	}
	return 0, nil
}

func specificSlot(r *http.Request) (Slot, error) {
	if slotStr := r.URL.Query().Get("slot"); slotStr != "" {
		slot, err := strconv.ParseUint(slotStr, 10, 64)
		if err != nil {
			return Slot(0), err
		}
		return Slot(slot), nil
	}
	return Slot(0), ErrParamNotFound
}

func blockHash(r *http.Request) (types.Hash, error) {
	if bhStr := r.URL.Query().Get("block_hash"); bhStr != "" {
		var bh types.Hash
		if err := bh.UnmarshalText([]byte(bhStr)); err != nil {
			return bh, err
		}
		return bh, nil
	}
	return types.Hash{}, ErrParamNotFound
}

func publickKey(r *http.Request) (types.PublicKey, error) {
	if pkStr := r.URL.Query().Get("proposer_pubkey"); pkStr != "" {
		var pk types.PublicKey
		if err := pk.UnmarshalText([]byte(pkStr)); err != nil {
			return pk, err
		}
		return pk, nil
	}
	return types.PublicKey{}, ErrParamNotFound
}

func blockNumber(r *http.Request) (uint64, error) {
	if bnStr := r.URL.Query().Get("block_number"); bnStr != "" {
		return strconv.ParseUint(bnStr, 10, 64)
	}
	return 0, ErrParamNotFound
}

func limit(r *http.Request) (uint64, error) {
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		return strconv.ParseUint(limitStr, 10, 64)
	}
	return 0, ErrParamNotFound
}

func cursor(r *http.Request) (uint64, error) {
	if cursorStr := r.URL.Query().Get("cursor"); cursorStr != "" {
		return strconv.ParseUint(cursorStr, 10, 64)
	}
	return 0, ErrParamNotFound
}

type HeaderRequest map[string]string

func ParseHeaderRequest(r *http.Request) HeaderRequest {
	return mux.Vars(r)
}

func (hr HeaderRequest) Slot() (Slot, error) {
	slot, err := strconv.Atoi(hr["slot"])
	return Slot(slot), err
}

func (hr HeaderRequest) parentHash() (types.Hash, error) {
	var parentHash types.Hash
	err := parentHash.UnmarshalText([]byte(strings.ToLower(hr["parent_hash"])))
	return parentHash, err
}

func (hr HeaderRequest) pubkey() (PubKey, error) {
	var pk PubKey
	if err := pk.UnmarshalText([]byte(strings.ToLower(hr["pubkey"]))); err != nil {
		return PubKey{}, fmt.Errorf("invalid public key")
	}
	return pk, nil
}

type jsonError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
