package db

import (
	"Intelligent_Dev_ToolKit_Odoo/internal/config"
	"context"
	"errors"
	"log"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

var testStore Store

func TestMain(m *testing.M) {
	config, err := config.LoadConfig("../..")
	if err != nil {
		log.Fatal("cannot load config:", err)
	}

	dbPool, err := pgxpool.New(context.Background(), config.DBSource)
	if err != nil {
		log.Fatal("Cannot connect to a database:", err)
	}
	defer dbPool.Close()

	// Run migrations
	migration, err := migrate.New(
		"file://../../db/migrations",
		config.DBSource,
	)
	if err != nil {
		log.Fatal("cannot create new migrate instance:", err)
	}

	if err = migration.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatal("failed to run migrate up:", err)
	}

	log.Println("db migrated successfully")

	testStore = NewStore(dbPool)

	exitCode := m.Run()

	// Optional: Drop everything for a clean slate next time
	if err = migration.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatal("failed to run migrate down:", err)
	}

	os.Exit(exitCode)
}
