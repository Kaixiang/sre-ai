# sre-ai Agent Workflow Reference

This document describes the YAML workflow format used by `sre-ai agent run`. The goal is to make it simple to compose repeatable, human-auditable agent playbooks that combine LLM prompts, tool invocations, and post-processing in a consistent `collect ? plan ? act ? verify ? report` loop.

The MVP implementation focuses on two step types (`prompt`, `tool`) and sample/mock tooling so you can iterate on flows before wiring real integrations. The schema is intentionally declarative and safe-by-default: every step is explicit, output capture is opt-in, and dry-run/plan modes are first class.

---

## File Layout

```yaml
version: 0.1
name: <human-friendly name>
description: <what this workflow accomplishes>
agent:            # Optional overrides for model/provider defaults
inputs:           # User-supplied parameters and documentation
tools:            # Allowlisted tools referenced by steps
workflow:         # Ordered stages containing steps
  stages: [...]
outputs:          # Named artifacts rendered after execution (optional)
macros:           # Reusable step blocks (reserved for future use)
```

Every workflow lives in a single YAML file. Paths referenced inside the file are resolved relative to the workflow file location, so you can keep sample fixtures alongside the spec (see `workflows/sample_data/lark_thread.json`).

---

## Top-Level Metadata

| Field        | Required | Purpose |
|--------------|----------|---------|
| `version`    | ?        | Workflow schema version. Current MVP expects `0.1`.
| `name`       | ?        | Short identifier used in CLI output.
| `description`| ?        | Free-form summary displayed in CLI plan/apply output.

Example:

```yaml
version: 0.1
name: lark-oncall-rca
description: Analyze an incident chat thread and draft RCA materials.
```

---

## `agent` Block

Overrides runtime defaults the CLI would otherwise source from `--model`, `--provider`, or config files.

```yaml
agent:
  provider: gemini
  model: gemini-1.5-flash-latest
  temperature: 0.2   # optional, falls back to CLI/global default when omitted
```

Currently supported keys:

- `model`: LLM model id. Defaults to CLI/global setting or `gemini-1.5-flash-latest`.
- `provider`: Provider string (`gemini` in MVP). The runner loads the provider-specific client.
- `temperature`: Optional float overriding sampling temperature.

Additional knobs (caps, MCP attachments, env) are part of the design but not yet implemented in code; reserve them for future use.

---

## `inputs`

Declared inputs define what parameters a workflow expects. Each entry contains:

```yaml
inputs:
  thread_path:
    type: string
    description: Relative path to the Lark thread export JSON
    default: sample_data/lark_thread.json
  incident_goal:
    type: string
    description: Desired mitigation objective supplied by the operator
    required: false
```

Fields:

- `type`: Free-text type hint (`string`, `number`, `json`, etc.). Used for documentation only for now.
- `description`: Human-oriented guidance.
- `default`: Optional default value if the caller omits this input.
- `required`: Set to `false` to make the input optional. Missing required inputs cause `agent run` to fail fast.

At runtime, supply overrides via `--input key=value` (repeatable). These land in template contexts as `.inputs.<key>` or `index .inputs "key"`.

---

## `tools`

Tools act as an allowlist for callable resources. In the MVP they support `kind: sample` (static fixture data) so you can prototype without wiring real MCP servers. Each tool exposes one logical operation referenced by steps.

```yaml
tools:
  lark_thread:
    kind: sample
    description: Static export of a Lark incident conversation
    sample_file: sample_data/lark_thread.json
```

Supported fields:

| Field         | Required | Notes |
|---------------|----------|-------|
| `kind`        | ?        | Currently: `sample` (alias `mock`). Future kinds (`mcp`, `shell`, `http`, etc.) are reserved.
| `description` | ?        | Documentation only.
| `sample_file` | ?        | Path to a JSON file providing fake data. Relative paths resolve against workflow dir.
| `sample_data` | ?        | Inline JSON-compatible structure to return if no file is provided.

