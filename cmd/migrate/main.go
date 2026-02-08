package main

import (
	"context"
	"fmt"
	"os"

	"github.com/allyjweir/scoutmark/internal/database"
)

func main() {
	ctx := context.Background()

	db, err := database.Connect(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error connecting to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	dir := "migrations"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	if err := db.Migrate(ctx, dir); err != nil {
		fmt.Fprintf(os.Stderr, "error running migrations: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Migrations complete.")
}
