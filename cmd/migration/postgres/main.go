package main

import (
	"database/sql"
	"embed"
	"errors"
	"flag"
	"log"
	"os"

	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"

	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*
var content embed.FS

type flags struct {
	databaseURL   string
	migrationType string
	version       uint
	verbose       bool
}

var cf = flags{}

func init() {
	flag.StringVar(&cf.databaseURL, "db", "", "Database URL")
	flag.StringVar(&cf.migrationType, "migrations", "", `Type of migration (available: "validators")`)
	flag.BoolVar(&cf.verbose, "verbose", true, "Verbosity of logs during run")
	flag.UintVar(&cf.version, "version", 0, "Version parameter sets db changes to specified revision (up or down)")
	flag.Parse()
}

func main() {
	log.SetOutput(os.Stdout)
	// Initialize configuration
	if cf.databaseURL == "" {
		log.Fatal(errors.New("database url is not set"))
	}

	if err := migrateDB(cf.databaseURL); err != nil {
		log.Fatal(err)
	}
}

func migrateDB(dburl string) error {
	db, err := sql.Open("postgres", dburl)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if cf.migrationType != "validators" { // it will have more
		log.Fatal("migration type (`migrations`) is not properly set")
	}

	d, err := iofs.New(content, "migrations/"+cf.migrationType)
	if err != nil {
		log.Fatal(err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", d, dburl)
	if err != nil {
		return err
	}
	defer m.Close()

	if cf.version > 0 {
		if cf.verbose {
			log.Println("Migrating to version: ", cf.version)
		}

		if err := m.Migrate(cf.version); err != nil {
			return err
		}
	} else {
		err = m.Up()
	}

	if err != nil {
		if err != migrate.ErrNoChange {
			return err
		}
		if cf.verbose {
			log.Println("No change")
		}
	}
	return nil
}
