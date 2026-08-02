package watcher

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// Run starts informers for all watched types and blocks until ctx is done.
// kubeconfig may be empty to use the default clientcmd loading rules.
func Run(ctx context.Context, kubeconfig string, state *State) error {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		rules.ExplicitPath = kubeconfig
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).ClientConfig()
	if err != nil {
		return fmt.Errorf("load kubeconfig: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create clientset: %w", err)
	}

	factory := informers.NewSharedInformerFactory(cs, 30*time.Second)

	addHandler(factory.Core().V1().Pods().Informer(),
		func(o *corev1.Pod) { state.SetPod(o) },
		func(o *corev1.Pod) { state.DeletePod(o.Namespace, o.Name) })
	addHandler(factory.Core().V1().Events().Informer(),
		func(o *corev1.Event) { state.SetEvent(o) },
		func(o *corev1.Event) {}) // stale events are harmless; keep them
	addHandler(factory.Core().V1().Nodes().Informer(),
		func(o *corev1.Node) { state.SetNode(o) },
		func(o *corev1.Node) { state.DeleteNode(o.Name) })
	addHandler(factory.Resource().V1().ResourceClaims().Informer(),
		func(o *resourcev1.ResourceClaim) { state.SetClaim(o) },
		func(o *resourcev1.ResourceClaim) { state.DeleteClaim(o.Namespace, o.Name) })
	addHandler(factory.Resource().V1().ResourceSlices().Informer(),
		func(o *resourcev1.ResourceSlice) { state.SetSlice(o) },
		func(o *resourcev1.ResourceSlice) { state.DeleteSlice(o.Name) })
	addHandler(factory.Resource().V1().DeviceClasses().Informer(),
		func(o *resourcev1.DeviceClass) { state.SetClass(o) },
		func(o *resourcev1.DeviceClass) { state.DeleteClass(o.Name) })
	addHandler(factory.Resource().V1().ResourceClaimTemplates().Informer(),
		func(o *resourcev1.ResourceClaimTemplate) { state.SetTemplate(o) },
		func(o *resourcev1.ResourceClaimTemplate) {})

	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())
	<-ctx.Done()
	return nil
}

// addHandler wires typed add/update/delete callbacks onto an informer.
func addHandler[T any](inf cache.SharedIndexInformer, set func(T), del func(T)) {
	inf.AddEventHandler(cache.ResourceEventHandlerFuncs{ //nolint:errcheck
		AddFunc: func(obj any) {
			if o, ok := obj.(T); ok {
				set(o)
			}
		},
		UpdateFunc: func(_, obj any) {
			if o, ok := obj.(T); ok {
				set(o)
			}
		},
		DeleteFunc: func(obj any) {
			if ts, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = ts.Obj
			}
			if o, ok := obj.(T); ok {
				del(o)
			}
		},
	})
}
