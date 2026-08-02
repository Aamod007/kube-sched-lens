// Package demo loads embedded fixtures into the same State the live
// watchers would fill, so all endpoints work without a cluster.
package demo

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"

	"github.com/Aamod007/kube-sched-lens/internal/watcher"
)

//go:embed testdata/fixtures/*.json
var fixtures embed.FS

func loadInto[T any](name string, set func(*T)) error {
	b, err := fixtures.ReadFile("testdata/fixtures/" + name)
	if err != nil {
		return err
	}
	var list []T
	if err := json.Unmarshal(b, &list); err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}
	for i := range list {
		set(&list[i])
	}
	return nil
}

// Load fills state from the embedded fixtures.
func Load(state *watcher.State) error {
	if err := loadInto("nodes.json", func(o *corev1.Node) { state.SetNode(o) }); err != nil {
		return err
	}
	if err := loadInto("pods.json", func(o *corev1.Pod) { state.SetPod(o) }); err != nil {
		return err
	}
	if err := loadInto("events.json", func(o *corev1.Event) { state.SetEvent(o) }); err != nil {
		return err
	}
	if err := loadInto("resourceclaims.json", func(o *resourcev1.ResourceClaim) { state.SetClaim(o) }); err != nil {
		return err
	}
	if err := loadInto("resourceslices.json", func(o *resourcev1.ResourceSlice) { state.SetSlice(o) }); err != nil {
		return err
	}
	return loadInto("deviceclasses.json", func(o *resourcev1.DeviceClass) { state.SetClass(o) })
}

// Run loads fixtures and then toggles the allocated demo device between
// gpu-0 and gpu-1 every ~5s so WebSocket clients see live updates.
func Run(ctx context.Context, state *watcher.State) error {
	if err := Load(state); err != nil {
		return err
	}
	go func() {
		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()
		dev := "gpu-0"
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if dev == "gpu-0" {
					dev = "gpu-1"
				} else {
					dev = "gpu-0"
				}
				claim := state.Claim("default", "gpu-pod-claim")
				if claim == nil || claim.Status.Allocation == nil || len(claim.Status.Allocation.Devices.Results) == 0 {
					continue
				}
				c := claim.DeepCopy()
				c.Status.Allocation.Devices.Results[0].Device = dev
				state.SetClaim(c)
			}
		}
	}()
	return nil
}
