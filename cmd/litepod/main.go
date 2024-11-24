package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/comp231-402-Team3-F24/internal/api"
	"github.com/comp231-402-Team3-F24/internal/pod"
	"github.com/comp231-402-Team3-F24/internal/runtime"
)

func main() {
	// Initialize container runtime
	rt, err := runtime.NewContainerdRuntime("/run/containerd/containerd.sock", "litepod")
	if err != nil {
		log.Fatalf("Failed to create runtime: %v", err)
	}

	defer func(rt *runtime.ContainerdRuntime) {
		err := rt.Close()
		if err != nil {

		}
	}(rt)

	pm := pod.NewManager(rt)
	server := api.NewServer(pm)
	mux := server.SetupRoutes()
	handler := api.CORS(api.Logger(api.Recoverer(mux)))

	srv := &http.Server{
		Addr:    ":8080",
		Handler: handler,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server starting on :8080")
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Wait for interrupt signal
	<-stop
	log.Printf("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown failed: %v", err)
	}

	log.Printf("Server stopped")
}
