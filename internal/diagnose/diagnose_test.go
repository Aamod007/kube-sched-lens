package diagnose

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Aamod007/kube-sched-lens/internal/watcher"
)

func ptr[T any](v T) *T { return &v }

// fixtureState builds a small cluster: 2 nodes, 2 slices with 2 gpus each,
// a gpu DeviceClass, plus whatever the test adds.
func fixtureState() *watcher.State {
	s := watcher.NewState()
	for _, node := range []string{"node-a", "node-b"} {
		s.SetNode(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: node}})
		s.SetSlice(&resourcev1.ResourceSlice{
			ObjectMeta: metav1.ObjectMeta{Name: node + "-gpu"},
			Spec: resourcev1.ResourceSliceSpec{
				Driver:   "gpu.example.com",
				Pool:     resourcev1.ResourcePool{Name: node},
				NodeName: ptr(node),
				Devices: []resourcev1.Device{
					{Name: "gpu-0", Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
						"model": {StringValue: ptr("LATEST-GPU-MODEL")},
					}},
					{Name: "gpu-1"},
				},
			},
		})
	}
	s.SetClass(&resourcev1.DeviceClass{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu.example.com"},
		Spec: resourcev1.DeviceClassSpec{
			Selectors: []resourcev1.DeviceSelector{
				{CEL: &resourcev1.CELDeviceSelector{Expression: `device.driver == "gpu.example.com"`}},
			},
		},
	})
	return s
}

func pendingPod(name string, claims ...corev1.PodResourceClaim) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       corev1.PodSpec{ResourceClaims: claims},
		Status:     corev1.PodStatus{Phase: corev1.PodPending},
	}
}

func claim(name, className string) *resourcev1.ResourceClaim {
	return &resourcev1.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: resourcev1.ResourceClaimSpec{
			Devices: resourcev1.DeviceClaim{
				Requests: []resourcev1.DeviceRequest{
					{Name: "req", Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: className}},
				},
			},
		},
	}
}

func allocate(c *resourcev1.ResourceClaim, pool, device string) *resourcev1.ResourceClaim {
	c.Status.Allocation = &resourcev1.AllocationResult{
		Devices: resourcev1.DeviceAllocationResult{
			Results: []resourcev1.DeviceRequestAllocationResult{
				{Request: "req", Driver: "gpu.example.com", Pool: pool, Device: device},
			},
		},
	}
	return c
}

func event(pod, reason, msg string) *corev1.Event {
	return &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: pod + ".evt", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: "default", Name: pod},
		Reason:         reason,
		Message:        msg,
	}
}

