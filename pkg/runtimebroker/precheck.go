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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/scion/pkg/api"
)

const (
	defaultPreCheckTimeout = 30 * time.Second
	defaultMaxOutputSize   = 10 * 1024   // 10KB
	hardMaxOutputSize      = 1024 * 1024 // 1MB
	stderrMaxSize          = 10 * 1024   // 10KB cap for stderr
)

// limitedWriter wraps a bytes.Buffer and enforces a size cap during writes,
// preventing unbounded memory growth from noisy subprocesses.
type limitedWriter struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	remaining := w.max - w.buf.Len()
	if remaining <= 0 {
		w.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		w.buf.Write(p[:remaining])
		w.truncated = true
		return len(p), nil
	}
	w.buf.Write(p)
	return len(p), nil
}

// executePreCheck runs a pre-flight check command on the broker.
// Returns (skipReason, stdout, error):
//   - On success (exit 0): ("", stdout, nil)
//   - On failure (non-zero or timeout): (reason, "", error)
func (s *Server) executePreCheck(ctx context.Context, cfg *api.PreCheckConfig, grovePath string, env map[string]string) (string, string, error) {
	timeout := defaultPreCheckTimeout
	if cfg.Timeout != "" {
		if d := api.ParseDuration(cfg.Timeout); d > 0 {
			timeout = d
		}
	}

	maxOutput := defaultMaxOutputSize
	if cfg.MaxOutputSize > 0 {
		maxOutput = cfg.MaxOutputSize
		if maxOutput > hardMaxOutputSize {
			maxOutput = hardMaxOutputSize
		}
	}

	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(checkCtx, "sh", "-c", cfg.Command)
	if grovePath != "" {
		cmd.Dir = grovePath
	}

	// Build env: deduplicate by building a map (base → agent env → pre_check env)
	envMap := make(map[string]string)
	for _, e := range os.Environ() {
		if k, v, ok := strings.Cut(e, "="); ok {
			envMap[k] = v
		}
	}
	for k, v := range env {
		envMap[k] = v
	}
	for k, v := range cfg.Env {
		envMap[k] = v
	}
	cmd.Env = make([]string, 0, len(envMap))
	for k, v := range envMap {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	stdout := &limitedWriter{max: maxOutput}
	stderr := &limitedWriter{max: stderrMaxSize}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if err != nil {
		var reason string
		if errors.Is(checkCtx.Err(), context.DeadlineExceeded) {
			reason = fmt.Sprintf("timed out after %s", timeout)
		} else {
			exitCode := -1
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
			reason = fmt.Sprintf("exit %d", exitCode)
			if stderr.buf.Len() > 0 {
				reason += ": " + strings.TrimSpace(stderr.buf.String())
			}
		}
		return reason, "", err
	}

	output := strings.TrimSpace(stdout.buf.String())
	if stdout.truncated {
		output += "\n[truncated at " + strconv.Itoa(maxOutput) + " bytes]"
	}

	return "", output, nil
}
