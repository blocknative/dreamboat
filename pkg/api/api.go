//go:generate mockgen  -destination=./mocks/mocks.go -package=mocks github.com/blocknative/dreamboat/pkg/api Service

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

	"github.com/blocknative/dreamboat/pkg/structs"
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
	DataLimit = 200
)

var (
	ErrParamNotFound = errors.New("not found")
)

type Service interface {
	// Proposer APIs (builder spec https://github.com/ethereum/builder-specs)
	RegisterValidatorSingular(ctx context.Context, payload structs.SignedValidatorRegistration) error
	RegisterValidator(context.Context, []structs.SignedValidatorRegistration) error
	GetHeader(context.Context, structs.HeaderRequest) (*types.GetHeaderResponse, error)
	GetPayload(context.Context, *types.SignedBlindedBeaconBlock) (*types.GetPayloadResponse, error)

	// Builder APIs (relay spec https://flashbots.notion.site/Relay-API-Spec-5fb0819366954962bc02e81cb33840f5)
	SubmitBlock(context.Context, *types.BuilderSubmitBlockRequest) error
	GetValidators() structs.BuilderGetValidatorsResponseEntrySlice

	// Data APIs
	GetPayloadDelivered(context.Context, structs.TraceQuery) ([]structs.BidTraceExtended, error)
	GetBlockReceived(context.Context, structs.TraceQuery) ([]structs.BidTraceWithTimestamp, error)
	Registration(context.Context, types.PublicKey) (types.SignedValidatorRegistration, error)
}

type API struct {
	l log.Logger
	s Service

	m APIMetrics
}

func NewApi(l log.Logger, s Service) (a *API) {
	a = &API{l: l, s: s}
	a.initMetrics()
	return a
}

func (a *API) AttachToHandler(m *http.ServeMux) {
	router := mux.NewRouter()
	router.Use(
		mux.CORSMethodMiddleware(router),
		withContentType("application/json"),
		withLogger(a.l)) // set middleware

	// root returns 200 - nil
	router.HandleFunc("/", status)

	// proposer related
	router.HandleFunc(PathStatus, status).Methods(http.MethodGet)
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

	router.Use(mux.CORSMethodMiddleware(router))
	m.Handle("/", router)
}

func handler(f func(http.ResponseWriter, *http.Request) (int, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := f(w, r)

		if status == 0 || status == http.StatusOK {
			status = http.StatusOK
		}

		w.WriteHeader(status)

		if err != nil {
			_ = json.NewEncoder(w).Encode(jsonError{
				Code:    status,
				Message: err.Error(),
			})
		}
	}
}

func status(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// proposer related handlers
func (a *API) registerValidator(w http.ResponseWriter, r *http.Request) (status int, err error) {
	timer := prometheus.NewTimer(a.m.ApiReqTiming.WithLabelValues("registerValidator"))
	defer timer.ObserveDuration()

	payload := []structs.SignedValidatorRegistration{}
	if err = json.NewDecoder(r.Body).Decode(&payload); err != nil {
		a.m.ApiReqCounter.WithLabelValues("registerValidator", "400").Inc()
		return http.StatusBadRequest, errors.New("invalid payload")
	}

	if payload != nil {
		a.m.ApiReqElCount.WithLabelValues("registerValidator", "payload").Observe(float64(len(payload)))
	}

	if len(payload) == 1 { // We don't need complex sync for just one payload
		if err = a.s.RegisterValidatorSingular(r.Context(), payload[0]); err != nil {
			a.m.ApiReqCounter.WithLabelValues("registerValidator", "200").Inc()
			return http.StatusOK, err
		}
	}

	if err = a.s.RegisterValidator(r.Context(), payload); err != nil {
		a.m.ApiReqCounter.WithLabelValues("registerValidator", "400").Inc()
		return http.StatusBadRequest, err
	}

	a.m.ApiReqCounter.WithLabelValues("registerValidator", "200").Inc()
	return http.StatusOK, err
}

func (a *API) getHeader(w http.ResponseWriter, r *http.Request) (int, error) {
	timer := prometheus.NewTimer(a.m.ApiReqTiming.WithLabelValues("getHeader"))
	defer timer.ObserveDuration()

	response, err := a.s.GetHeader(r.Context(), ParseHeaderRequest(r))
	if err != nil {
		a.m.ApiReqCounter.WithLabelValues("getHeader", "400").Inc()
		return http.StatusBadRequest, err
	}

	if err = json.NewEncoder(w).Encode(response); err != nil {
		a.l.WithError(err).WithField("path", r.URL.Path).Debug("failed to write response")
		a.m.ApiReqCounter.WithLabelValues("getHeader", "500").Inc()
		return http.StatusInternalServerError, err
	}

	a.m.ApiReqCounter.WithLabelValues("getHeader", "200").Inc()
	return http.StatusOK, nil
}

func (a *API) getPayload(w http.ResponseWriter, r *http.Request) (int, error) {
	timer := prometheus.NewTimer(a.m.ApiReqTiming.WithLabelValues("getPayload"))
	defer timer.ObserveDuration()

	var block types.SignedBlindedBeaconBlock
	if err := json.NewDecoder(r.Body).Decode(&block); err != nil {
		a.m.ApiReqCounter.WithLabelValues("getPayload", "400").Inc()
		return http.StatusBadRequest, errors.New("invalid payload")
	}

	payload, err := a.s.GetPayload(r.Context(), &block)
	if err != nil {
		a.m.ApiReqCounter.WithLabelValues("getPayload", "400").Inc()
		return http.StatusBadRequest, err
	}

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		a.l.WithError(err).WithField("path", r.URL.Path).Debug("failed to write response")
		a.m.ApiReqCounter.WithLabelValues("getPayload", "500").Inc()
		return http.StatusInternalServerError, err
	}

	a.m.ApiReqCounter.WithLabelValues("getPayload", "200").Inc()
	return http.StatusOK, nil
}

