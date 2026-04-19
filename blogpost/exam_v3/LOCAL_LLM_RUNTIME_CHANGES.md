# Local runtime changes used for the clean exam_v3 rerun

These changes were made under `~/play/llama`, which is **not** a git repository on this machine. This file preserves the exact local code/config changes that were required to get the clean rerun done.

## 1) `~/play/llama/exam-driver.go`

Reason: this local `llama-swap` build accepts `POST /api/models/unload`; the older `/models/unload` path returned `404`.

Current `unload()`:

```go
func unload() {
	for _, path := range []string{"/api/models/unload", "/models/unload"} {
		r, err := http.Post(*fEndpoint+path, "", nil)
		if err != nil {
			continue
		}
		r.Body.Close()
		if r.StatusCode >= 200 && r.StatusCode < 300 {
			return
		}
	}
}
```

## 2) `~/play/llama/config.yaml`

### Global health-check timeout

Reason: `qwen3-coder-draft` was timing out during startup under the default `llama-swap` health-check window.

```yaml
healthCheckTimeout: 300
```

Placed near the top-level config, above `macros:`.

### `qwen3-coder-draft` context reduction

Reason: even after the startup timeout fix, the draft model remained non-viable for the full exam run at its previous context setting. For smoke retries it was reduced to the minimum acceptable context for this benchmark.

Current stanza:

```yaml
  qwen3-coder-draft:
    name: "Qwen3 Coder 30B-A3B (Q4_K_M) + Qwen3-0.6B draft"
    aliases:
      - "qwen3-coder-30b-draft"
    ttl: 300
    cmd: |
      ${llama-server} --port ${PORT}
      --model ${hf}/models--unsloth--Qwen3-Coder-30B-A3B-Instruct-GGUF/snapshots/b17cb02dd882d5b6ab62fc777ad2995f19668350/Qwen3-Coder-30B-A3B-Instruct-Q4_K_M.gguf
      --model-draft ${hf}/models--Qwen--Qwen3-0.6B-GGUF/snapshots/23749fefcc72300e3a2ad315e1317431b06b590a/Qwen3-0.6B-Q8_0.gguf
      --draft-max 16
      --ctx-size 32768
      --reasoning off
      ${common-flags}
```

## Notes

- These changes are documented here because they cannot be committed in place.
- `qwen3-coder-draft` still did not yield a valid scored run after these changes; see `exam_v3/REPORT.md` and the smoke logs for evidence.
