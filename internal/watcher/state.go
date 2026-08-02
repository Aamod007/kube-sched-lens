// Package watcher maintains in-memory indexed cluster state, filled either by
// client-go informers (live mode) or by embedded fixtures (demo mode).
package watcher

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
)

// State is the shared in-memory store. All maps are keyed by "namespace/name"
// (or plain name for cluster-scoped objects).
type State struct {
	mu        sync.RWMutex
	pods      map[string]*corev1.Pod
	events    map[string][]*corev1.Event // keyed by involvedObject ns/name
	nodes     map[string]*corev1.Node
	claims    map[string]*resourcev1.ResourceClaim
	slices    map[string]*resourcev1.ResourceSlice
	classes   map[string]*resourcev1.DeviceClass
	templates map[string]*resourcev1.ResourceClaimTemplate

	subMu sync.Mutex
	subs  []chan struct{}
}

func NewState() *State {
	return &State{
		pods:      map[string]*corev1.Pod{},
		events:    map[string][]*corev1.Event{},
		nodes:     map[string]*corev1.Node{},
		claims:    map[string]*resourcev1.ResourceClaim{},
		slices:    map[string]*resourcev1.ResourceSlice{},
		classes:   map[string]*resourcev1.DeviceClass{},
		templates: map[string]*resourcev1.ResourceClaimTemplate{},
	}
}

func key(ns, name string) string {
	if ns == "" {
		return name
	}
	return ns + "/" + name
}

// Subscribe returns a channel that receives a signal on every state change.
// Signals are best-effort (dropped if the receiver is slow).
func (s *State) Subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	s.subMu.Lock()
	s.subs = append(s.subs, ch)
	s.subMu.Unlock()
	return ch
}

func (s *State) notify() {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for _, ch := range s.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// --- setters/deleters ---

func (s *State) SetPod(p *corev1.Pod) {
	s.mu.Lock()
	s.pods[key(p.Namespace, p.Name)] = p
	s.mu.Unlock()
	s.notify()
}

func (s *State) DeletePod(ns, name string) {
	s.mu.Lock()
	delete(s.pods, key(ns, name))
	s.mu.Unlock()
	s.notify()
}

// SetEvent indexes an event under its involved object.
func (s *State) SetEvent(e *corev1.Event) {
	k := key(e.InvolvedObject.Namespace, e.InvolvedObject.Name)
	s.mu.Lock()
	list := s.events[k]
	replaced := false
	for i, old := range list {
		if old.Name == e.Name && old.Namespace == e.Namespace {
			list[i] = e
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, e)
	}
	s.events[k] = list
	s.mu.Unlock()
	s.notify()
}

func (s *State) SetNode(n *corev1.Node) {
	s.mu.Lock()
	s.nodes[n.Name] = n
	s.mu.Unlock()
	s.notify()
}

func (s *State) DeleteNode(name string) {
	s.mu.Lock()
	delete(s.nodes, name)
	s.mu.Unlock()
	s.notify()
}

func (s *State) SetClaim(c *resourcev1.ResourceClaim) {
	s.mu.Lock()
	s.claims[key(c.Namespace, c.Name)] = c
	s.mu.Unlock()
	s.notify()
}

func (s *State) DeleteClaim(ns, name string) {
	s.mu.Lock()
	delete(s.claims, key(ns, name))
	s.mu.Unlock()
	s.notify()
}

func (s *State) SetSlice(sl *resourcev1.ResourceSlice) {
	s.mu.Lock()
	s.slices[sl.Name] = sl
	s.mu.Unlock()
	s.notify()
}

func (s *State) DeleteSlice(name string) {
	s.mu.Lock()
	delete(s.slices, name)
	s.mu.Unlock()
	s.notify()
}

func (s *State) SetClass(c *resourcev1.DeviceClass) {
	s.mu.Lock()
	s.classes[c.Name] = c
	s.mu.Unlock()
	s.notify()
}

func (s *State) DeleteClass(name string) {
	s.mu.Lock()
	delete(s.classes, name)
	s.mu.Unlock()
	s.notify()
}

func (s *State) SetTemplate(t *resourcev1.ResourceClaimTemplate) {
	s.mu.Lock()
	s.templates[key(t.Namespace, t.Name)] = t
	s.mu.Unlock()
	s.notify()
}

// --- getters (return references; treat as read-only) ---

func (s *State) Pod(ns, name string) *corev1.Pod {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pods[key(ns, name)]
}

func (s *State) PendingPods() []*corev1.Pod {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*corev1.Pod
	for _, p := range s.pods {
		if p.Status.Phase == corev1.PodPending {
			out = append(out, p)
		}
	}
	return out
}

func (s *State) PodEvents(ns, name string) []*corev1.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]*corev1.Event(nil), s.events[key(ns, name)]...)
}

func (s *State) Claim(ns, name string) *resourcev1.ResourceClaim {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.claims[key(ns, name)]
}

func (s *State) Claims() []*resourcev1.ResourceClaim {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*resourcev1.ResourceClaim, 0, len(s.claims))
	for _, c := range s.claims {
		out = append(out, c)
	}
	return out
}

func (s *State) Slices() []*resourcev1.ResourceSlice {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*resourcev1.ResourceSlice, 0, len(s.slices))
	for _, sl := range s.slices {
		out = append(out, sl)
	}
	return out
}

func (s *State) Classes() map[string]*resourcev1.DeviceClass {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*resourcev1.DeviceClass, len(s.classes))
	for k, v := range s.classes {
		out[k] = v
	}
	return out
}

func (s *State) Nodes() []*corev1.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*corev1.Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		out = append(out, n)
	}
	return out
}