// builder related handlers
func (a *API) submitBlock(w http.ResponseWriter, r *http.Request) (int, error) {
	timer := prometheus.NewTimer(a.m.ApiReqTiming.WithLabelValues("submitBlock"))
	defer timer.ObserveDuration()
	var br types.BuilderSubmitBlockRequest
	if err := json.NewDecoder(r.Body).Decode(&br); err != nil {
		a.m.ApiReqCounter.WithLabelValues("submitBlock", "400").Inc()
		return http.StatusBadRequest, err
	}

	if br.ExecutionPayload != nil && br.ExecutionPayload.Transactions != nil {
		a.m.ApiReqElCount.WithLabelValues("submitBlock", "transaction").Observe(float64(len(br.ExecutionPayload.Transactions)))
	}

	if err := a.s.SubmitBlock(r.Context(), &br); err != nil {
		a.m.ApiReqCounter.WithLabelValues("submitBlock", "400").Inc()
		return http.StatusBadRequest, err
	}

	a.m.ApiReqCounter.WithLabelValues("submitBlock", "200").Inc()
	return http.StatusOK, nil
}

func (a *API) getValidators(w http.ResponseWriter, r *http.Request) (int, error) {
	timer := prometheus.NewTimer(a.m.ApiReqTiming.WithLabelValues("getValidators"))
	defer timer.ObserveDuration()

	vs := a.s.GetValidators()
	if vs == nil {
		a.l.Trace("no registered validators for epoch")
		vs = structs.BuilderGetValidatorsResponseEntrySlice{}
	}

	if vs != nil {
		a.m.ApiReqElCount.WithLabelValues("getValidators", "validator").Observe(float64(len(vs)))
	}

	if err := json.NewEncoder(w).Encode(vs); err != nil {
		a.m.ApiReqCounter.WithLabelValues("getValidators", "500").Inc()
		return http.StatusInternalServerError, err
	}

	a.m.ApiReqCounter.WithLabelValues("getValidators", "200").Inc()
	return http.StatusOK, nil
}

// data API related handlers
func (a *API) specificRegistration(w http.ResponseWriter, r *http.Request) (int, error) {
	timer := prometheus.NewTimer(a.m.ApiReqTiming.WithLabelValues("specificRegistration"))
	defer timer.ObserveDuration()

	pkStr := r.URL.Query().Get("pubkey")

	var pk types.PublicKey
	if err := pk.UnmarshalText([]byte(pkStr)); err != nil {
		a.m.ApiReqCounter.WithLabelValues("specificRegistration", "400").Inc()
		return http.StatusBadRequest, err
	}

	registration, err := a.s.Registration(r.Context(), pk)
	if err != nil {
		a.m.ApiReqCounter.WithLabelValues("specificRegistration", "500").Inc()
		return http.StatusInternalServerError, err
	}

	if err := json.NewEncoder(w).Encode(registration); err != nil {
		a.m.ApiReqCounter.WithLabelValues("specificRegistration", "500").Inc()
		return http.StatusInternalServerError, err
	}

	a.m.ApiReqCounter.WithLabelValues("specificRegistration", "200").Inc()
	return http.StatusOK, nil
}

