// Package diagnose answers "why is my Pod Pending?" by joining scheduler
// events, ResourceClaims, ResourceSlices, DeviceClasses and node capacity.
package diagnose

import (
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"

	"github.com/Aamod007/kube-sched-lens/internal/watcher"
)

type EvidenceItem struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

type Diagnosis struct {
	Pod        string         `json:"pod"`
	Namespace  string         `json:"namespace"`
	Category   string         `json:"category"` // unallocated-claim | no-matching-device | insufficient-capacity | taint | affinity | unknown
	Summary    string         `json:"summary"`
	Evidence   []EvidenceItem `json:"evidence"`
	Suggestion string         `json:"suggestion"`
}

// Diagnose builds a Diagnosis for a pending pod. It never returns nil.
func Diagnose(state *watcher.State, pod *corev1.Pod) *Diagnosis {
	d := &Diagnosis{Pod: pod.Name, Namespace: pod.Namespace, Category: "unknown"}

	// 1. Scheduler events.
	var failMsg string
	for _, ev := range state.PodEvents(pod.Namespace, pod.Name) {
		if ev.Reason == "FailedScheduling" {
			failMsg = ev.Message
			d.Evidence = append(d.Evidence, EvidenceItem{Kind: "Event", Name: ev.Reason, Detail: ev.Message})
		}
	}

	// 2. Resolve pod resourceClaims -> ResourceClaim objects.
	claimProblem := diagnoseClaims(state, pod, d)

	// 3. Categorize. Claim problems win: they are the DRA-specific root cause;
	// the scheduler event for them is usually a generic "cannot allocate".
	switch {
	case claimProblem != "":
		d.Category = claimProblem
	case failMsg != "":
		d.Category = categorizeEventMessage(failMsg)
		d.Summary = fmt.Sprintf("Pod %s/%s cannot be scheduled: %s", pod.Namespace, pod.Name, failMsg)
	default:
		d.Summary = fmt.Sprintf("Pod %s/%s is Pending but no FailedScheduling event or claim problem was found; it may be waiting for image pull, volume binding, or a scheduler pass.", pod.Namespace, pod.Name)
	}

	if d.Suggestion == "" {
		d.Suggestion = suggestionFor(d.Category)
	}
	return d
}

// diagnoseClaims inspects each ResourceClaim referenced by the pod. Returns a
// category string if a claim-level problem was found, else "".
func diagnoseClaims(state *watcher.State, pod *corev1.Pod, d *Diagnosis) string {
	category := ""
	for _, prc := range pod.Spec.ResourceClaims {
		claimName := resolveClaimName(pod, prc)
		if claimName == "" {
			continue
		}
		claim := state.Claim(pod.Namespace, claimName)
		if claim == nil {
			d.Evidence = append(d.Evidence, EvidenceItem{Kind: "ResourceClaim", Name: claimName,
				Detail: "referenced by pod but not found in cluster"})
			d.Summary = fmt.Sprintf("Pod references ResourceClaim %q which does not exist.", claimName)
			category = "unallocated-claim"
			continue
		}

		if alloc := claim.Status.Allocation; alloc != nil {
			var devs []string
			for _, r := range alloc.Devices.Results {
				devs = append(devs, fmt.Sprintf("%s/%s/%s", r.Driver, r.Pool, r.Device))
			}
			d.Evidence = append(d.Evidence, EvidenceItem{Kind: "ResourceClaim", Name: claim.Name,
				Detail: "allocated: " + strings.Join(devs, ", ")})
			continue
		}

		// Unallocated: figure out why.
		cat, detail := whyUnallocated(state, claim)
		d.Evidence = append(d.Evidence, EvidenceItem{Kind: "ResourceClaim", Name: claim.Name, Detail: detail})
		d.Summary = fmt.Sprintf("Pod is waiting for ResourceClaim %q: %s", claim.Name, detail)
		// no-matching-device is more specific than unallocated-claim; keep the most specific.
		if category == "" || cat != "unallocated-claim" {
			category = cat
		}
	}
	return category
}

