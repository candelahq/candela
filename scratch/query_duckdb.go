package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/duckdb/duckdb-go/v2"
)

func main() {
	db, err := sql.Open("duckdb", "candela.duckdb")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query("SELECT DISTINCT user_id FROM spans")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = rows.Close() }()

	fmt.Println("Users in DuckDB:")
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("- %s\n", userID)
	}
}
