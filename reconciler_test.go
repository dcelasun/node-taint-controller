package main

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	return s
}

func newNode(name string, ready corev1.ConditionStatus, taints ...corev1.Taint) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.NodeSpec{
			Taints: taints,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: ready,
				},
			},
		},
	}
}

func outOfServiceTaint() corev1.Taint {
	return corev1.Taint{
		Key:    TaintKey,
		Value:  TaintValue,
		Effect: TaintEffect,
	}
}

func TestIsNodeReady(t *testing.T) {
	r := &NodeTaintReconciler{}

	tests := []struct {
		name     string
		node     *corev1.Node
		expected bool
	}{
		{
			name:     "node is ready",
			node:     newNode("test", corev1.ConditionTrue),
			expected: true,
		},
		{
			name:     "node is not ready",
			node:     newNode("test", corev1.ConditionFalse),
			expected: false,
		},
		{
			name:     "node ready status unknown",
			node:     newNode("test", corev1.ConditionUnknown),
			expected: false,
		},
		{
			name: "node has no ready condition",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{}},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.isNodeReady(tt.node); got != tt.expected {
				t.Errorf("isNodeReady() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHasOutOfServiceTaint(t *testing.T) {
	r := &NodeTaintReconciler{}

	tests := []struct {
		name     string
		node     *corev1.Node
		expected bool
	}{
		{
			name:     "has out-of-service taint",
			node:     newNode("test", corev1.ConditionFalse, outOfServiceTaint()),
			expected: true,
		},
		{
			name:     "no taints",
			node:     newNode("test", corev1.ConditionFalse),
			expected: false,
		},
		{
			name: "has other taint",
			node: newNode("test", corev1.ConditionFalse, corev1.Taint{
				Key:    "node.kubernetes.io/unschedulable",
				Effect: corev1.TaintEffectNoSchedule,
			}),
			expected: false,
		},
		{
			name: "has taint with same key but different effect",
			node: newNode("test", corev1.ConditionFalse, corev1.Taint{
				Key:    TaintKey,
				Value:  TaintValue,
				Effect: corev1.TaintEffectNoSchedule,
			}),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.hasOutOfServiceTaint(tt.node); got != tt.expected {
				t.Errorf("hasOutOfServiceTaint() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTrackNotReady(t *testing.T) {
	r := &NodeTaintReconciler{
		NotReadySince: make(map[string]time.Time),
	}

	// First call should return 0 and start tracking
	d1 := r.trackNotReady("node1")
	if d1 != 0 {
		t.Errorf("first trackNotReady() = %v, want 0", d1)
	}

	if _, ok := r.NotReadySince["node1"]; !ok {
		t.Error("node1 should be tracked")
	}

	// Small sleep to ensure time passes
	time.Sleep(10 * time.Millisecond)

	// Second call should return positive duration
	d2 := r.trackNotReady("node1")
	if d2 <= 0 {
		t.Errorf("second trackNotReady() = %v, want > 0", d2)
	}

	// Clear tracking
	r.clearTracking("node1")
	if _, ok := r.NotReadySince["node1"]; ok {
		t.Error("node1 should not be tracked after clear")
	}
}

func TestReconcile_NotReadyNodeGetsTainted(t *testing.T) {
	node := newNode("test-node", corev1.ConditionFalse)
	client := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(node).
		Build()

	r := &NodeTaintReconciler{
		Client:            client,
		NotReadyThreshold: 50 * time.Millisecond,
		ReconcileInterval: 30 * time.Second,
		NotReadySince:     make(map[string]time.Time),
	}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-node"}}

	// First reconcile: starts tracking, doesn't taint yet
	result, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Error("expected positive RequeueAfter")
	}

	var updated corev1.Node
	if err := client.Get(ctx, req.NamespacedName, &updated); err != nil {
		t.Fatal(err)
	}
	if r.hasOutOfServiceTaint(&updated) {
		t.Error("node should not be tainted yet")
	}

	// Wait for threshold
	time.Sleep(60 * time.Millisecond)

	// Second reconcile: should apply taint
	_, err = r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if err := client.Get(ctx, req.NamespacedName, &updated); err != nil {
		t.Fatal(err)
	}
	if !r.hasOutOfServiceTaint(&updated) {
		t.Error("node should be tainted after threshold")
	}
}

func TestReconcile_ReadyNodeGetsTaintRemoved(t *testing.T) {
	node := newNode("test-node", corev1.ConditionTrue, outOfServiceTaint())
	client := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(node).
		Build()

	r := &NodeTaintReconciler{
		Client:            client,
		NotReadyThreshold: 5 * time.Minute,
		ReconcileInterval: 30 * time.Second,
		NotReadySince:     make(map[string]time.Time),
	}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-node"}}

	_, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var updated corev1.Node
	if err := client.Get(ctx, req.NamespacedName, &updated); err != nil {
		t.Fatal(err)
	}
	if r.hasOutOfServiceTaint(&updated) {
		t.Error("taint should be removed from ready node")
	}
}

func TestReconcile_ReadyNodeClearsTracking(t *testing.T) {
	node := newNode("test-node", corev1.ConditionTrue)
	client := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(node).
		Build()

	r := &NodeTaintReconciler{
		Client:            client,
		NotReadyThreshold: 5 * time.Minute,
		ReconcileInterval: 30 * time.Second,
		NotReadySince: map[string]time.Time{
			"test-node": time.Now().Add(-10 * time.Minute),
		},
	}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-node"}}

	_, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if _, ok := r.NotReadySince["test-node"]; ok {
		t.Error("tracking should be cleared for ready node")
	}
}

func TestReconcile_AlreadyTaintedNodeStaysTainted(t *testing.T) {
	node := newNode("test-node", corev1.ConditionFalse, outOfServiceTaint())
	client := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithObjects(node).
		Build()

	r := &NodeTaintReconciler{
		Client:            client,
		NotReadyThreshold: 5 * time.Minute,
		ReconcileInterval: 30 * time.Second,
		NotReadySince:     make(map[string]time.Time),
	}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-node"}}

	_, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var updated corev1.Node
	if err := client.Get(ctx, req.NamespacedName, &updated); err != nil {
		t.Fatal(err)
	}

	taintCount := 0
	for _, taint := range updated.Spec.Taints {
		if taint.Key == TaintKey {
			taintCount++
		}
	}
	if taintCount != 1 {
		t.Errorf("expected exactly 1 out-of-service taint, got %d", taintCount)
	}
}

func TestReconcile_DeletedNodeClearsTracking(t *testing.T) {
	client := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		Build()

	r := &NodeTaintReconciler{
		Client:            client,
		NotReadyThreshold: 5 * time.Minute,
		ReconcileInterval: 30 * time.Second,
		NotReadySince: map[string]time.Time{
			"deleted-node": time.Now().Add(-10 * time.Minute),
		},
	}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "deleted-node"}}

	_, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if _, ok := r.NotReadySince["deleted-node"]; ok {
		t.Error("tracking should be cleared for deleted node")
	}
}
