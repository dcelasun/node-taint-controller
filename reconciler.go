package main

import (
	"context"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	TaintKey    = "node.kubernetes.io/out-of-service"
	TaintValue  = "nodeshutdown"
	TaintEffect = corev1.TaintEffectNoExecute
)

type NodeTaintReconciler struct {
	client.Client
	NotReadyThreshold time.Duration
	ReconcileInterval time.Duration
	Notifier          *Notifier

	mu            sync.Mutex
	NotReadySince map[string]time.Time
}

func (r *NodeTaintReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var node corev1.Node
	if err := r.Get(ctx, req.NamespacedName, &node); err != nil {
		if errors.IsNotFound(err) {
			r.clearTracking(req.Name)
			return ctrl.Result{}, nil
		}
		r.Notifier.Error("getting node "+req.Name, err)
		return ctrl.Result{}, err
	}

	isReady := r.isNodeReady(&node)
	hasTaint := r.hasOutOfServiceTaint(&node)

	if isReady {
		r.clearTracking(node.Name)

		if hasTaint {
			log.Info("node is Ready, removing out-of-service taint", "node", node.Name)
			if err := r.removeTaint(ctx, &node); err != nil {
				r.Notifier.Error("removing taint from "+node.Name, err)
				return ctrl.Result{}, err
			}
			r.Notifier.TaintRemoved(node.Name)
		}
		return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
	}

	// Node is NotReady
	notReadyDuration := r.trackNotReady(node.Name)

	if hasTaint {
		log.V(1).Info("node already tainted", "node", node.Name)
		return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
	}

	if notReadyDuration >= r.NotReadyThreshold {
		log.Info("node exceeded NotReady threshold, applying out-of-service taint",
			"node", node.Name,
			"notReadyDuration", notReadyDuration.Round(time.Second),
			"threshold", r.NotReadyThreshold,
		)
		if err := r.addTaint(ctx, &node); err != nil {
			r.Notifier.Error("adding taint to "+node.Name, err)
			return ctrl.Result{}, err
		}
		r.Notifier.TaintAdded(node.Name)
		return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
	}

	remaining := r.NotReadyThreshold - notReadyDuration
	log.V(1).Info("node is NotReady, waiting for threshold",
		"node", node.Name,
		"notReadyDuration", notReadyDuration.Round(time.Second),
		"remaining", remaining.Round(time.Second),
	)

	return ctrl.Result{RequeueAfter: remaining}, nil
}

func (r *NodeTaintReconciler) isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (r *NodeTaintReconciler) hasOutOfServiceTaint(node *corev1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == TaintKey && taint.Effect == TaintEffect {
			return true
		}
	}
	return false
}

func (r *NodeTaintReconciler) trackNotReady(nodeName string) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	if since, ok := r.NotReadySince[nodeName]; ok {
		return time.Since(since)
	}

	r.NotReadySince[nodeName] = time.Now()
	return 0
}

func (r *NodeTaintReconciler) clearTracking(nodeName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.NotReadySince, nodeName)
}

func (r *NodeTaintReconciler) addTaint(ctx context.Context, node *corev1.Node) error {
	patch := client.MergeFrom(node.DeepCopy())

	node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
		Key:    TaintKey,
		Value:  TaintValue,
		Effect: TaintEffect,
	})

	return r.Patch(ctx, node, patch)
}

func (r *NodeTaintReconciler) removeTaint(ctx context.Context, node *corev1.Node) error {
	patch := client.MergeFrom(node.DeepCopy())

	var newTaints []corev1.Taint
	for _, taint := range node.Spec.Taints {
		if taint.Key == TaintKey && taint.Effect == TaintEffect {
			continue
		}
		newTaints = append(newTaints, taint)
	}
	node.Spec.Taints = newTaints

	return r.Patch(ctx, node, patch)
}
