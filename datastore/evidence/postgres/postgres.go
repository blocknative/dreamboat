package dspostgres

import (
	"database/sql"
	"errors"
)

var SlotsPerEpoch = 32
var ErrNoRows = errors.New("no rows")

type Datastore struct {
	RelayID uint64
	DB      *sql.DB
}

func NewDatastore(db *sql.DB, relayID uint64) *Datastore {
	return &Datastore{
		DB:      db,
		RelayID: relayID}
}
