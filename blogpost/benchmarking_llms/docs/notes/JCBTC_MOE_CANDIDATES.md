# jcbtc / ROCmFPX model catalogue — MoE candidates

_Date: 2026-08-03. Source: HF API (`/api/models?author=jcbtc`, per-repo `gguf` metadata + LFS pointers). All sizes/digests read from the API, not guessed._

## Why this doc exists

We ran `jcbtc/qwopus3.6-27b-v2-chadrock-rocmfp4-mtp` on the Framework 13 and got
**5.49 t/s** with MTP on. That model is **dense 27B** (`architecture: qwen35`,
27.32 B params, 13.79 GiB resident). On an 89.6 GB/s unified-memory bus a dense
27B reads ~13.8 GiB per token. The arithmetic caps it around 6 t/s; MTP got us
2.8x over the 1.97 t/s baseline and that is as good as it gets.

Both target machines want MoE:

| box | GPU | mem ceiling | why MoE |
|---|---|---|---|
| fw13 | Radeon 890M `gfx1150` | 62 GiB shared, ~89.6 GB/s | bandwidth-bound; only active experts get read |
| hopper | Radeon AI PRO R9700 `gfx1201` | **31.9 GiB VRAM**, host only 30 GiB (≈14 used) | model must fit VRAM; no room to spill |

The hopper constraint is the sharp one: **usable model budget is ~24–26 GiB**
(VRAM minus KV cache minus compute scratch). Anything ≥31 GiB is out.

## The 18-repo catalogue, split by architecture

### MoE, Qwen3.6-35B-A3B class (`qwen35moe`, 35.51 B total / 3 B active)

All four are the same base architecture, 262144 train context, MTP head embedded.

| repo | file | size | fits hopper VRAM | vision |
|---|---|---|---|---|
| `chadrock-35b-ace-saber-rocmfp4-mtp` | `Qwen3.6-35B-A3B-NSC-ACE-SABER-MTP-F16-to-ROCmFP4-STRIX_LEAN.gguf` | **19.047 GB** | ✅ | ✅ 0.903 GB mmproj |
| `CHADROCK3.6-35B-UNCENSORED-MTP-STRIX-LEAN` | `CHADROCK3.6-35B-UNCENSORED-MTP-STRIX-LEAN.gguf` | **19.047 GB** | ✅ | — |
| `qwen3.6-35b-a3b-crown-halo-mtp-dynamic` | `Qwen3.6-35B-A3B-HaloStrix-Dyn-MTP-v7.gguf` | **22.585 GB** | ✅ tight | ✅ 0.899 GB mmproj |
| `chadrock-35b-ace-saber-rocmfp4-mtp` | `CHADROCK-35B-Ace-Saber-MTP-ROCmFPX-MoEQuality-7.07BPW.gguf` | 31.411 GB | ❌ | ✅ |
| `chadrock3.6-27b-coder-rocmfp4-mtp` | `CHADROCK3.6-35B-A3B-Coder-MTP-ROCmFPX-MoEQuality-7.08BPW.gguf` | 31.412 GB | ❌ | ✅ 1.786 GB |
| `Ornith1.0-35b-CIRU-DUALVIEW-FPX7-Q8-MTP-GGUF` | `Ornith1.0-35b-CIRU-DUALVIEW-FPX7+Q8-MTP.gguf` | 33.537 GB | ❌ | ✅ 1.786 GB |

Digests (sha256 LFS oid prefix) for the three viable ones:

```
6a635d1d8ac4af8f  Qwen3.6-35B-A3B-NSC-ACE-SABER-MTP-F16-to-ROCmFP4-STRIX_LEAN.gguf  19.047 GB
32f40ebf853ee081  CHADROCK3.6-35B-UNCENSORED-MTP-STRIX-LEAN.gguf                    19.047 GB
342e3ee059792dbc  Qwen3.6-35B-A3B-HaloStrix-Dyn-MTP-v7.gguf                         22.585 GB
```

### Dense — what we already have, and its siblings

| repo | file | size | arch |
|---|---|---|---|
| `qwopus3.6-27b-v2-chadrock-rocmfp4-mtp` | `Qwopus3.6-27B-v2-...-STRIX_LEAN.gguf` | 14.817 GB | `qwen35` dense 27B — **already downloaded and measured** |
| `chadrock3.6-27b-coder-rocmfp4-mtp` | `CHADROCK3.6-27B-Coder-MTP-ROCmFP4-STRIX_LEAN.gguf` | 14.817 GB | dense 27B, coder-tuned |
| `chadrock3.6-27b-pi-agent-rocmfp4-mtp` | `CHADROCK3.6-27B-Pi-Agent-MTP-ROCmFP4-STRIX_LEAN.gguf` | 14.817 GB | dense 27B, tags: `pi-agent`, `no-thinking`, `tool-calling` |
| `chadrock3.6-40b-opus-deckard-...-rocmfp4` | 39.5 B params | — | dense 40B — slower than the 27B on both boxes |

Note the tag mismatch on `chadrock3.6-27b-coder`: it is tagged both `dense` and
`moe` because the repo holds a dense 27B **and** a 35B-A3B MoE file. HF's
`gguf.architecture` for that repo reports `qwen35` (the dense one). Don't trust
the repo-level metadata; read per-file.