If a step passes `params.file`, it overrides `sample_file` at runtime, allowing fixture reuse.

---

## `workflow` ? `stages`

Workflows are executed stage-by-stage. Each stage models an agentic phase (collect, plan, act, verify, report). Order matters.

```yaml
workflow:
  stages:
    - id: collect_chat
      kind: collect
      description: Load the incident conversation
      steps:
        ...
```

Stage fields:

- `id`: Unique identifier for referencing stage outputs.
- `kind`: Free-form string used for documentation or future policy (e.g., `collect`, `plan`, `act`).
- `description`: Optional summary displayed in plan output.
- `steps`: Ordered list of step objects executed sequentially.

---

## Steps

Each step has a `type` that controls execution. The MVP supports two core types.

### Tool Step

Invokes one of the declared tools.

```yaml
- name: load_thread
  type: tool
  tool: lark_thread
  params:
    file: "{{ .inputs.thread_path }}"   # template interpolation
  capture:
    thread: data
```

Fields:

| Field        | Required | Description |
|--------------|----------|-------------|
| `name`       | ?        | Identifier; autogenerated if omitted.
| `type`       | ?        | Literal `tool`.
| `tool`       | ?        | Reference to a key in the `tools` map.
| `description`| ?        | Human docs.
| `params`     | ?        | Map of templated values passed to the tool (MVP sample tools only make use of `file` or `data`).
| `capture`    | ?        | Map of capture name ? JSON path within the tool result. The MVP returns `{"data": <payload>}` for sample tools, so `capture.thread: data` stores the entire fixture at `.steps.load_thread.thread`.

### Prompt Step

Sends a templated prompt to the configured model and stores the result.

```yaml
- name: summarize_thread
  type: prompt
  description: Extract key insights and timeline
  template: |
    You are the incident commander...
    {{- with index .inputs "incident_goal" }}
    User goal: {{ . }}
    {{- end }}

    Transcript:
    {{- range $msg := index .steps "load_thread" "thread" "conversation" }}
    - {{$msg.timestamp}} | {{$msg.user}} :: {{$msg.message}}
    {{- end }}
  expect:
    format: json
  capture:
    analysis: json
    raw_text: text
```

Fields:

| Field        | Required | Description |
|--------------|----------|-------------|
| `template`   | ?        | Go text/template string. Context exposes `.inputs` (map of resolved inputs) and `.steps` (per-step captured data, including `_raw`). Helper `toJSON` is available (`{{ toJSON .steps }}`).
| `expect`     | ?        | Structure describing expected output. MVP supports `format: json`, which attempts to parse the model response as JSON and stores it at `capture` key `json`.
| `capture`    | ?        | Map of alias ? path within the response payload. For prompts: `text` (raw string) is always available; `json` is set when `format: json` and parsing succeeds.

> ?? Ensure template lookups include the leading dot (`{{ .inputs.thread_path }}`) — omitting it leads to the `function "inputs" not defined` error you encountered earlier.

---

## Outputs

Outputs are optional named artifacts rendered after all stages succeed. They use the same templating rules as steps and can reference `.inputs` and `.steps`.

```yaml
outputs:
  timeline_markdown:
    template: |
      ## Incident Timeline
      {{- $timeline := index .steps "summarize_thread" "analysis" "timeline" }}
      {{- if $timeline }}
      {{- range $event := $timeline }}
      - **{{$event.time}}** ({{$event.actor}}): {{$event.event}}{{ if $event.notes }} — {{$event.notes}}{{ end }}
      {{- end }}
      {{- else }}
      - Timeline data not available.
      {{- end }}
```

When you run `sre-ai agent run --json`, the CLI emits a `Result` object:

```json
{
  "workflow": "lark-oncall-rca",
  "plan_only": false,
  "steps": [...],
  "outputs": {
    "timeline_markdown": "## Incident Timeline\n- ...",
    "rca_draft": "# RCA Draft\n..."
  }
}
```

