//go:generate mockgen  -destination=./mocks/mocks.go -package=mocks github.com/blocknative/dreamboat/api Relay,Registrations

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/flashbots/go-boost-utils/types"
	"github.com/gorilla/mux"
	"github.com/lthibault/log"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/blocknative/dreamboat/relay"
	"github.com/blocknative/dreamboat/structs"
	"github.com/blocknative/dreamboat/structs/forks/bellatrix"
	"github.com/blocknative/dreamboat/structs/forks/capella"
	"github.com/blocknative/dreamboat/validators"
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
)

const (
	DataLimit = 450
)

var (
	ErrParamNotFound = errors.New("not found")
)

type Relay interface {
	// Proposer APIs (builder spec https://github.com/ethereum/builder-specs)
	GetHeader(context.Context, *structs.MetricGroup, structs.HeaderRequest) (structs.GetHeaderResponse, error)
	GetPayload(context.Context, *structs.MetricGroup, structs.SignedBlindedBeaconBlock) (structs.GetPayloadResponse, error)

	// Builder APIs (relay spec https://flashbots.notion.site/Relay-API-Spec-5fb0819366954962bc02e81cb33840f5)
	SubmitBlock(context.Context, *structs.MetricGroup, structs.SubmitBlockRequest) error

	// Data APIs
	GetPayloadDelivered(context.Context, structs.PayloadTraceQuery) ([]structs.BidTraceExtended, error)
	GetBlockReceived(context.Context, structs.HeaderTraceQuery) ([]structs.BidTraceWithTimestamp, error)
}

type Registrations interface {
	// Proposer APIs (builder spec https://github.com/ethereum/builder-specs)
	RegisterValidator(context.Context, *structs.MetricGroup, []types.SignedValidatorRegistration) error
	// Data APIs
	Registration(context.Context, types.PublicKey) (types.SignedValidatorRegistration, error)
	// Builder APIs (relay spec https://flashbots.notion.site/Relay-API-Spec-5fb0819366954962bc02e81cb33840f5)
	GetValidators(*structs.MetricGroup) structs.BuilderGetValidatorsResponseEntrySlice
}

type RateLimitter interface {
	Allow(ctx context.Context, pubkey [48]byte) error
}

type State interface {
	ForkVersion(epoch structs.Slot) structs.ForkVersion
	HeadSlot() structs.Slot
}

type API struct {
	l   log.Logger
	r   Relay
	reg Registrations
	st  State

	lim RateLimitter

	m *APIMetrics
}

func NewApi(l log.Logger, r Relay, reg Registrations, st State, lim RateLimitter) (a *API) {
	a = &API{
		l:   l,
		r:   r,
		reg: reg,
		st:  st,
		lim: lim,
		m:   &APIMetrics{}}
	a.initMetrics()
	return a
}

func (a *API) AttachToHandler(m *http.ServeMux) {
	router := mux.NewRouter()
	router.Use(mux.CORSMethodMiddleware(router), withAddons(a.l))

	// root returns 200 - nil
	router.HandleFunc("/", status)

	// proposer related
	router.HandleFunc(PathStatus, status).Methods(http.MethodGet)
	router.HandleFunc(PathRegisterValidator, a.registerValidator).Methods(http.MethodPost)
	router.HandleFunc(PathGetHeader, a.getHeader).Methods(http.MethodGet)
	router.HandleFunc(PathGetPayload, a.getPayload).Methods(http.MethodPost)

	// builder related
	router.HandleFunc(PathSubmitBlock, a.submitBlock).Methods(http.MethodPost)
	router.HandleFunc(PathGetValidators, a.getValidators).Methods(http.MethodGet)

	// data API related
	router.HandleFunc(PathProposerPayloadsDelivered, a.proposerPayloadsDelivered).Methods(http.MethodGet)
	router.HandleFunc(PathBuilderBlocksReceived, a.builderBlocksReceived).Methods(http.MethodGet)
	router.HandleFunc(PathSpecificRegistration, a.specificRegistration).Methods(http.MethodGet)

	router.Use(mux.CORSMethodMiddleware(router))
	m.Handle("/", router)
}

