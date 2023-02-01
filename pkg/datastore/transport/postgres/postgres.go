package postgres

import (
	"database/sql"
	"time"

	_ "github.com/lib/pq"
)

func Open(dbURL string, maxOpen, maxIdle int, maxIdleTime time.Duration) (*sql.DB, error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxIdleTime(maxIdleTime)

	// test db connection
	if err = db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}
