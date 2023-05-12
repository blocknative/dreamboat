package sim

import (
	"context"

	"github.com/lthibault/log"

	"github.com/blocknative/dreamboat/sim/client/fallback"
	"github.com/blocknative/dreamboat/sim/client/transport/gethhttp"
	"github.com/blocknative/dreamboat/sim/client/transport/gethrpc"
	"github.com/blocknative/dreamboat/sim/client/transport/gethws"
	"github.com/blocknative/dreamboat/sim/client/types"
)

const (
	gethSimNamespace = "flashbots"
)

type Client interface {
	ValidateBlock(ctx context.Context, block *types.BuilderBlockValidationRequest) (err error)
	ValidateBlockV2(ctx context.Context, block *types.BuilderBlockValidationRequestV2) (err error)
	Kind() string
}

type Fallback interface {
	AddClient()
}

type Manager struct {
	fb *fallback.Fallback
	l  log.Logger

	ws *gethws.ReConn
}

func NewManager(l log.Logger, fb *fallback.Fallback) (m *Manager) {
	m = &Manager{
		l:  l,
		fb: fb,
	}

	return m
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
	//if len(cfg.BlockSimulation.WS.Address) > 0 {//}
	//for _, s := range cfg.BlockSimulation.WS.Address {
	if m.ws == nil {
		m.ws = gethws.NewReConn(m.l)
		simWSCli := gethws.NewClient(m.ws, gethSimNamespace, retry, m.l)
		m.fb.AddClient(simWSCli)
	}

	input := make(chan []byte, 1000)
	go m.ws.KeepConnection(address, input)

}

func (m *Manager) AddHTTPClient(ctx context.Context, simHttpAddr string) {
	// if simHttpAddr := cfg.BlockSimulation.HTTP.Address; simHttpAddr != "" {	//}
	simHTTPCli := gethhttp.NewClient(simHttpAddr, gethSimNamespace, m.l)
	m.fb.AddClient(simHTTPCli)
}
