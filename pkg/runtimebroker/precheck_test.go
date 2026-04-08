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

package runtimebroker

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/scion/pkg/api"
)

func newTestServerForPreCheck() *Server {
	return &Server{
		agentLifecycleLog: slog.Default(),
	}
}

func TestExecutePreCheck_ExitZero(t *testing.T) {
	s := newTestServerForPreCheck()
	cfg := &api.PreCheckConfig{Command: "echo hello world"}
	skipReason, output, err := s.executePreCheck(context.Background(), cfg, "", nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if skipReason != "" {
		t.Errorf("expected empty skipReason, got: %q", skipReason)
	}
	if output != "hello world" {
		t.Errorf("expected output %q, got %q", "hello world", output)
	}
}

func TestExecutePreCheck_ExitNonZero(t *testing.T) {
	s := newTestServerForPreCheck()
	cfg := &api.PreCheckConfig{Command: "echo fail >&2; exit 1"}
	skipReason, output, err := s.executePreCheck(context.Background(), cfg, "", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if output != "" {
		t.Errorf("expected empty output, got: %q", output)
	}
	if !strings.Contains(skipReason, "exit 1") {
		t.Errorf("expected skipReason to contain 'exit 1', got: %q", skipReason)
	}
	if !strings.Contains(skipReason, "fail") {
		t.Errorf("expected skipReason to contain stderr 'fail', got: %q", skipReason)
	}
}

func TestExecutePreCheck_Timeout(t *testing.T) {
	s := newTestServerForPreCheck()
	cfg := &api.PreCheckConfig{
		Command: "sleep 60",
		Timeout: "100ms",
	}
	skipReason, _, err := s.executePreCheck(context.Background(), cfg, "", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(skipReason, "timed out") {
		t.Errorf("expected skipReason to contain 'timed out', got: %q", skipReason)
	}
}

func TestExecutePreCheck_CustomTimeout(t *testing.T) {
	s := newTestServerForPreCheck()
	// Short timeout, but command completes instantly
	cfg := &api.PreCheckConfig{
		Command: "echo fast",
		Timeout: "5s",
	}
	skipReason, output, err := s.executePreCheck(context.Background(), cfg, "", nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if skipReason != "" {
		t.Errorf("expected empty skipReason, got: %q", skipReason)
	}
	if output != "fast" {
		t.Errorf("expected output %q, got %q", "fast", output)
	}
}

func TestExecutePreCheck_MaxOutputSize(t *testing.T) {
	s := newTestServerForPreCheck()
	// Generate output larger than 32 bytes
	cfg := &api.PreCheckConfig{
		Command:       "printf '%0.s_' {1..100}",
		MaxOutputSize: 32,
	}
	_, output, err := s.executePreCheck(context.Background(), cfg, "", nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(output) > 100 {
		t.Errorf("expected truncated output, got length %d", len(output))
	}
	if !strings.Contains(output, "[truncated at 32 bytes]") {
		t.Errorf("expected truncation note, got: %q", output)
	}
}

func TestExecutePreCheck_EnvInjection(t *testing.T) {
	s := newTestServerForPreCheck()
	cfg := &api.PreCheckConfig{
		Command: "echo $PRECHECK_TEST_VAR",
		Env: map[string]string{
			"PRECHECK_TEST_VAR": "injected_value",
		},
	}
	_, output, err := s.executePreCheck(context.Background(), cfg, "", nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output != "injected_value" {
		t.Errorf("expected output %q, got %q", "injected_value", output)
	}
}
