// Would have been internal if only it wasnt reserved keyword
package inner

import (
	"encoding/json"
	"net/http"
	"strings"
)

type APIConfig interface {
	GetBool(key string) (bool, error)
	SetBool(key string, val bool) error
}

type API struct {
	cfg APIConfig
}

func NewAPI(cfg APIConfig) *API {
	return &API{cfg: cfg}
}

func (a *API) AttachToHandler(m *http.ServeMux) {
	m.HandleFunc("/services/status", a.getStatus)
	m.HandleFunc("/services/set_availability", a.setAvailability)
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

func (a *API) setAvailability(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	query := r.URL.Query()
	for k, v := range query {
		if len(v) != 1 {
			w.Write([]byte(`{"error": "wrong parameter count"}`))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var val bool
		switch strings.ToLower(v[0]) {
		case "true":
			val = true
		case "false":
		default:
			w.Write([]byte(`{"error": "wrong parameter"}`))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := a.cfg.SetBool(k, val); err != nil {
			w.Write([]byte(`{"error": "key not found"}`))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

	}
}

type Status struct {
	Services ServiceStatus `json:"enabled_services"`
}

type ServiceStatus struct {
	GetHeader   bool `json:"getHeader"`
	GetPayload  bool `json:"getPayload"`
	SubmitBlock bool `json:"submitBlock"`
}