The `pi-agent` variant is interesting for the agentic runbook (explicitly
tool-calling + no-thinking), but it is dense 27B — same bandwidth wall. Worth
one measurement on **hopper only**.

### Too big for either box

| repo | arch | total params | files |
|---|---|---|---|
| `Hy3-Chadrock-FPX-IFP2-MTP` | `hy_v3` (Hunyuan) | 298.8 B (295B-A21B) | 5 shards |
| `Step-3.7-Flash-ROCmFPX-Q3-QualityPlus` | `step35` | 197.0 B | 9 shards |
| `Laguna-S-2.1-Chadrock-ROCmFP4-StrixKVSpine-V4-GGUF` | `laguna` (poolside) | 117.6 B | 1 |
| `Laguna-S-2.1-NVFP4-GGUF` | `laguna` | 117.6 B | NVFP4 — Blackwell, not AMD |

Laguna-S-2.1 is the frustrating one: tagged `agentic-coding`, `tool-use`,
`long-context` — exactly our Phase 2 target — but 117 B params won't fit 32 GiB
VRAM at any usable quant, and the ROCmFP4 build targets `ryzen-ai-max-395`
(128 GB Strix Halo). Park it; revisit only if a smaller Laguna appears.

## Recommendation

**Download `Qwen3.6-35B-A3B-NSC-ACE-SABER-MTP-F16-to-ROCmFP4-STRIX_LEAN.gguf` (19.047 GB) first.**

Reasons, in order:

1. Same quant recipe (`ROCmFP4 ... STRIX_LEAN`) as the dense model we already
   validated end-to-end — the runtime path is proven, so a failure isolates to
   the model, not the stack.
2. 3 B active params vs 27 B dense. On fw13 that should move decode from 5.49
   t/s into the 25–40 t/s range (for calibration: Qwen3.6-35B-A3B UD-Q4_K_XL
   already does ~22 t/s on this laptop via plain Vulkan, and that is *without*
   MTP).
3. Fits hopper VRAM with ~12 GiB headroom for KV + scratch, so we can actually
   test long context there instead of the synthetic 8k.
4. MTP head embedded → the 2.8x decode multiplier we measured should compound
   with the MoE win, and MoE makes draft verification cheaper per token.
5. Has a vision mmproj if we later want it.

Second pick: `CHADROCK3.6-35B-UNCENSORED-MTP-STRIX-LEAN` — byte-identical size,
different tune, no vision. Useful as a same-size A/B on tune quality.

Third: `Crown-Halo-Dynamic` (22.585 GB) only if the 19 GB ones underperform —
it's a different (dynamic) quant recipe, so it isolates quant from tune.

## Open risk: ROCmFP4 on RDNA4 is unvalidated by the publisher

This is the one thing that could sink the hopper plan, and it is not resolved.

Verified from the source tree (`~/play/ROCmFPX-qwopus`, commit `7aa484a`):

- `ggml/src/ggml-cuda/vendors/hip.h:213` — `__GFX12__` → `RDNA4`; `gfx1150–gfx1153`
  → `RDNA3_5`. So `gfx1201` compiles down the RDNA4 path, **not** the RDNA3.5
  path the ROCmFP4 work was tuned on.
- `template-instances/mmq-instance-q4_0_rocmfp4.cu` has **no architecture guard** —
  the ROCmFP4 MMQ kernels instantiate for any target. So it should build.
- `mmvq.cu` has explicit `MMVQ_PARAMETERS_RDNA3_5` and `MMVQ_PARAMETERS_RDNA4`
  tables — RDNA4 gets its own tuned parameters, which is encouraging.
- `docs/BUILD-AMD-ARCHITECTURES.md:50` maps RDNA4 (`gfx1200`/`gfx1201`) to build
  target `gfx1200`, and states plainly: *"Published benchmark numbers and
  regression guards assume Strix Halo / `gfx1151`."*
- The quant ftypes are literally named `..._STRIX` / `..._STRIX_LEAN`
  (`ggml.h:486-487`, "Strix Halo quality/speed recipe"). The recipe is tuned for
  a bandwidth-starved APU. On a 32 GiB dGPU it may be leaving quality on the
  table for no reason.
- ROCm 7.2.1 `-complete` ships tensile/rocBLAS device libs for `gfx1201`
  (verified by listing the image), so the toolchain is not the blocker.

**Consequence for the runbook:** on hopper, correctness must be proven before any
throughput number is believed. `test-backend-ops` is the gate — and note our
current image does **not** contain it, because the Dockerfile change limited
build targets to `llama-server llama-bench`. Build once without that limit for
the RDNA4 validation pass.

Also note we built hopper for native `gfx1201`, not the documented `gfx1200`.
Native is the right default (it worked for `gfx1150` over the documented
`gfx1151`), but if `gfx1201` misbehaves, rebuilding at `gfx1200` is the first
thing to try.

**Status as of 2026-08-03:** the `gfx1201` image builds clean (419 s, no arch
errors) and HIP enumerates the R9700 as `ROCm0` with 32096 MiB free. So the
toolchain and device path are fine. What is still unproven is whether the
ROCmFP4 **MMQ kernels produce correct numerics** on RDNA4 — that needs
`test-backend-ops`, which our target-limited image does not contain. Until that
gate passes, treat any hopper throughput number as unverified.
