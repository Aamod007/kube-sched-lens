// kube-sched-lens: GPU/accelerator scheduling debugger for Kubernetes DRA.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/Aamod007/kube-sched-lens/internal/api"
	"github.com/Aamod007/kube-sched-lens/internal/demo"
	"github.com/Aamod007/kube-sched-lens/internal/watcher"
)

func main() {
	port := flag.Int("port", 8151, "HTTP listen port")
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig (default: standard loading rules)")
	demoMode := flag.Bool("demo", false, "serve embedded fixtures instead of connecting to a cluster")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	state := watcher.NewState()

	if *demoMode {
		if err := demo.Run(ctx, state); err != nil {
			log.Fatalf("demo mode: %v", err)
		}
		log.Printf("demo mode: serving embedded fixtures")
	} else {
		go func() {
			if err := watcher.Run(ctx, *kubeconfig, state); err != nil {
				log.Fatalf("watcher: %v", err)
			}
		}()
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: api.Handler(state),
	}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background()) //nolint:errcheck
	}()

	log.Printf("kube-sched-lens listening on http://localhost:%d", *port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