func status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

// proposer related handlers
func (a *API) registerValidator(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	timer := prometheus.NewTimer(a.m.ApiReqTiming.WithLabelValues("registerValidator"))
	defer timer.ObserveDuration()

	payload := []types.SignedValidatorRegistration{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		a.m.ApiReqCounter.WithLabelValues("registerValidator", "400", "input decoding").Inc()
		writeError(w, http.StatusBadRequest, errors.New("invalid payload"))
		return
	}

	if payload != nil {
		a.m.ApiReqElCount.WithLabelValues("registerValidator", "payload").Observe(float64(len(payload)))
	}

	m := structs.NewMetricGroup(4)
	if err := a.reg.RegisterValidator(r.Context(), m, payload); err != nil {
		m.ObserveWithError(a.m.RelayTiming, unwrapError(err, "register validator unknown"))
		a.m.ApiReqCounter.WithLabelValues("registerValidator", "400", "register validator").Inc()
		a.l.With(log.F{
			"code":     400,
			"endpoint": "registerValidator",
			"type":     "single",
			"payload":  payload,
		}).WithError(err).Debug("failed registerValidator")
		writeError(w, http.StatusBadRequest, err)
		return
	}

	m.Observe(a.m.RelayTiming)
	a.m.ApiReqCounter.WithLabelValues("registerValidator", "200", "").Inc()
}

func (a *API) getHeader(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	timer := prometheus.NewTimer(a.m.ApiReqTiming.WithLabelValues("getHeader"))
	defer timer.ObserveDuration()

	req := ParseHeaderRequest(r)
	m := structs.NewMetricGroup(4)
	response, err := a.r.GetHeader(r.Context(), m, req)
	if err != nil {
		m.ObserveWithError(a.m.RelayTiming, unwrapError(err, "get header unknown"))
		a.m.ApiReqCounter.WithLabelValues("getHeader", "400", "get header").Inc()
		slot, _ := req.Slot()
		proposer, _ := req.Pubkey()
		a.l.With(log.F{
			"code":     400,
			"endpoint": "getHeader",
			"payload":  req,
			"slot":     slot,
			"proposer": proposer,
		}).WithError(err).Debug("failed getHeader")
		writeError(w, http.StatusBadRequest, err)
		return
	}

	m.Observe(a.m.RelayTiming)

	if err = json.NewEncoder(w).Encode(response); err != nil {
		a.l.WithError(err).WithField("path", r.URL.Path).Debug("failed to write response")
		a.m.ApiReqCounter.WithLabelValues("getHeader", "500", "response encode").Inc()
		// we don't write response as encoder already crashed
		return
	}

	a.m.ApiReqCounter.WithLabelValues("getHeader", "200", "").Inc()
}

