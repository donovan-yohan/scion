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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/GoogleCloudPlatform/scion/pkg/api"
	"github.com/GoogleCloudPlatform/scion/pkg/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestKubernetesRuntime_List(t *testing.T) {
	// Create a fake clientset
	clientset := k8sfake.NewClientset()

	// Create a pod that mimics what we expect
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
			Labels: map[string]string{
				"scion.name":     "test-agent",
				"scion.template": "test-template",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Image: "test-image",
				},
			},
		},
	}

	_, err := clientset.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	// Create a generic scheme for dynamic client
	scheme := k8sruntime.NewScheme()

	fc := fake.NewSimpleDynamicClient(scheme)

	client := k8s.NewTestClient(fc, clientset)
	r := NewKubernetesRuntime(client)

	agents, err := r.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
		return
	}

	if agents[0].ContainerID != "test-agent" {
		t.Errorf("expected ContainerID test-agent, got %s", agents[0].ContainerID)
	}

	if agents[0].ContainerStatus != "Running" {
		t.Errorf("expected container status Running, got %s", agents[0].ContainerStatus)
	}

	if agents[0].Image != "test-image" {
		t.Errorf("expected image test-image, got %s", agents[0].Image)
	}
}

func TestKubernetesRuntime_List_TerminalPhases(t *testing.T) {
	clientset := k8sfake.NewClientset()
	scheme := k8sruntime.NewScheme()
	fc := fake.NewSimpleDynamicClient(scheme)
	client := k8s.NewTestClient(fc, clientset)
	r := NewKubernetesRuntime(client)

	pods := []*corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "completed-agent",
				Namespace: "default",
				Labels: map[string]string{
					"scion.name": "completed-agent",
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodSucceeded,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "agent",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason:   "Completed",
								ExitCode: 0,
							},
						},
					},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Image: "test-image"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "failed-agent",
				Namespace: "default",
				Labels: map[string]string{
					"scion.name": "failed-agent",
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodFailed,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "agent",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason:   "Error",
								ExitCode: 1,
							},
						},
					},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Image: "test-image"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sync-held-agent",
				Namespace: "default",
				Labels: map[string]string{
					"scion.name": "sync-held-agent",
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "agent",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason:   "Completed",
								ExitCode: 0,
							},
						},
					},
					{
						Name: "sync-helper",
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
					},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Image: "test-image"}, {Image: "test-image"}},
			},
		},
	}

	for _, pod := range pods {
		if _, err := clientset.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{}); err != nil {
			t.Fatalf("failed to create pod %q: %v", pod.Name, err)
		}
	}

	agents, err := r.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	got := map[string]api.AgentInfo{}
	for _, agent := range agents {
		got[agent.Name] = agent
	}

	if got["completed-agent"].Phase != "stopped" {
		t.Errorf("completed-agent phase = %q, want %q", got["completed-agent"].Phase, "stopped")
	}
	if got["completed-agent"].ContainerStatus != "Succeeded (Completed)" {
		t.Errorf("completed-agent container status = %q, want %q", got["completed-agent"].ContainerStatus, "Succeeded (Completed)")
	}
	if got["failed-agent"].Phase != "error" {
		t.Errorf("failed-agent phase = %q, want %q", got["failed-agent"].Phase, "error")
	}
	if got["failed-agent"].ContainerStatus != "Failed (Error)" {
		t.Errorf("failed-agent container status = %q, want %q", got["failed-agent"].ContainerStatus, "Failed (Error)")
	}
	if got["sync-held-agent"].Phase != "stopped" {
		t.Errorf("sync-held-agent phase = %q, want %q", got["sync-held-agent"].Phase, "stopped")
	}
	if got["sync-held-agent"].ContainerStatus != "Succeeded (Completed)" {
		t.Errorf("sync-held-agent container status = %q, want %q", got["sync-held-agent"].ContainerStatus, "Succeeded (Completed)")
	}
}

