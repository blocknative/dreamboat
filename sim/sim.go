package sim

import (
	"context"

	"github.com/lthibault/log"

	"github.com/blocknative/dreamboat/sim/client"
	"github.com/blocknative/dreamboat/sim/client/transport/gethhttp"
	"github.com/blocknative/dreamboat/sim/client/transport/gethrpc"
	"github.com/blocknative/dreamboat/sim/client/transport/gethws"
)

const (
	gethSimNamespace = "flashbots"
)

type Fallback interface {
	AddClient(cli client.Client)
}

type Manager struct {
	fb Fallback
	l  log.Logger

	ws *gethws.ReConn
}

func NewManager(l log.Logger, fb Fallback) (m *Manager) {
	return &Manager{
		l:  l,
		fb: fb,
	}
}

func (m *Manager) AddRPCClient(ctx context.Context, simHttpAddr string) {
	simRPCCli := gethrpc.NewClient(gethSimNamespace, simHttpAddr)
	if err := simRPCCli.Dial(ctx); err != nil {
		m.l.WithError(err).Fatalf("fail to initialize rpc connection (%s): %w", simHttpAddr, err)
		return
	}
	m.fb.AddClient(simRPCCli)
}

func (m *Manager) AddWsClients(ctx context.Context, address string, retry bool) {
	if m.ws == nil {
		m.ws = gethws.NewReConn(m.l)
		simWSCli := gethws.NewClient(m.ws, gethSimNamespace, retry, m.l)
		m.fb.AddClient(simWSCli)
	}

	input := make(chan []byte, 1000)
	go m.ws.KeepConnection(address, input)

}

func (m *Manager) AddHTTPClient(ctx context.Context, simHttpAddr string) {
	m.fb.AddClient(gethhttp.NewClient(simHttpAddr, gethSimNamespace, m.l))
}

/*
func (m *Manager) OnConfigChange(c structs.OldNew) (err error) {
	switch c.ParamPath {
	case "block_simulation.ws.address":
	case "block_simulation.http.address":
	case "block_simulation.rpc.address":
	}
	return nil
}
*/
