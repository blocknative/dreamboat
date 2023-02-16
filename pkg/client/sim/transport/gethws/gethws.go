package gethws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blocknative/dreamboat/pkg/client"
	"github.com/blocknative/dreamboat/pkg/client/sim/types"

	"github.com/gorilla/websocket"
	"github.com/lthibault/log"
)

const (
	healthInterval      = 2 * time.Second
	decodeWorkersNumber = 10
	workersQueueLen     = 50
)

type Conn struct {
	c  *websocket.Conn
	rc *RespCache

	l log.Logger

	input     chan []byte
	messageID uint64

	lastRead time.Time
	healthy  bool

	Done chan struct{}

	closeLock sync.Mutex
	isClosed  bool
}

func NewConn(l log.Logger, input chan []byte) *Conn {
	return &Conn{
		l:         l,
		messageID: 2,
		input:     input,
		Done:      make(chan struct{}),
		rc:        NewRespCache(),
	}
}

func (conn *Conn) EnqueueRPC(ctx context.Context, method string, content []byte) {
	select {
	case conn.input <- concatBytes(atomic.AddUint64(&conn.messageID, 1), method, content):
	case <-ctx.Done(): // allow to discard on full queue
	}
}

func (conn *Conn) RequestRPC(ctx context.Context, method string, content []byte) (b types.RpcRawResponse, err error) {
	respCh := conn.rc.PoolGet()
	defer conn.rc.PoolPut(respCh)
	id := atomic.AddUint64(&conn.messageID, 1)
	conn.rc.Set(id, respCh)
	conn.input <- concatBytes(id, method, content)

	select {
	case b = <-respCh:
	case <-ctx.Done():
		conn.rc.Del(id)
		return b, ctx.Err()
	}
	return b, err
}

func (conn *Conn) Close() {
	conn.closeLock.Lock()
	defer conn.closeLock.Unlock()

	if conn.isClosed {
		return
	}

	conn.isClosed = true
	conn.healthy = false
	conn.c.Close()
	close(conn.Done)
}

func (conn *Conn) Connect(url string) (err error) {
	conn.c, _, err = websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return err
	}

	go conn.readHandler()
	go conn.writeHandler()

	conn.healthy = true
	return nil
}

func (conn *Conn) rpcDecoder(in <-chan []byte) {
	readr := bytes.NewReader(nil)
	dec := json.NewDecoder(readr)
	for msg := range in {
		readr.Reset(msg)
		r := types.RpcRawResponse{}
		if err := dec.Decode(&r); err != nil {
			continue
		}

		if r.ID == 1 {
			continue
		}

		ch, ok := conn.rc.Get(r.ID)
		if !ok && r.Error != nil && r.Error.Message != "" {
			conn.l.With(log.F{
				"response": r,
			}).Error("error in async call")
			continue
		}
		ch <- r
	}
}

func (conn *Conn) readHandler() {
	// run parallel decoder
	ch := make(chan []byte, workersQueueLen)
	defer close(ch)
	for i := 0; i < decodeWorkersNumber; i++ {
		go conn.rpcDecoder(ch)
	}
	for {
		_, message, err := conn.c.ReadMessage() // why not NextReader? We allocate more but process jsonDecode faster in workers
		if err != nil {
			conn.l.WithError(err).Warn("error reading from ws")
			return
		}
		conn.lastRead = time.Now()
		ch <- message
	}
}

func (conn *Conn) writeHandler() {
	ticker := time.NewTicker(healthInterval)
	defer conn.Close()

	// allow to wait first full interval
	conn.lastRead = time.Now()
	for {
		select {
		case in := <-conn.input:
			err := conn.c.WriteMessage(websocket.TextMessage, in)
			if err != nil {
				conn.l.WithError(err).Warn("error writing from ws")
				return
			}
		case <-ticker.C:
			if time.Since(conn.lastRead) > healthInterval*2 {
				conn.l.Warn("ws timed out")
				return
			}

			err := conn.c.WriteMessage(websocket.TextMessage, []byte(`{"jsonrpc": "2.0", "id": 1, "method": "net_version"}`))
			if err != nil {
				conn.l.WithError(err).Warn("error writing from ws")
				return
			}
		case <-conn.Done:
			err := conn.c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				conn.l.WithError(err).Warn("error closing ws")
				return
			}
			conn.l.Info("closing connection")
			return
		}
	}
}

func (conn *Conn) Status() bool {
	return true
}

type ReConn struct {
	lock sync.RWMutex
	l    log.Logger

	c    []*Conn
	next uint32
}

func NewReConn(l log.Logger) *ReConn {
	return &ReConn{l: l}
}

func (rc *ReConn) KeepConnection(url string, input chan []byte) {

	var connID int
	for {
		_, cancel := context.WithCancel(context.Background())
		c := NewConn(rc.l, input)
		rc.lock.Lock()
		rc.c = append(rc.c, c)
		connID = len(rc.c)
		rc.lock.Unlock()

		err := c.Connect(url)
		if err != nil {
			rc.l.WithError(err).Warn("error connecting")
			rc.lock.Lock()
			if len(rc.c) == 1 {
				rc.c = []*Conn{}
			} else {
				rc.c[connID-1] = rc.c[len(rc.c)-1]
				rc.c = rc.c[:len(rc.c)-1]
			}
			rc.lock.Unlock()
			continue
		}

		<-c.Done
		cancel()

		rc.lock.Lock()
		if len(rc.c) == 1 {
			rc.c = []*Conn{}
		} else {
			rc.c[connID-1] = rc.c[len(rc.c)-1]
			rc.c = rc.c[:len(rc.c)-1]
		}
		rc.lock.Unlock()
	}
}

func (rc *ReConn) Next() *Conn {
	rc.lock.RLock()
	defer rc.lock.RUnlock()
	n := atomic.AddUint32(&rc.next, 1)
	return rc.c[(int(n)-1)%len(rc.c)]
}

func (rc *ReConn) Get() (*Conn, error) {
	c := rc.Next()
	if c == nil || !c.healthy {
		return nil, client.ErrConnectionFailure
	}

	return c, nil
}

func concatBytes(id uint64, method string, params []byte) []byte {
	b := bytes.NewBuffer(nil)
	fmt.Fprintf(b, `{"id": %d,"method": "%s","params": `, id, method)
	b.Write(params)
	b.WriteString(` }`)
	return b.Bytes()
}

type RespCache struct {
	c map[uint64]chan types.RpcRawResponse
	l sync.Mutex
	p sync.Pool
}

func NewRespCache() *RespCache {
	return &RespCache{
		c: make(map[uint64]chan types.RpcRawResponse),
		p: sync.Pool{
			New: func() any {
				return make(chan types.RpcRawResponse, 1)
			},
		},
	}
}

func (rc *RespCache) PoolGet() (ch chan types.RpcRawResponse) {
	return rc.p.Get().(chan types.RpcRawResponse)
}

func (rc *RespCache) PoolPut(ch chan types.RpcRawResponse) {
	rc.p.Put(ch)
}

func (rc *RespCache) Set(id uint64, ch chan types.RpcRawResponse) {
	rc.l.Lock()
	defer rc.l.Unlock()
	rc.c[id] = ch
}

func (rc *RespCache) Del(id uint64) {
	rc.l.Lock()
	defer rc.l.Unlock()
	delete(rc.c, id)
}

func (rc *RespCache) Get(id uint64) (ch chan types.RpcRawResponse, ok bool) {
	rc.l.Lock()
	defer rc.l.Unlock()
	ch, ok = rc.c[id]
	if ok {
		delete(rc.c, id)
	}
	return ch, ok
}