func (a *API) proposerPayloadsDelivered(w http.ResponseWriter, r *http.Request) (int, error) {
	slot, err := specificSlot(r)
	if isInvalidParameter(err) {
		a.m.ApiReqCounter.WithLabelValues("proposerPayloadsDelivered", "400").Inc()
		return http.StatusBadRequest, err
	}

	bh, err := blockHash(r)
	if isInvalidParameter(err) {
		a.m.ApiReqCounter.WithLabelValues("proposerPayloadsDelivered", "400").Inc()
		return http.StatusBadRequest, err
	}

	bn, err := blockNumber(r)
	if isInvalidParameter(err) {
		a.m.ApiReqCounter.WithLabelValues("proposerPayloadsDelivered", "400").Inc()
		return http.StatusBadRequest, err
	}

	pk, err := publickKey(r)
	if isInvalidParameter(err) {
		a.m.ApiReqCounter.WithLabelValues("proposerPayloadsDelivered", "400").Inc()
		return http.StatusBadRequest, err
	}

	limit, err := limit(r)
	if isInvalidParameter(err) {
		a.m.ApiReqCounter.WithLabelValues("proposerPayloadsDelivered", "400").Inc()
		return http.StatusBadRequest, err
	} else if errors.Is(err, ErrParamNotFound) {
		limit = DataLimit
	}

	cursor, err := cursor(r)
	if isInvalidParameter(err) {
		a.m.ApiReqCounter.WithLabelValues("proposerPayloadsDelivered", "400").Inc()
		return http.StatusBadRequest, err
	}

	query := structs.TraceQuery{
		Slot:      slot,
		BlockHash: bh,
		BlockNum:  bn,
		Pubkey:    pk,
		Cursor:    cursor,
		Limit:     limit,
	}

	payloads, err := a.s.GetPayloadDelivered(r.Context(), query)
	if err != nil {
		a.m.ApiReqCounter.WithLabelValues("proposerPayloadsDelivered", "500").Inc()
		return http.StatusInternalServerError, err
	}

	if payloads != nil {
		a.m.ApiReqElCount.WithLabelValues("proposerPayloadsDelivered", "payload").Observe(float64(len(payloads)))
	}

	if err := json.NewEncoder(w).Encode(payloads); err != nil {
		a.m.ApiReqCounter.WithLabelValues("proposerPayloadsDelivered", "500").Inc()
		return http.StatusInternalServerError, err
	}

	a.m.ApiReqCounter.WithLabelValues("proposerPayloadsDelivered", "200").Inc()
	return http.StatusOK, nil
}

func (a *API) builderBlocksReceived(w http.ResponseWriter, r *http.Request) (int, error) {
	slot, err := specificSlot(r)
	if isInvalidParameter(err) {
		a.m.ApiReqCounter.WithLabelValues("builderBlocksReceived", "400").Inc()
		return http.StatusBadRequest, err
	}

	bh, err := blockHash(r)
	if isInvalidParameter(err) {
		a.m.ApiReqCounter.WithLabelValues("builderBlocksReceived", "400").Inc()
		return http.StatusBadRequest, err
	}

	bn, err := blockNumber(r)
	if isInvalidParameter(err) {
		a.m.ApiReqCounter.WithLabelValues("builderBlocksReceived", "400").Inc()
		return http.StatusBadRequest, err
	}

	limit, err := limit(r)
	if isInvalidParameter(err) {
		a.m.ApiReqCounter.WithLabelValues("builderBlocksReceived", "400").Inc()
		return http.StatusBadRequest, err
	} else if errors.Is(err, ErrParamNotFound) {
		limit = DataLimit
	}

	query := structs.TraceQuery{
		Slot:      slot,
		BlockHash: bh,
		BlockNum:  bn,
		Limit:     limit,
	}

	blocks, err := a.s.GetBlockReceived(r.Context(), query)
	if err != nil {
		a.m.ApiReqCounter.WithLabelValues("builderBlocksReceived", "500").Inc()
		return http.StatusInternalServerError, err
	}

	if blocks != nil {
		a.m.ApiReqElCount.WithLabelValues("builderBlocksReceived", "block").Observe(float64(len(blocks)))
	}

	if err := json.NewEncoder(w).Encode(blocks); err != nil {
		a.m.ApiReqCounter.WithLabelValues("builderBlocksReceived", "500").Inc()
		return http.StatusInternalServerError, err
	}

	a.m.ApiReqCounter.WithLabelValues("builderBlocksReceived", "200").Inc()
	return http.StatusOK, nil
}

func isInvalidParameter(err error) bool {
	return err != nil && !errors.Is(err, ErrParamNotFound)
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
