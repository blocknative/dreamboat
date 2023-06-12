package dspostgres

import (
	"database/sql"

	"github.com/lthibault/log"
)

var (
	Emptybytes32 = [32]byte{}
	Emptybytes48 = [48]byte{}
)

var SlotsPerEpoch = 32

type Datastore struct {
	RelayID uint64
	DB      *sql.DB
	l       log.Logger
	m       PostgresMetrics
}

func NewDatastore(l log.Logger, db *sql.DB, relayID uint64) *Datastore {
	ds := &Datastore{DB: db, RelayID: relayID, l: l}
	ds.initMetrics()
	return ds
}