func (a *API) getPayload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	timer := prometheus.NewTimer(a.m.ApiReqTiming.WithLabelValues("getPayload"))
	defer timer.ObserveDuration()

	var req structs.SignedBlindedBeaconBlock
	fork := a.st.ForkVersion(a.st.HeadSlot())
	switch fork {
	case structs.ForkCapella:
		var creq capella.SignedBlindedBeaconBlock
		if err := json.NewDecoder(r.Body).Decode(&creq); err != nil {
			a.m.ApiReqCounter.WithLabelValues("getPayload", "400", "payload decode").Inc()
			writeError(w, http.StatusBadRequest, errors.New("invalid getPayload request cappella decode"))
			return
		}
		req = &creq
		if !creq.Validate() {
			a.m.ApiReqCounter.WithLabelValues("getPayload", "400", "payload validation").Inc()
			a.l.With(log.F{
				"code":      400,
				"endpoint":  "getPayload",
				"slot":      creq.Slot(),
				"blockHash": creq.BlockHash(),
			}).Debug("invalid payload")
			writeError(w, http.StatusBadRequest, errors.New("invalid payload"))
			return
		}
	case structs.ForkBellatrix:
		var breq bellatrix.SignedBlindedBeaconBlock
		if err := json.NewDecoder(r.Body).Decode(&breq); err != nil {
			a.m.ApiReqCounter.WithLabelValues("getPayload", "400", "payload decode").Inc()
			writeError(w, http.StatusBadRequest, errors.New("invalid getPayload request bellatrix decode"))
			return
		}
		req = &breq
		if !breq.Validate() {
			a.m.ApiReqCounter.WithLabelValues("getPayload", "400", "payload validation").Inc()
			a.l.With(log.F{
				"code":      400,
				"endpoint":  "getPayload",
				"slot":      breq.Slot(),
				"blockHash": breq.BlockHash(),
			}).Debug("invalid payload")
			writeError(w, http.StatusBadRequest, errors.New("invalid payload"))
			return
		}
	default:
		writeError(w, http.StatusInternalServerError, errors.New("not supported fork version"))
		return
	}

	m := structs.NewMetricGroup(4)
	payload, err := a.r.GetPayload(r.Context(), m, req)
	if err != nil {
		m.ObserveWithError(a.m.RelayTiming, unwrapError(err, "get payload unknown"))
		a.m.ApiReqCounter.WithLabelValues("getPayload", "400", "get payload").Inc()
		a.l.With(log.F{
			"code":      400,
			"endpoint":  "getPayload",
			"slot":      req.Slot(),
			"blockHash": req.BlockHash(),
		}).WithError(err).Debug("failed getPayload")
		writeError(w, http.StatusBadRequest, err)
		return
	}

	m.Observe(a.m.RelayTiming)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		a.l.WithError(err).WithField("path", r.URL.Path).Debug("failed to write response")
		a.m.ApiReqCounter.WithLabelValues("getPayload", "500", "encode response").Inc()
		// we don't write response as encoder already crashed
		return
	}

	a.m.ApiReqCounter.WithLabelValues("getPayload", "200", "").Inc()
}