You can redirect these strings into files or use tooling like `jq`/`yq` to extract them.

---

## Templating Cheat Sheet

Inside any `template` or templated `params` value you can rely on:

- `.inputs`: map of resolved workflow inputs.
- `.steps`: nested map keyed by step name ? captured values. Each step has `_raw` with the original map and, if `capture` was used, any aliases you defined.
- Control structures from Go templates (`{{ if }}`, `{{ range }}`, `{{ with }}`).
- Helper function `toJSON`: pretty-print arbitrary values.

Example snippet joining captured data:

```yaml
template: |
  {{- $analysis := index .steps "summarize_thread" "analysis" }}
  Summary: {{$analysis.summary}}
  {{- range $impact := $analysis.impact }}
  - {{$impact}}
  {{- end }}
```

---

## Design Patterns Supported Today

Even with the MVP primitives you can model several agentic patterns described in Phil Schmid’s “Agentic Patterns” blog post:

1. **Collection Stage** (`kind: collect`): Gather telemetry, logs, or fixture data via tool steps, preparing context for LLM reasoning.
2. **Planning Stage** (`kind: plan`): Run prompts that synthesize collected data into plans, hypotheses, or summaries. You can chain multiple prompt steps (e.g., plan plus self-critique) and capture results separately.
3. **Action Stage** (`kind: act`): Combine prompt steps that produce command recommendations with tool steps that would execute them (currently mock tools). Capture outputs to feed verification.
4. **Verification & Reporting** (`kind: verify` / `kind: report`): Use prompts to validate success criteria or craft human-facing reports via the `outputs` block.
5. **Template-driven Branching**: While explicit `branch` steps are not implemented yet, you can emulate decision logic inside prompt templates using `if`/`range` and capture results for downstream steps.

Planned enhancements include `reflect` steps (self-critique loops), `branch`/`loop` control structures, richer tool kinds (MCP stdio/http, shell commands), and guardrail policies (auto-confirm, dry-run-first). The current schema already reserves space (`macros`, `ExpectSpec`, `ToolSpec.kind`) so future versions will extend without breaking existing workflows.

---

## Example: Lark Incident RCA

Located at `workflows/lark_oncall.yaml`, this example demonstrates the full flow:

1. **Inputs**: `thread_path` (defaults to fixture) and optional `incident_goal`.
2. **Tools**: `lark_thread` returning the exported conversation.
3. **Stages**:
   - `collect_chat`: loads conversation via tool step.
   - `analyze_chat`: prompts Gemini to summarize and extract timeline/action items as structured JSON.
4. **Outputs**: Renders Markdown timeline and RCA draft using captured JSON.

Run it with plan-only mode to inspect the steps:

```powershell
sre-ai agent run --workflow workflows/lark_oncall.yaml --plan --json
```

Execute the full flow (needs a Gemini API key saved via `sre-ai config login --provider gemini`):

```powershell
sre-ai agent run --workflow workflows/lark_oncall.yaml --json \
  --input incident_goal="Restore checkout-api availability" \
  | jq -r '.outputs.rca_draft' > rca.md
```

---

## Authoring Checklist

1. Start with metadata (`version`, `name`, `description`).
2. Declare inputs users must pass; supply defaults for fixtures.
3. Register tools for the workflow (sample/mock during prototyping).
4. Sketch stages that mirror your runbook (collect ? plan ? act ? verify ? report).
5. For each step, decide whether it’s a tool call or an LLM prompt. Capture only the fields you need downstream.
6. Add outputs to transform captured state into artifacts (Markdown, JSON, etc.).
7. Validate with `--plan` first; switch to full execution once satisfied.
8. Share the workflow file alongside any sample data so teammates can iterate quickly.

With these building blocks you can encode bespoke on-call triage routines, repetitive RCA tasks, or other agentic workflows in a few dozen lines of YAML.
