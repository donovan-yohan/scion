// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package runtime

import (
	"context"
	"testing"

	"github.com/GoogleCloudPlatform/scion/pkg/k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestCreateAgentNetworkPolicy_EgressRules(t *testing.T) {
	rt, clientset, _ := newTestK8sRuntime()

	spec := &v1alpha1.NetworkPolicySpec{
		Egress: []networkingv1.NetworkPolicyEgressRule{
			{
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Port:     portPtr(443),
						Protocol: protocolPtr(corev1.ProtocolTCP),
					},
				},
			},
		},
	}

	labels := map[string]string{
		"scion.name":  "test-agent",
		"scion.grove": "my-grove",
	}

	err := rt.createAgentNetworkPolicy(context.Background(), "default", "test-agent", spec, labels)
	if err != nil {
		t.Fatalf("createAgentNetworkPolicy failed: %v", err)
	}

	np, err := clientset.NetworkingV1().NetworkPolicies("default").Get(context.Background(), "scion-test-agent", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get created NetworkPolicy: %v", err)
	}

	// Verify pod selector targets the agent
	if np.Spec.PodSelector.MatchLabels["scion.name"] != "test-agent" {
		t.Errorf("expected pod selector scion.name=test-agent, got %v", np.Spec.PodSelector.MatchLabels)
	}

	// Verify egress rules
	if len(np.Spec.Egress) != 1 {
		t.Fatalf("expected 1 egress rule, got %d", len(np.Spec.Egress))
	}

	// Verify policy types include Egress only
	if len(np.Spec.PolicyTypes) != 1 || np.Spec.PolicyTypes[0] != networkingv1.PolicyTypeEgress {
		t.Errorf("expected PolicyTypes=[Egress], got %v", np.Spec.PolicyTypes)
	}

	// Verify scion labels are copied
	if np.Labels["scion.agent"] != "test-agent" {
		t.Errorf("expected label scion.agent=test-agent, got %v", np.Labels["scion.agent"])
	}
	if np.Labels["scion.grove"] != "my-grove" {
		t.Errorf("expected label scion.grove=my-grove, got %v", np.Labels["scion.grove"])
	}
}

func TestCreateAgentNetworkPolicy_IngressRules(t *testing.T) {
	rt, clientset, _ := newTestK8sRuntime()

	spec := &v1alpha1.NetworkPolicySpec{
		Ingress: []networkingv1.NetworkPolicyIngressRule{
			{
				From: []networkingv1.NetworkPolicyPeer{
					{
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"scion.name": "other-agent"},
						},
					},
				},
			},
		},
	}

	err := rt.createAgentNetworkPolicy(context.Background(), "default", "test-agent", spec, nil)
	if err != nil {
		t.Fatalf("createAgentNetworkPolicy failed: %v", err)
	}

	np, err := clientset.NetworkingV1().NetworkPolicies("default").Get(context.Background(), "scion-test-agent", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get created NetworkPolicy: %v", err)
	}

	if len(np.Spec.Ingress) != 1 {
		t.Fatalf("expected 1 ingress rule, got %d", len(np.Spec.Ingress))
	}

	if len(np.Spec.PolicyTypes) != 1 || np.Spec.PolicyTypes[0] != networkingv1.PolicyTypeIngress {
		t.Errorf("expected PolicyTypes=[Ingress], got %v", np.Spec.PolicyTypes)
	}
}

func TestCreateAgentNetworkPolicy_BothIngressAndEgress(t *testing.T) {
	rt, clientset, _ := newTestK8sRuntime()

	spec := &v1alpha1.NetworkPolicySpec{
		Ingress: []networkingv1.NetworkPolicyIngressRule{
			{},
		},
		Egress: []networkingv1.NetworkPolicyEgressRule{
			{
				Ports: []networkingv1.NetworkPolicyPort{
					{Port: portPtr(443), Protocol: protocolPtr(corev1.ProtocolTCP)},
				},
			},
		},
	}

	err := rt.createAgentNetworkPolicy(context.Background(), "default", "test-agent", spec, nil)
	if err != nil {
		t.Fatalf("createAgentNetworkPolicy failed: %v", err)
	}

	np, err := clientset.NetworkingV1().NetworkPolicies("default").Get(context.Background(), "scion-test-agent", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get created NetworkPolicy: %v", err)
	}

	if len(np.Spec.PolicyTypes) != 2 {
		t.Fatalf("expected 2 policy types, got %d", len(np.Spec.PolicyTypes))
	}
}

func TestCreateAgentNetworkPolicy_EmptySpecDeniesAll(t *testing.T) {
	rt, clientset, _ := newTestK8sRuntime()

	// Empty spec (no ingress/egress rules) should deny all traffic
	spec := &v1alpha1.NetworkPolicySpec{}

	err := rt.createAgentNetworkPolicy(context.Background(), "default", "test-agent", spec, nil)
	if err != nil {
		t.Fatalf("createAgentNetworkPolicy failed: %v", err)
	}

	np, err := clientset.NetworkingV1().NetworkPolicies("default").Get(context.Background(), "scion-test-agent", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get created NetworkPolicy: %v", err)
	}

	// Empty spec with both policy types = deny all
	if len(np.Spec.PolicyTypes) != 2 {
		t.Fatalf("expected 2 policy types (deny-all), got %d", len(np.Spec.PolicyTypes))
	}
	if len(np.Spec.Ingress) != 0 {
		t.Errorf("expected 0 ingress rules for deny-all, got %d", len(np.Spec.Ingress))
	}
	if len(np.Spec.Egress) != 0 {
		t.Errorf("expected 0 egress rules for deny-all, got %d", len(np.Spec.Egress))
	}
}

func TestCleanupAgentSecrets_IncludesNetworkPolicies(t *testing.T) {
	rt, clientset, _ := newTestK8sRuntime()

	// Create a NetworkPolicy that should be cleaned up
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scion-test-agent",
			Namespace: "default",
			Labels: map[string]string{
				"scion.agent": "test-agent",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"scion.name": "test-agent"},
			},
		},
	}
	_, err := clientset.NetworkingV1().NetworkPolicies("default").Create(context.Background(), np, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create test NetworkPolicy: %v", err)
	}

	// Verify it exists
	_, err = clientset.NetworkingV1().NetworkPolicies("default").Get(context.Background(), "scion-test-agent", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("NetworkPolicy should exist before cleanup: %v", err)
	}

	// Run cleanup
	rt.cleanupAgentSecrets(context.Background(), "default", "test-agent")

	// Verify it was deleted
	list, err := clientset.NetworkingV1().NetworkPolicies("default").List(context.Background(), metav1.ListOptions{
		LabelSelector: "scion.agent=test-agent",
	})
	if err != nil {
		t.Fatalf("failed to list NetworkPolicies: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected NetworkPolicy to be cleaned up, but %d remain", len(list.Items))
	}
}

func portPtr(port int) *intstr.IntOrString {
	p := intstr.FromInt32(int32(port))
	return &p
}

func protocolPtr(proto corev1.Protocol) *corev1.Protocol {
	return &proto
}