// resolveClaimName maps a PodResourceClaim to the actual ResourceClaim name.
func resolveClaimName(pod *corev1.Pod, prc corev1.PodResourceClaim) string {
	if prc.ResourceClaimName != nil {
		return *prc.ResourceClaimName
	}
	// Template-generated: the real name is recorded in pod status.
	for _, st := range pod.Status.ResourceClaimStatuses {
		if st.Name == prc.Name && st.ResourceClaimName != nil {
			return *st.ResourceClaimName
		}
	}
	return ""
}

// whyUnallocated determines why a claim has no allocation.
func whyUnallocated(state *watcher.State, claim *resourcev1.ResourceClaim) (category, detail string) {
	classes := state.Classes()
	slices := state.Slices()

	for _, req := range claim.Spec.Devices.Requests {
		if req.Exactly == nil {
			continue // FirstAvailable subrequests: too complex to reason about here
		}
		className := req.Exactly.DeviceClassName
		if _, ok := classes[className]; !ok {
			return "no-matching-device",
				fmt.Sprintf("request %q references DeviceClass %q which does not exist", req.Name, className)
		}
		total, free := countDevices(state, slices, className, classes)
		if total == 0 {
			return "no-matching-device",
				fmt.Sprintf("no ResourceSlice publishes any device matching DeviceClass %q", className)
		}
		if free == 0 {
			return "insufficient-capacity",
				fmt.Sprintf("all %d devices matching DeviceClass %q are already allocated", total, className)
		}
	}
	return "unallocated-claim", "claim is unallocated; scheduler has not (yet) found a placement"
}

// countDevices counts devices matching a DeviceClass across all slices, and
// how many of those are not consumed by an existing allocation.
//
// ponytail: we do not evaluate CEL selectors. Heuristic: a class with no
// selectors matches everything; a class whose CEL expression contains
// `device.driver == "<x>"` matches slices of that driver. Good enough for
// the dra-example-driver pattern; plug in cel-go if real expressions appear.
func countDevices(state *watcher.State, slices []*resourcev1.ResourceSlice, className string, classes map[string]*resourcev1.DeviceClass) (total, free int) {
	class := classes[className]
	allocated := allocatedDevices(state)
	for _, sl := range slices {
		if !classMatchesDriver(class, sl.Spec.Driver) {
			continue
		}
		for _, dev := range sl.Spec.Devices {
			total++
			if !allocated[sl.Spec.Driver+"/"+sl.Spec.Pool.Name+"/"+dev.Name] {
				free++
			}
		}
	}
	return total, free
}

// classMatchesDriver reports whether a DeviceClass could match devices of the
// given driver, using the string-match heuristic described on countDevices.
func classMatchesDriver(class *resourcev1.DeviceClass, driver string) bool {
	if class == nil {
		return false
	}
	if len(class.Spec.Selectors) == 0 {
		return true
	}
	for _, sel := range class.Spec.Selectors {
		if sel.CEL != nil && strings.Contains(sel.CEL.Expression, `"`+driver+`"`) {
			return true
		}
	}
	return false
}

// allocatedDevices returns the set of "driver/pool/device" keys consumed by
// allocated claims.
func allocatedDevices(state *watcher.State) map[string]bool {
	out := map[string]bool{}
	for _, c := range state.Claims() {
		if c.Status.Allocation == nil {
			continue
		}
		for _, r := range c.Status.Allocation.Devices.Results {
			out[r.Driver+"/"+r.Pool+"/"+r.Device] = true
		}
	}
	return out
}

// categorizeEventMessage maps a FailedScheduling message to a category.
func categorizeEventMessage(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "insufficient"):
		return "insufficient-capacity"
	case strings.Contains(lower, "taint") || strings.Contains(lower, "untolerated"):
		return "taint"
	case strings.Contains(lower, "affinity") || strings.Contains(lower, "didn't match pod's node selector"),
		strings.Contains(lower, "node selector"):
		return "affinity"
	case strings.Contains(lower, "cannot allocate") || strings.Contains(lower, "resourceclaim"):
		return "unallocated-claim"
	default:
		return "unknown"
	}
}

