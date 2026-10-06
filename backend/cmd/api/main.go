package main

import (
	"log"
	"net/http"
	"time"
)

const version = "0.1.0"

type application struct {
	version   string
	startedAt time.Time
}

func main() {
	app := &application{
		version:   version,
		startedAt: time.Now(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", app.healthz)

	addr := ":8080"
	log.Printf("Agora API %s adresinde dinliyor.", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