// builder related handlers
func (a *API) submitBlock(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var l = a.l
	timer := prometheus.NewTimer(a.m.ApiReqTiming.WithLabelValues("submitBlock"))
	defer timer.ObserveDuration()
	var req structs.SubmitBlockRequest

	switch a.st.ForkVersion(a.st.HeadSlot()) {
	case structs.ForkCapella:
		var creq capella.SubmitBlockRequest
		if err := json.NewDecoder(r.Body).Decode(&creq); err != nil {
			a.m.ApiReqCounter.WithLabelValues("submitBlock", "400", "payload decode").Inc()
			writeError(w, http.StatusBadRequest, errors.New("invalid submitblock request capella decode"))
			return
		}
		req = &creq
		l = a.l.With(log.F{
			"fork":      "capella",
			"slot":      creq.CapellaMessage.Slot,
			"blockHash": creq.CapellaMessage.BlockHash,
			"bidValue":  creq.CapellaMessage.Value,
			"proposer":  creq.CapellaMessage.ProposerPubkey,
			"builder":   creq.CapellaMessage.BuilderPubkey,
		})
		if !creq.Validate() {
			a.m.ApiReqCounter.WithLabelValues("submitBlock", "400", "payload validation").Inc()
			l.Warn("invalid block submit payload")
			writeError(w, http.StatusBadRequest, errors.New("invalid payload"))
			return
		}
	case structs.ForkBellatrix:
		var breq bellatrix.SubmitBlockRequest
		if err := json.NewDecoder(r.Body).Decode(&breq); err != nil {
			a.m.ApiReqCounter.WithLabelValues("submitBlock", "400", "payload decode").Inc()
			writeError(w, http.StatusBadRequest, errors.New("invalid submitblock request bellatrix decode"))
			return
		}
		req = &breq
		l = a.l.With(log.F{
			"fork":      "bellatrix",
			"slot":      breq.BellatrixMessage.Slot,
			"blockHash": breq.BellatrixMessage.BlockHash,
			"bidValue":  breq.BellatrixMessage.Value,
			"proposer":  breq.BellatrixMessage.ProposerPubkey,
			"builder":   breq.BellatrixMessage.BuilderPubkey,
		})
		if !breq.Validate() {
			a.m.ApiReqCounter.WithLabelValues("submitBlock", "400", "payload validation").Inc()
			l.Warn("invalid block submit payload")
			writeError(w, http.StatusBadRequest, errors.New("invalid payload"))
			return
		}
	default:
		a.m.ApiReqCounter.WithLabelValues("submitBlock", "500", "unsupported fork").Inc()
		writeError(w, http.StatusInternalServerError, errors.New("not supported fork version"))
		return

	}
	if req.Slot() == 0 {
		a.m.ApiReqCounter.WithLabelValues("submitBlock", "400", "payload decode").Inc()
		writeError(w, http.StatusBadRequest, errors.New("invalid payload (slot)"))
		return
	}

	if err := a.lim.Allow(r.Context(), req.BuilderPubkey()); err != nil {
		a.m.ApiReqCounter.WithLabelValues("submitBlock", "429", "rate limitted").Inc()
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	a.m.ApiReqElCount.WithLabelValues("submitBlock", "transaction").Observe(float64(req.NumTx()))

	m := structs.NewMetricGroup(4)
	if err := a.r.SubmitBlock(r.Context(), m, req); err != nil {
		m.ObserveWithError(a.m.RelayTiming, unwrapError(err, "submit block unknown"))
		if errors.Is(err, relay.ErrPayloadAlreadyDelivered) {
			a.m.ApiReqCounter.WithLabelValues("submitBlock", "400", "payload already delivered").Inc()
		} else {
			a.m.ApiReqCounter.WithLabelValues("submitBlock", "400", "block submission").Inc()
			l.With(log.F{
				"code":     400,
				"endpoint": "submitBlock",
			}).WithError(err).Debug("failed block submission")
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	m.Observe(a.m.RelayTiming)
	a.m.ApiReqCounter.WithLabelValues("submitBlock", "200", "").Inc()
}

func (a *API) getValidators(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	timer := prometheus.NewTimer(a.m.ApiReqTiming.WithLabelValues("getValidators"))
	defer timer.ObserveDuration()

	m := structs.NewMetricGroup(4)
	vs := a.reg.GetValidators(m)
	if vs == nil {
		a.l.Trace("no registered validators for epoch")
		vs = structs.BuilderGetValidatorsResponseEntrySlice{}
		m.ObserveWithError(a.m.RelayTiming, fmt.Errorf("no validators"))
	}

	if vs != nil {
		m.Observe(a.m.RelayTiming)
		a.m.ApiReqElCount.WithLabelValues("getValidators", "validator").Observe(float64(len(vs)))
	}

	if err := json.NewEncoder(w).Encode(vs); err != nil {
		a.m.ApiReqCounter.WithLabelValues("getValidators", "500", "response encode").Inc()
		// we don't write response as encoder already crashed
		return
	}

	a.m.ApiReqCounter.WithLabelValues("getValidators", "200", "").Inc()
}

// data API related handlers
func (a *API) specificRegistration(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	timer := prometheus.NewTimer(a.m.ApiReqTiming.WithLabelValues("specificRegistration"))
	defer timer.ObserveDuration()

	pkStr := r.URL.Query().Get("pubkey")

	var pk types.PublicKey
	if err := pk.UnmarshalText([]byte(pkStr)); err != nil {
		a.m.ApiReqCounter.WithLabelValues("specificRegistration", "400", "unmarshaling pk").Inc()
		writeError(w, http.StatusBadRequest, err)
		return
	}

	registration, err := a.reg.Registration(r.Context(), pk)
	if err != nil {
		a.m.ApiReqCounter.WithLabelValues("specificRegistration", "500", "registration").Inc()
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if err := json.NewEncoder(w).Encode(registration); err != nil {
		a.m.ApiReqCounter.WithLabelValues("specificRegistration", "500", "encode response").Inc()
		// we don't write response as encoder already crashed
		return
	}

	a.m.ApiReqCounter.WithLabelValues("specificRegistration", "200", "").Inc()
}

func (a *API) proposerPayloadsDelivered(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	query, kind, err := validateProposerPayloadsDelivered(r)
	if err != nil {
		a.m.ApiReqCounter.WithLabelValues("proposerPayloadsDelivered", "400", "bad "+kind).Inc()
		writeError(w, http.StatusBadRequest, err)
		return
	}

	payloads, err := a.r.GetPayloadDelivered(r.Context(), query)
	if err != nil {
		a.m.ApiReqCounter.WithLabelValues("proposerPayloadsDelivered", "500", "get payloads").Inc()
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if payloads != nil {
		a.m.ApiReqElCount.WithLabelValues("proposerPayloadsDelivered", "payload").Observe(float64(len(payloads)))
	}

	if err := json.NewEncoder(w).Encode(payloads); err != nil {
		a.m.ApiReqCounter.WithLabelValues("proposerPayloadsDelivered", "500", "encode response").Inc()
		// we don't write response as encoder already crashed
		return
	}

	a.m.ApiReqCounter.WithLabelValues("proposerPayloadsDelivered", "200", "").Inc()
}

func (a *API) builderBlocksReceived(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	query, kind, err := validateBuilderBlocksReceived(r)
	if err != nil {
		a.m.ApiReqCounter.WithLabelValues("builderBlocksReceived", "400", "bad "+kind).Inc()
		writeError(w, http.StatusBadRequest, err)
		return
	}

	blocks, err := a.r.GetBlockReceived(r.Context(), query)
	if err != nil {
		a.m.ApiReqCounter.WithLabelValues("builderBlocksReceived", "500", "get block").Inc()
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if blocks != nil {
		a.m.ApiReqElCount.WithLabelValues("builderBlocksReceived", "block").Observe(float64(len(blocks)))
	}

	if err := json.NewEncoder(w).Encode(blocks); err != nil {
		a.m.ApiReqCounter.WithLabelValues("builderBlocksReceived", "500", "encode response").Inc()
		// we don't write response as encoder already crashed
		return
	}

	a.m.ApiReqCounter.WithLabelValues("builderBlocksReceived", "200", "").Inc()
}

func writeError(w http.ResponseWriter, code int, err error) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(jsonError{
		Code:    code,
		Message: err.Error(),
	})
}

func validateBuilderBlocksReceived(r *http.Request) (query structs.HeaderTraceQuery, kind string, err error) {

	slot, err := specificSlot(r)
	if err != nil && !errors.Is(err, ErrParamNotFound) {
		return query, "slot", err
	}

	bh, err := blockHash(r)
	if err != nil && !errors.Is(err, ErrParamNotFound) {
		return query, "hash", err
	}

	bn, err := blockNumber(r)
	if err != nil && !errors.Is(err, ErrParamNotFound) {
		return query, "number", err
	}

	limit, err := limit(r)
	if err != nil && !errors.Is(err, ErrParamNotFound) {
		return query, "limit", err
	}

	if errors.Is(err, ErrParamNotFound) {
		limit = DataLimit
	}

	return structs.HeaderTraceQuery{
		Slot:      slot,
		BlockHash: bh,
		BlockNum:  bn,
		Limit:     limit,
	}, "", nil
}

func validateProposerPayloadsDelivered(r *http.Request) (query structs.PayloadTraceQuery, kind string, err error) {

	slot, err := specificSlot(r)
	if err != nil && !errors.Is(err, ErrParamNotFound) {
		return query, "slot", err
	}

	bh, err := blockHash(r)
	if err != nil && !errors.Is(err, ErrParamNotFound) {
		return query, "hash", err
	}

	bn, err := blockNumber(r)
	if err != nil && !errors.Is(err, ErrParamNotFound) {
		return query, "number", err
	}

	pk, err := publickKey(r)
	if err != nil && !errors.Is(err, ErrParamNotFound) {
		return query, "key", err
	}

	limit, err := limit(r)
	if err != nil && !errors.Is(err, ErrParamNotFound) {
		return query, "limit", err
	}

	if errors.Is(err, ErrParamNotFound) {
		limit = DataLimit
	}

	cursor, err := cursor(r)
	if err != nil && !errors.Is(err, ErrParamNotFound) {
		return query, "cursor", err
	}

	return structs.PayloadTraceQuery{
		Slot:      slot,
		BlockHash: bh,
		BlockNum:  bn,
		Pubkey:    pk,
		Cursor:    cursor,
		Limit:     limit,
	}, "", nil
}

func specificSlot(r *http.Request) (structs.Slot, error) {
	if slotStr := r.URL.Query().Get("slot"); slotStr != "" {
		slot, err := strconv.ParseUint(slotStr, 10, 64)
		if err != nil {
			return structs.Slot(0), err
		}
		return structs.Slot(slot), nil
	}
	return structs.Slot(0), ErrParamNotFound
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
		limit, err := strconv.ParseUint(limitStr, 10, 64)
		if err != nil {
			return 0, err
		} else if DataLimit < limit {
			return 0, fmt.Errorf("limit is higher than %d", DataLimit)
		}
		return limit, err
	}
	return 0, ErrParamNotFound
}

func cursor(r *http.Request) (uint64, error) {
	if cursorStr := r.URL.Query().Get("cursor"); cursorStr != "" {
		return strconv.ParseUint(cursorStr, 10, 64)
	}
	return 0, ErrParamNotFound
}

func ParseHeaderRequest(r *http.Request) structs.HeaderRequest {
	return mux.Vars(r)
}

type jsonError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func unwrapError(err error, defaultMsg string) error {
	if errors.Is(err, relay.ErrUnknownValue) {
		return relay.ErrUnknownValue
	} else if errors.Is(err, relay.ErrPayloadAlreadyDelivered) {
		return relay.ErrPayloadAlreadyDelivered
	} else if errors.Is(err, relay.ErrNoPayloadFound) {
		return relay.ErrNoPayloadFound
	} else if errors.Is(err, relay.ErrMissingRequest) {
		return relay.ErrMissingRequest
	} else if errors.Is(err, relay.ErrMissingSecretKey) {
		return relay.ErrMissingSecretKey
	} else if errors.Is(err, relay.ErrNoBuilderBid) {
		return relay.ErrNoBuilderBid
	} else if errors.Is(err, relay.ErrOldSlot) {
		return relay.ErrOldSlot
	} else if errors.Is(err, relay.ErrBadHeader) {
		return relay.ErrBadHeader
	} else if errors.Is(err, relay.ErrInvalidSignature) {
		return relay.ErrInvalidSignature
	} else if errors.Is(err, relay.ErrStore) {
		return relay.ErrStore
	} else if errors.Is(err, relay.ErrMarshal) {
		return relay.ErrMarshal
	} else if errors.Is(err, relay.ErrInternal) {
		return relay.ErrInternal
	} else if errors.Is(err, relay.ErrUnknownValidator) {
		return relay.ErrUnknownValidator
	} else if errors.Is(err, relay.ErrVerification) {
		return relay.ErrVerification
	} else if errors.Is(err, relay.ErrInvalidTimestamp) {
		return relay.ErrInvalidTimestamp
	} else if errors.Is(err, relay.ErrInvalidSlot) {
		return relay.ErrInvalidSlot
	} else if errors.Is(err, relay.ErrEmptyBlock) {
		return relay.ErrEmptyBlock
	} else if errors.Is(err, relay.ErrInvalidRandao) {
		return relay.ErrInvalidRandao
	} else if errors.Is(err, validators.ErrInvalidSignature) {
		return validators.ErrInvalidSignature
	} else if errors.Is(err, validators.ErrUnknownValidator) {
		return validators.ErrUnknownValidator
	} else if errors.Is(err, validators.ErrInvalidTimestamp) {
		return validators.ErrInvalidTimestamp
	}

	return errors.New(defaultMsg)
}