func suggestionFor(category string) string {
	switch category {
	case "no-matching-device":
		return "Check that the DRA driver is running and publishing ResourceSlices, and that the claim's deviceClassName and selectors match an existing device."
	case "insufficient-capacity":
		return "All matching devices are in use. Free up devices by deleting pods holding allocations, or add nodes/devices."
	case "taint":
		return "Add a matching toleration to the pod, or remove the taint from the node."
	case "affinity":
		return "Relax the pod's nodeSelector/affinity rules, or label a node to match."
	case "unallocated-claim":
		return "Inspect the ResourceClaim status and scheduler logs; the claim exists but has not been allocated."
	default:
		return "Check 'kubectl describe pod' for details."
	}
}

// PendingSummary is the list-view row for a pending pod.
type PendingSummary struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	SinceSeconds int64  `json:"sinceSeconds"`
	Category     string `json:"category"`
	Summary      string `json:"summary"`
}

// PendingPods returns one summary row per pending pod, sorted for stable output.
func PendingPods(state *watcher.State, now func() int64) []PendingSummary {
	var out []PendingSummary
	for _, p := range state.PendingPods() {
		d := Diagnose(state, p)
		since := int64(0)
		if !p.CreationTimestamp.IsZero() {
			since = now() - p.CreationTimestamp.Unix()
			if since < 0 {
				since = 0
			}
		}
		out = append(out, PendingSummary{
			Namespace: p.Namespace, Name: p.Name,
			SinceSeconds: since, Category: d.Category, Summary: d.Summary,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// DeviceInfo is one device in a capacity report.
type DeviceInfo struct {
	Name       string            `json:"name"`
	Attributes map[string]string `json:"attributes"`
}

// CapacityEntry summarizes one ResourceSlice pool.
type CapacityEntry struct {
	Driver         string       `json:"driver"`
	Pool           string       `json:"pool"`
	Node           string       `json:"node"`
	DeviceCount    int          `json:"deviceCount"`
	AllocatedCount int          `json:"allocatedCount"`
	Devices        []DeviceInfo `json:"devices"`
}

// Capacity derives per-driver/per-pool totals from ResourceSlices plus
// allocated claims.
func Capacity(state *watcher.State) []CapacityEntry {
	allocated := allocatedDevices(state)
	var out []CapacityEntry
	for _, sl := range state.Slices() {
		node := ""
		if sl.Spec.NodeName != nil {
			node = *sl.Spec.NodeName
		}
		e := CapacityEntry{Driver: sl.Spec.Driver, Pool: sl.Spec.Pool.Name, Node: node}
		for _, dev := range sl.Spec.Devices {
			e.DeviceCount++
			if allocated[sl.Spec.Driver+"/"+sl.Spec.Pool.Name+"/"+dev.Name] {
				e.AllocatedCount++
			}
			e.Devices = append(e.Devices, DeviceInfo{Name: dev.Name, Attributes: flattenAttributes(dev.Attributes)})
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Driver != out[j].Driver {
			return out[i].Driver < out[j].Driver
		}
		return out[i].Pool < out[j].Pool
	})
	return out
}

// flattenAttributes renders DeviceAttribute union values as strings.
func flattenAttributes(attrs map[resourcev1.QualifiedName]resourcev1.DeviceAttribute) map[string]string {
	if len(attrs) == 0 {
		return nil
	}
	out := make(map[string]string, len(attrs))
	for k, v := range attrs {
		switch {
		case v.StringValue != nil:
			out[string(k)] = *v.StringValue
		case v.IntValue != nil:
			out[string(k)] = fmt.Sprintf("%d", *v.IntValue)
		case v.BoolValue != nil:
			out[string(k)] = fmt.Sprintf("%t", *v.BoolValue)
		case v.VersionValue != nil:
			out[string(k)] = *v.VersionValue
		}
	}
	return out
}
