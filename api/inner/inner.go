// Would have been internal if only it wasnt reserved keyword
package inner

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/blocknative/dreamboat/structs"
	"github.com/flashbots/go-boost-utils/types"
)

type APIConfig interface {
	GetBool(key string) (bool, error)
	SetBool(key string, val bool) error
}

type GlobalConfig interface {
	GetConfigJSON() ([]byte, error)
}

type API struct {
	cfg APIConfig
	gc  GlobalConfig
}

func NewAPI(cfg APIConfig, gc GlobalConfig) *API {
	return &API{
		cfg: cfg,
		gc:  gc,
	}
}

func (a *API) AttachToHandler(m *http.ServeMux) {
	m.HandleFunc("/services/status", a.getStatus)
	m.HandleFunc("/services/config", a.getConfig)
	m.HandleFunc("/services/endpoints/set_availability", a.setAvailability)

	m.HandleFunc("/ ", a.getStatus)
}

func (a *API) getStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	gh, err1 := a.cfg.GetBool("getHeader")
	gp, err2 := a.cfg.GetBool("getPayload")
	sb, err3 := a.cfg.GetBool("submitBlock")
	if err1 != nil || err2 != nil || err3 != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "wrong configuration"}`))
		return
	}

	enc := json.NewEncoder(w)
	enc.Encode(Status{
		Services: ServiceStatus{
			GetHeader:   gh,
			GetPayload:  gp,
			SubmitBlock: sb,
		},
	})
}

func (a *API) getConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	b, err := a.gc.GetConfigJSON()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(b)
}

func (a *API) setAvailability(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	query := r.URL.Query()
	for k, v := range query {
		if len(v) != 1 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "wrong parameter count"}`))
			return
		}
		var val bool
		switch strings.ToLower(v[0]) {
		case "true", "1":
			val = true
		case "false", "0":
		default:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "wrong parameter"}`))
			return
		}

		if err := a.cfg.SetBool(k, val); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "key not found"}`))
			return
		}

	}
}

type Status struct {
	Services ServiceStatus `json:"endpoints"`
}

type ServiceStatus struct {
	GetHeader   bool `json:"getHeader"`
	GetPayload  bool `json:"getPayload"`
	SubmitBlock bool `json:"submitBlock"`
}

func specificSlot(r *http.Request) (structs.Slot, error) {
	if slotStr := r.URL.Query().Get("slot"); slotStr != "" {
		slot, err := strconv.ParseUint(slotStr, 10, 64)
		if err != nil {
			return structs.Slot(0), err
		}
		return structs.Slot(slot), nil
	}
	return structs.Slot(0), nil
}

func blockHash(r *http.Request) (types.Hash, error) {
	if bhStr := r.URL.Query().Get("block_hash"); bhStr != "" {
		var bh types.Hash
		if err := bh.UnmarshalText([]byte(bhStr)); err != nil {
			return bh, err
		}
		return bh, nil
	}
	return types.Hash{}, nil
}

func publickKey(r *http.Request) (types.PublicKey, error) {
	if pkStr := r.URL.Query().Get("proposer_pubkey"); pkStr != "" {
		var pk types.PublicKey
		if err := pk.UnmarshalText([]byte(pkStr)); err != nil {
			return pk, err
		}
		return pk, nil
	}
	return types.PublicKey{}, nil
}
