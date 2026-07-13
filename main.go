package main

import (
	"database/sql"
	"log"
	"net/http"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	db, err := sql.Open("pgx", "postgres://postgres:devpass@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal("cannot reach database: ", err)
	}
	log.Println("connected to database")

	store := &Store{db: db}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /shorten", store.shortenHandler)
	mux.HandleFunc("GET /{code}", store.redirectHandler)

	log.Fatal(http.ListenAndServe(":8080", mux))
}
