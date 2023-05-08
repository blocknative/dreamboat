package main

import (
	"io"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			log.Println("err:", err)
		}

		log.Println("request: ", r.Header, "     ", string(body))
		w.WriteHeader(400)
	})
	svr := http.Server{
		Addr:           "0.0.0.0:2345",
		Handler:        mux,
		MaxHeaderBytes: 4096,
	}

	if err := svr.ListenAndServe(); err != nil {
		log.Println(err)
	}
}
