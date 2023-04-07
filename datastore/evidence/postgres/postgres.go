package dspostgres

import (
	"database/sql"
)

var (
	Emptybytes32 = [32]byte{}
	Emptybytes48 = [48]byte{}
)

type Datastore struct {
	RelayID uint64
	DB      *sql.DB
}

func NewDatastore(db *sql.DB, relayID uint64) *Datastore {
	return &Datastore{DB: db, RelayID: relayID}
}