func TestDiagnose(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(s *watcher.State) *corev1.Pod
		wantCategory string
		wantInSummry string
	}{
		{
			name: "no-matching-device: deviceclass matches no slice",
			setup: func(s *watcher.State) *corev1.Pod {
				s.SetClass(&resourcev1.DeviceClass{
					ObjectMeta: metav1.ObjectMeta{Name: "fpga.example.com"},
					Spec: resourcev1.DeviceClassSpec{
						Selectors: []resourcev1.DeviceSelector{
							{CEL: &resourcev1.CELDeviceSelector{Expression: `device.driver == "fpga.example.com"`}},
						},
					},
				})
				s.SetClaim(claim("stuck", "fpga.example.com"))
				pod := pendingPod("p1", corev1.PodResourceClaim{Name: "c", ResourceClaimName: ptr("stuck")})
				s.SetPod(pod)
				return pod
			},
			wantCategory: "no-matching-device",
			wantInSummry: "no ResourceSlice publishes any device",
		},
		{
			name: "no-matching-device: deviceclass does not exist",
			setup: func(s *watcher.State) *corev1.Pod {
				s.SetClaim(claim("stuck", "ghost.example.com"))
				pod := pendingPod("p2", corev1.PodResourceClaim{Name: "c", ResourceClaimName: ptr("stuck")})
				s.SetPod(pod)
				return pod
			},
			wantCategory: "no-matching-device",
			wantInSummry: "does not exist",
		},
		{
			name: "insufficient-capacity: all matching devices allocated",
			setup: func(s *watcher.State) *corev1.Pod {
				// Allocate all 4 gpus via other claims.
				s.SetClaim(allocate(claim("a1", "gpu.example.com"), "node-a", "gpu-0"))
				s.SetClaim(allocate(claim("a2", "gpu.example.com"), "node-a", "gpu-1"))
				s.SetClaim(allocate(claim("a3", "gpu.example.com"), "node-b", "gpu-0"))
				s.SetClaim(allocate(claim("a4", "gpu.example.com"), "node-b", "gpu-1"))
				s.SetClaim(claim("wanting", "gpu.example.com"))
				pod := pendingPod("p3", corev1.PodResourceClaim{Name: "c", ResourceClaimName: ptr("wanting")})
				s.SetPod(pod)
				return pod
			},
			wantCategory: "insufficient-capacity",
			wantInSummry: "already allocated",
		},
		{
			name: "insufficient-capacity from event message",
			setup: func(s *watcher.State) *corev1.Pod {
				pod := pendingPod("p4")
				s.SetPod(pod)
				s.SetEvent(event("p4", "FailedScheduling", "0/2 nodes are available: 2 Insufficient cpu."))
				return pod
			},
			wantCategory: "insufficient-capacity",
			wantInSummry: "Insufficient cpu",
		},
		{
			name: "taint from event message",
			setup: func(s *watcher.State) *corev1.Pod {
				pod := pendingPod("p5")
				s.SetPod(pod)
				s.SetEvent(event("p5", "FailedScheduling",
					"0/2 nodes are available: 2 node(s) had untolerated taint {gpu: true}."))
				return pod
			},
			wantCategory: "taint",
		},
		{
			name: "affinity from event message",
			setup: func(s *watcher.State) *corev1.Pod {
				pod := pendingPod("p6")
				s.SetPod(pod)
				s.SetEvent(event("p6", "FailedScheduling",
					"0/2 nodes are available: 2 node(s) didn't match Pod's node affinity/selector."))
				return pod
			},
			wantCategory: "affinity",
		},
		{
			name: "unallocated-claim: claim exists, devices free, not yet allocated",
			setup: func(s *watcher.State) *corev1.Pod {
				s.SetClaim(claim("fresh", "gpu.example.com"))
				pod := pendingPod("p7", corev1.PodResourceClaim{Name: "c", ResourceClaimName: ptr("fresh")})
				s.SetPod(pod)
				return pod
			},
			wantCategory: "unallocated-claim",
		},
		{
			name: "unallocated-claim: referenced claim missing",
			setup: func(s *watcher.State) *corev1.Pod {
				pod := pendingPod("p8", corev1.PodResourceClaim{Name: "c", ResourceClaimName: ptr("nope")})
				s.SetPod(pod)
				return pod
			},
			wantCategory: "unallocated-claim",
			wantInSummry: "does not exist",
		},
		{
			name: "allocated claim, unrelated event -> unknown category from event",
			setup: func(s *watcher.State) *corev1.Pod {
				s.SetClaim(allocate(claim("ok", "gpu.example.com"), "node-a", "gpu-0"))
				pod := pendingPod("p9", corev1.PodResourceClaim{Name: "c", ResourceClaimName: ptr("ok")})
				s.SetPod(pod)
				s.SetEvent(event("p9", "FailedScheduling", "something odd happened"))
				return pod
			},
			wantCategory: "unknown",
		},
		{
			name: "template-generated claim resolved via pod status",
			setup: func(s *watcher.State) *corev1.Pod {
				s.SetClaim(claim("p10-gen-abc", "ghost.example.com"))
				pod := pendingPod("p10", corev1.PodResourceClaim{Name: "c", ResourceClaimTemplateName: ptr("tmpl")})
				pod.Status.ResourceClaimStatuses = []corev1.PodResourceClaimStatus{
					{Name: "c", ResourceClaimName: ptr("p10-gen-abc")},
				}
				s.SetPod(pod)
				return pod
			},
			wantCategory: "no-matching-device",
		},
		{
			name: "no events, no claims -> unknown",
			setup: func(s *watcher.State) *corev1.Pod {
				pod := pendingPod("p11")
				s.SetPod(pod)
				return pod
			},
			wantCategory: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := fixtureState()
			pod := tt.setup(s)
			d := Diagnose(s, pod)
			if d.Category != tt.wantCategory {
				t.Errorf("category = %q, want %q (summary: %s)", d.Category, tt.wantCategory, d.Summary)
			}
			if tt.wantInSummry != "" && !strings.Contains(d.Summary, tt.wantInSummry) {
				t.Errorf("summary %q does not contain %q", d.Summary, tt.wantInSummry)
			}
			if d.Summary == "" {
				t.Error("summary is empty")
			}
			if d.Suggestion == "" {
				t.Error("suggestion is empty")
			}
		})
	}
}

func TestCapacity(t *testing.T) {
	s := fixtureState()
	s.SetClaim(allocate(claim("a1", "gpu.example.com"), "node-a", "gpu-0"))

	got := Capacity(s)
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	// Sorted by driver then pool: node-a first.
	a := got[0]
	if a.Pool != "node-a" || a.Node != "node-a" || a.Driver != "gpu.example.com" {
		t.Errorf("unexpected first entry: %+v", a)
	}
	if a.DeviceCount != 2 || a.AllocatedCount != 1 {
		t.Errorf("node-a counts = %d/%d, want 2/1", a.DeviceCount, a.AllocatedCount)
	}
	if got[1].AllocatedCount != 0 {
		t.Errorf("node-b allocated = %d, want 0", got[1].AllocatedCount)
	}
	if a.Devices[0].Attributes["model"] != "LATEST-GPU-MODEL" {
		t.Errorf("attributes not flattened: %+v", a.Devices[0])
	}
}

func TestPendingPods(t *testing.T) {
	s := fixtureState()
	s.SetPod(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "running", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	})
	p := pendingPod("waiting")
	p.CreationTimestamp = metav1.Unix(1000, 0)
	s.SetPod(p)
	s.SetEvent(event("waiting", "FailedScheduling", "0/2 nodes are available: 2 Insufficient cpu."))

	got := PendingPods(s, func() int64 { return 1100 })
	if len(got) != 1 {
		t.Fatalf("got %d pending pods, want 1", len(got))
	}
	if got[0].Name != "waiting" || got[0].SinceSeconds != 100 || got[0].Category != "insufficient-capacity" {
		t.Errorf("unexpected row: %+v", got[0])
	}
}
