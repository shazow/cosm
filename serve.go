package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/fvbock/endless"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
)

func serve(ctx context.Context, options Options) error {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	rtc := rtcServer{
		Logger: logger,
	}
	rtc.init()

	r.Get("/rtc", rtc.ServeHTTP)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/index.html")
	})

	bind := options.Serve.Bind
	fmt.Fprintf(os.Stderr, "listening on %s\n", bind)
	return endless.ListenAndServe(bind, r)
}
