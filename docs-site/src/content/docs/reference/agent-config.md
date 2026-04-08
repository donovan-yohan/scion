---
title: Agent Configuration (scion-agent.yaml)
description: Reference for Scion agent templates and configuration.
---

The `scion-agent.yaml` file acts as the blueprint for an agent. It defines the environment, resources, and harness configuration required to run the agent.

## File Locations

- **Templates**: `.scion/templates/<template-name>/scion-agent.yaml`
- **Active Agents**: `.scion/agents/<agent-name>/scion-agent.yaml`

:::note[Format Change]
Previous versions of Scion used `scion-agent.json`. The new versioned settings system uses `scion-agent.yaml`, though JSON is still supported as valid YAML.
:::

## Configuration Fields

### Core Fields

| Field | Type | Description |
| :--- | :--- | :--- |
| `schema_version` | string | Should be `"1"`. |
| `default_harness_config` | string | The name of the default harness config to use (e.g., `gemini`, `claude`). |
| `agent_instructions` | string | Role-specific instructions for the agent (harness-agnostic). |
| `system_prompt` | string | The system prompt to use for the agent (harness-agnostic). |
| `image` | string | Override the container image defined in the harness config. |
| `env` | map | Environment variables to inject into the container. |
| `volumes` | list | Additional volume mounts. |
| `detached` | bool | Run in background (default `true`). |
| `command_args` | list | Additional arguments passed to the harness entrypoint. |
| `task_flag` | string | CLI flag name for passing the task (e.g., `--input`). When set, the task is delivered as a flag value instead of a positional argument. |
| `model` | string | LLM model identifier override. |
| `pre_check` | string/object | Pre-flight shell command that gates agent start. See [Pre-Check](#pre-check-pre_check). |

:::caution[Harness Field Deprecated]
The `harness` field is no longer supported in `scion-agent.yaml`. Templates must be harness-agnostic. Use `default_harness_config` to specify a preferred harness, which can be overridden by users at runtime.
:::

### Automatic Instruction Extensions

Scion automatically appends contextual instructions to the base `agent_instructions` during provisioning:

- **`agents-git.md`**: Appended if the agent is running in a Git-backed workspace. Provides operational context for git worktree and branch management.
- **`agents-hub.md`**: Appended if the agent is connected to a Scion Hub. Provides instructions for status reporting and Hub API interaction.

These extensions are managed by Scion and do not need to be manually included in your template definition.

### Limits & Resources

| Field | Type | Description |
| :--- | :--- | :--- |
| `max_turns` | int | Maximum number of LLM turns before the agent stops. Exceeding this triggers a `LIMITS_EXCEEDED` state and termination. |
| `max_duration` | string | Maximum runtime duration (e.g., `"2h"`, `"30m"`). Exceeding this triggers a `LIMITS_EXCEEDED` state and termination. |
| `resources` | object | Container resource requests/limits (see below). |

### Resource Specification

```yaml
resources:
  requests:
    cpu: "500m"
    memory: "512Mi"
  limits:
    cpu: "2"
    memory: "2Gi"
  disk: "10Gi"
```

### Pre-Check (`pre_check`)

A pre-flight check that runs a shell command on the broker **before** starting the agent container. If the command exits non-zero, the agent start is skipped entirely (no container created, no LLM tokens spent). This is useful for scheduled tasks that should only run when there is work to do.

Stdout from a successful check can be injected into the agent's task prompt, providing context gathered during the check (e.g., a list of issues to process).

**Full form:**

```yaml
pre_check:
  command: "gh issue list --repo my-org/my-repo --label backlog --state open --json number,title"
  timeout: "30s"              # default: 30s
  inject_output: true         # inject stdout into agent prompt (default: true)
  max_output_size: 10240      # bytes, default 10240 (10KB), hard limit 1048576 (1MB)
  env:                        # extra env vars for pre_check only
    EXTRA_VAR: "some-value"
```

**String shorthand** (command only, all other fields use defaults):

```yaml
pre_check: "gh issue list --repo my-org/my-repo --label backlog --json number --jq 'length > 0'"
```

| Field | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `command` | string | *required* | Shell command to execute (run via `sh -c`). Exit 0 proceeds; non-zero skips. |
| `timeout` | string | `"30s"` | Maximum time to wait for the command (e.g., `"30s"`, `"1m"`). |
| `inject_output` | bool | `true` | When true, inject stdout into the agent's task prompt. |
| `max_output_size` | int | `10240` | Maximum bytes of stdout to capture. Hard limit: 1MB. |
| `env` | map | | Additional environment variables for the pre-check only. The broker's own environment is already inherited. |

:::tip[Scheduled Tasks]
`pre_check` is especially powerful with recurring schedules. For example, a schedule that fires every 2 hours can use a pre-check to verify there are new GitHub issues before spinning up an agent to process them, saving LLM tokens on empty runs.
:::

:::note[Manual Starts]
When starting an agent manually via `scion create`, you can bypass the pre-check with `--skip-pre-check`. This is useful when you want to force-start an agent regardless of the check result.
:::

### Sidecar Services (`services`)

Define auxiliary containers to run alongside the agent (e.g., a headless browser).

```yaml
services:
  - name: browser
    command: ["chromium", "--headless"]
    env:
      DISPLAY: ":99"
    ready_check:
      type: tcp
      target: "localhost:9222"
  - name: delayed-job
    command: ["./worker.sh"]
    ready_check:
      type: delay
      target: "5s"
```

| Field | Type | Description |
| :--- | :--- | :--- |
| `name` | string | **Required**. Service name. |
| `command` | list | **Required**. Entrypoint and arguments. |
| `restart` | string | Restart policy: `no`, `always`, or `on-failure`. |
| `env` | map | Environment variables for the service. |
| `ready_check` | object | Health check to determine if the service is ready. |

#### Ready Check (`ready_check`)

| Field | Type | Description |
| :--- | :--- | :--- |
| `type` | string | `tcp`, `http`, or `delay`. |
| `target` | string | Host:port (tcp/http) or duration (delay). |
| `timeout` | string | Maximum wait time. |

### Hub Override (`hub`)

Specify a different Hub endpoint for this agent.

```yaml
hub:
  endpoint: "https://hub.example.com"
```

### Required Secrets (`secrets`)

Define secrets required by the agent. These follow the same schema as [Orchestrator Settings Secrets](/scion/reference/orchestrator-settings/#required-secrets).

### Gemini Settings (`gemini`)

Harness-specific settings for Gemini.

```yaml
gemini:
  auth_selectedType: "vertex-ai"
```

### Telemetry (`telemetry`)

Override telemetry settings for this template or agent. These merge on top of any telemetry configuration defined in `settings.yaml` (global or grove scope), using last-write-wins semantics.

```yaml
telemetry:
  enabled: true
  cloud:
    endpoint: "monitoring.googleapis.com:443"
  filter:
    events:
      exclude:
        - "agent.user.prompt"
  resource:
    service.name: "my-specialized-agent"
```

See the [Orchestrator Settings Reference](/scion/reference/orchestrator-settings/#telemetry-configuration-telemetry) for the full field reference and the [Metrics guide](/scion/hub-admin/metrics/#configuration-hierarchy) for how telemetry settings merge across scopes.

### Kubernetes Specifics (`kubernetes`)

Overrides for Kubernetes runtimes.

```yaml
kubernetes:
  namespace: "custom-ns"
  serviceAccountName: "workload-identity-sa"
  runtimeClassName: "gvisor"
```

## Resolution Logic

When an agent starts:

1.  **Template Load**: Scion loads `scion-agent.yaml` from the selected template.
2.  **Harness Resolution**: It resolves the `harness_config` against the active profile's `harness_configs` map in `settings.yaml`.
3.  **Overrides**: CLI flags (e.g., `--image`, `--env`) override values in `scion-agent.yaml`.
4.  **Final Config**: The resolved configuration is written to the agent's runtime directory.
