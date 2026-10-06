package main

import (
	"log"
	"net/http"
)

func main()  {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", healthz)

	addr := ":8080"
	log.Printf("Agora API %s adresinde dinliyor.", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}