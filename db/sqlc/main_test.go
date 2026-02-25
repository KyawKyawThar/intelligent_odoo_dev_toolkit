package db

import (
	"Intelligent_Dev_ToolKit_Odoo/utils"
	"context"
	"log"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

var testStore Store

func TestMain(m *testing.M) {

	config, err := utils.LoadConfig("../../deploy/")

	if err != nil {
		log.Fatal("cannot load config", err)
	}

	dbPool, err := pgxpool.New(context.Background(), config.DBSource)

	if err != nil {

		log.Fatal("Cannot connect to a database", err)
	}
	testStore = NewStore(dbPool)

	os.Exit(m.Run())
}