func TestKubernetesRuntime_BuildPod_Env(t *testing.T) {
	clientset := k8sfake.NewClientset()
	scheme := k8sruntime.NewScheme()
	fc := fake.NewSimpleDynamicClient(scheme)
	client := k8s.NewTestClient(fc, clientset)
	r := NewKubernetesRuntime(client)

	config := RunConfig{
		Name:         "test-agent",
		Image:        "test-image",
		UnixUsername: "scion",
	}

	pod, _ := r.buildPod("default", config)

	foundUID := false
	foundGID := false
	foundHome := false
	foundUser := false
	foundLogname := false
	for _, env := range pod.Spec.Containers[0].Env {
		if env.Name == "SCION_HOST_UID" {
			foundUID = true
		}
		if env.Name == "SCION_HOST_GID" {
			foundGID = true
		}
		if env.Name == "HOME" && env.Value == "/home/scion" {
			foundHome = true
		}
		if env.Name == "USER" && env.Value == "scion" {
			foundUser = true
		}
		if env.Name == "LOGNAME" && env.Value == "scion" {
			foundLogname = true
		}
	}

	if !foundUID {
		t.Errorf("SCION_HOST_UID not found in pod env")
	}
	if !foundGID {
		t.Errorf("SCION_HOST_GID not found in pod env")
	}
	if !foundHome {
		t.Errorf("HOME not found in pod env")
	}
	if !foundUser {
		t.Errorf("USER not found in pod env")
	}
	if !foundLogname {
		t.Errorf("LOGNAME not found in pod env")
	}
}

func TestKubernetesRuntime_BuildPod_SyncHelperAndHomeVolume(t *testing.T) {
	clientset := k8sfake.NewClientset()
	scheme := k8sruntime.NewScheme()
	fc := fake.NewSimpleDynamicClient(scheme)
	client := k8s.NewTestClient(fc, clientset)
	r := NewKubernetesRuntime(client)

	config := RunConfig{
		Name:         "test-agent",
		Image:        "test-image",
		UnixUsername: "scion",
	}

	pod, err := r.buildPod("default", config)
	if err != nil {
		t.Fatalf("buildPod failed: %v", err)
	}

	if len(pod.Spec.InitContainers) != 1 {
		t.Fatalf("expected 1 init container, got %d", len(pod.Spec.InitContainers))
	}
	if pod.Spec.InitContainers[0].Name != "home-bootstrap" {
		t.Fatalf("expected home-bootstrap init container, got %q", pod.Spec.InitContainers[0].Name)
	}
	if len(pod.Spec.Containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(pod.Spec.Containers))
	}
	if pod.Spec.Containers[1].Name != "sync-helper" {
		t.Fatalf("expected sync-helper as second container, got %q", pod.Spec.Containers[1].Name)
	}

	volumeNames := make(map[string]bool, len(pod.Spec.Volumes))
	for _, volume := range pod.Spec.Volumes {
		volumeNames[volume.Name] = true
	}
	if !volumeNames["workspace"] {
		t.Fatal("expected workspace volume to be present")
	}
	if !volumeNames["home"] {
		t.Fatal("expected home volume to be present")
	}

	helperMounts := make(map[string]string, len(pod.Spec.Containers[1].VolumeMounts))
	for _, mount := range pod.Spec.Containers[1].VolumeMounts {
		helperMounts[mount.Name] = mount.MountPath
	}
	if helperMounts["workspace"] != "/workspace" {
		t.Fatalf("expected sync-helper workspace mount at /workspace, got %q", helperMounts["workspace"])
	}
	if helperMounts["home"] != "/home/scion" {
		t.Fatalf("expected sync-helper home mount at /home/scion, got %q", helperMounts["home"])
	}
}

func TestPersistK8sAgentInfoState(t *testing.T) {
	tmpDir := t.TempDir()
	infoPath := filepath.Join(tmpDir, "agent-info.json")
	initial := api.AgentInfo{
		Name:     "test-agent",
		Phase:    "running",
		Activity: "thinking",
	}
	data, err := json.MarshalIndent(initial, "", "  ")
	if err != nil {
		t.Fatalf("marshal initial info: %v", err)
	}
	if err := os.WriteFile(infoPath, data, 0644); err != nil {
		t.Fatalf("write initial info: %v", err)
	}

	if err := persistK8sAgentInfoState(infoPath, "stopped"); err != nil {
		t.Fatalf("persistK8sAgentInfoState failed: %v", err)
	}

	updatedData, err := os.ReadFile(infoPath)
	if err != nil {
		t.Fatalf("read updated info: %v", err)
	}
	var updated api.AgentInfo
	if err := json.Unmarshal(updatedData, &updated); err != nil {
		t.Fatalf("unmarshal updated info: %v", err)
	}
	if updated.Phase != "stopped" {
		t.Fatalf("updated phase = %q, want %q", updated.Phase, "stopped")
	}
	if updated.Activity != "" {
		t.Fatalf("updated activity = %q, want empty", updated.Activity)
	}
}
