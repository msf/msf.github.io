# ROCmFPX / Qwopus setup — reproducible instructions

_Date: 2026-08-03. Status: build + HIP device access verified on both Framework 13 (`gfx1150`) and hopper / R9700 (`gfx1201`). Inference measured on fw13 only; hopper correctness gate not yet run._

ROCmFP4 GGUFs from `jcbtc` **cannot be loaded by stock llama.cpp**. The quant types
(`GGML_TYPE_Q4_0_ROCMFP4`, ftype `..._STRIX_LEAN`) only exist in the ROCmFPX fork.
Verified: hopper's `llama-b9189` binary contains no `ROCMFP4` symbols.

Everything below runs in a container. No ROCm/HIP is installed on either host.

## 1. Source

```
repo:   ssh://git@github.com/ciru-ai/ROCmFPX.git
commit: 7aa484a2f0a504dc612a3d74a068024f3e6d6353   # pinned
local:  ~/play/ROCmFPX-qwopus
hopper: ~/play/ROCmFPX-qwopus                      # rsync'd, same commit
```

One local modification to `.devops/strix-rocmfp4.Dockerfile` (4 insertions):
adds `ARG JOBS`, passes it to the build script, limits build targets to
`llama-server llama-bench`, and copies `llama-bench` into the `server` stage.
Without the target limit the image builds all test binaries; without `JOBS` it
uses `nproc` and OOMs/thrashes a 12-thread box.

## 2. Pick the `gfx` target — do not use the Dockerfile default

The Dockerfile defaults to `gfx1151` (Strix **Halo**). That is not our hardware.

```bash
cat /sys/class/kfd/kfd/topology/nodes/*/properties | grep gfx_target_version
# 110500 -> gfx1150   Framework 13, Radeon 890M (RDNA3.5)
# 120001 -> gfx1201   hopper card0, Radeon AI PRO R9700 (RDNA4)
# 110003 -> gfx1103   hopper card1, Radeon 740M iGPU  (ignore)
```

Verified: `rocm/dev-ubuntu-24.04:7.2.1-complete` ships tensile/rocBLAS device
libs for `gfx1030 gfx1100 gfx1101 gfx1102 gfx1150 gfx1151 gfx1200 gfx1201`
(+ CDNA). So both targets are buildable from that base image.

## 3. Build

```bash
cd ~/play/ROCmFPX-qwopus
docker build \
  --build-arg CMAKE_HIP_ARCHITECTURES=gfx1150 \   # gfx1201 on hopper
  --build-arg JOBS=6 \
  --target server \
  -t local/rocmfpx-qwopus:gfx1150 \
  -f .devops/strix-rocmfp4.Dockerfile .
```

Measured:

| box | target | jobs | compile | image |
|---|---|---|---|---|
| fw13 (24 threads, `--cpu-quota=600000`) | `gfx1150` | 6 | 716 s | 22.06 GB |
| hopper (12 threads, unrestricted) | `gfx1201` | 4 | 419 s | 22.1 GB |

The ROCm `-complete` base dominates image size. Compiler emits infinity/fast-math
warnings — expected, not a failure. `gfx1201` compiled clean with no
architecture errors, so the RDNA4 code path at least builds.

Use `--cpu-quota` if you want the box usable during the build.

## 4. Verify HIP passthrough before downloading anything

```bash
docker run --rm \
  --device=/dev/kfd --device=/dev/dri \
  --group-add video --group-add render \
  --security-opt seccomp=unconfined \
  --entrypoint /app/llama-bench local/rocmfpx-qwopus:gfx1150 --list-devices
```

Framework 13, verified output:

```
ROCm0:   AMD Radeon 890M Graphics (31787 MiB, 48027 MiB free)
Vulkan0: AMD Radeon 890M Graphics (RADV GFX1150)
```

The build enables both HIP and Vulkan. Always pin `-dev ROCm0`, otherwise
llama.cpp may split across backends.

**On hopper there are two GPUs and the backends disagree on ordering.** Verified
output from the `gfx1201` image:

```
ROCm0:   AMD Radeon Graphics, gfx1201 (32624 MiB, 32096 MiB free)   <- R9700, pin this
ROCm1:   AMD Radeon 740M Graphics, gfx1103 (15623 MiB, 18835 MiB free)
Vulkan0: AMD Radeon 740M Graphics (RADV PHOENIX2) (3584 MiB)
Vulkan1: AMD Radeon Graphics (RADV GFX1201) (32624 MiB)
```

Three things to take from this:

1. `-dev ROCm0` is the R9700 — correct, but **`-dev Vulkan0` is the iGPU**. The
   ROCm and Vulkan indices are inverted. Never omit `-dev`.
2. The 740M (`gfx1103`) enumerates as `ROCm1` even though it is not in the
   compiled target set. Do not touch it; production `llama-swap` deliberately
   leaves it free for desktop/VAAPI/Immich (`GGML_VK_VISIBLE_DEVICES=1` there).
3. UMA accounting on this box is nonsense — `ROCm1` reports 18835 MiB free of
   15623 MiB total, and the backend logs
   `available_uma_memory: final available_memory_kb: 19287568` (~18.4 GiB) on a
   host with ~16 GiB actually available. Another reason to keep unified memory
   off and size the model to real VRAM (§6).

Before loading anything, free production VRAM and confirm idle:

```bash
curl -s 127.0.0.1:8090/running          # expect {"running":[]}
curl -s 127.0.0.1:8090/unload           # if not empty
cat /sys/class/drm/card0/device/mem_info_vram_used
```

Verified idle before the enumeration run: `{"running":[]}`, card0 at 0.51 GiB.

## 5. Model download + integrity

```bash
repo=jcbtc/qwopus3.6-27b-v2-chadrock-rocmfp4-mtp
file=Qwopus3.6-27B-v2-MTP-BF16-to-ROCmFP4-STRIX_LEAN.gguf
curl -L "https://huggingface.co/$repo/resolve/main/$file" -o "$file.part"
```

Public repos need no HF token. Verify against the publisher's LFS pointer
(`?path=` on the tree API gives `size` + `oid` sha256) — do not trust size alone.

Verified on Framework 13:

| item | value |
|---|---|
| size | `14817251552` bytes (14.817 GB) |
| sha256 | matches publisher LFS oid |
| path | `/mnt/ai-models/llama/models/Qwopus3.6-27B-v2-ROCmFP4/` |
| download | ~15.6 min @ ~15 MB/s |

Optional `mmproj-F32.mmproj` (0.931 GB) adds image **input**. The model cannot
generate images.

## 6. Run

Publisher's `serve.sh` (from the model card) with two deliberate deviations:

```bash
docker run --rm -d --name qwopus \
  --device=/dev/kfd --device=/dev/dri \
  --group-add video --group-add render \
  --security-opt seccomp=unconfined \
  -v /path/to/models:/models:ro \
  -p 127.0.0.1:18080:18080 \
  local/rocmfpx-qwopus:gfx1150 \
  -m /models/Qwopus3.6-27B-v2-MTP-BF16-to-ROCmFP4-STRIX_LEAN.gguf \
  --alias qwopus --host 0.0.0.0 --port 18080 --jinja \
  -c 8192 -ngl 999 -fa on -dev ROCm0 -b 512 -ub 512 \
  -ctk q4_0 -ctv q4_0 --no-mmap \
  --spec-type draft-mtp --spec-draft-ngl all \
  --spec-draft-type-k q4_0 --spec-draft-type-v q4_0 \
  --spec-draft-n-max 4 --spec-draft-n-min 0 --spec-draft-p-min 0.0 \
  --parallel 1 --metrics
```

Deviations from the model card, and why:

1. **No `HSA_OVERRIDE_GFX_VERSION=11.5.1`.** The card assumes Strix Halo. We
   compile for the native target, so the override is wrong on both boxes. On
   `gfx1201` it would be actively harmful.
2. **No `GGML_HIP_ENABLE_UNIFIED_MEMORY=1` on hopper.** On an APU it is how you
   exceed the VRAM carve-out. On hopper the R9700 has 31.9 GiB of real VRAM and
   the host has 30 GiB total with ~14 GiB already used — unified memory there
   invites the exact GTT-spill OOM documented in
   `selfhost/llm/triage/2026-05-07-oom-after-boot.md`. Keep it off; size the
   model to fit VRAM.

The MTP head is embedded in the GGUF — `--spec-type draft-mtp` needs no separate
draft model, and the server builds the draft context against the target model.

## 7. Measured results — Framework 13 / gfx1150 (dense 27B)

`llama-bench`, no MTP:

| test | t/s |
|---|---|
| pp128 | 78.47 |
| tg32 | **1.97** |

Server, MTP on, thinking **on**, cold first request:

| metric | value |
|---|---|
| prompt | 29 tok @ 1.07 t/s (27.0 s) |
| eval | 128 tok @ 4.21 t/s |
| draft acceptance | 87/151 = 0.576, mean len 3.23, per-pos (0.82, 0.64, 0.46, 0.31) |

Server, MTP on, thinking **off**, warm:

| metric | value |
|---|---|
| prompt | 35 tok @ 13.10 t/s |
| eval | 67 tok @ **5.49 t/s** |
| draft acceptance | 53/60 |
| output | valid, correct `IsPalindrome` |

### Reading these numbers

- MTP is worth **2.8x** on decode (1.97 → 5.49 t/s). It works, and acceptance is
  high (~88% warm).
- 5.49 t/s is still **unusable** for interactive coding. For comparison, Gemma 4
  26B-A4B MXFP4 does 18.6 t/s on the same laptop via plain Vulkan.
- Root cause is not the runtime: a **dense** 27B at ~13.8 GiB reads the whole
  model per token against an 89.6 GB/s bus. The ceiling is arithmetic. This
  model class does not belong on this laptop — see `JCBTC_MOE_CANDIDATES.md`.
- **Measurement trap:** discard the first request. Cold prompt processing was
  1.07 t/s vs 13.10 t/s warm — a 12x artifact.

## 7b. Measured results — Framework 13 / gfx1150 (MoE 35B-A3B)

Model: `Qwen3.6-35B-A3B-NSC-ACE-SABER-MTP-F16-to-ROCmFP4-STRIX_LEAN.gguf`
(19.047 GB, sha256 verified against publisher LFS oid `6a635d1d8ac4af8f…`),
at `/mnt/ai-models/llama/models/Qwen3.6-35B-A3B-ACE-SABER-ROCmFP4/`.
Reported as `qwen35moe 35B.A3B Q4_0_ROCMFP4_STRIX_LEAN`, 17.73 GiB resident.

`llama-bench` (`-p 128 -n 32 -r 2 -mmp 0`), no MTP:

| test | t/s |
|---|---|
| pp128 | 151.56 ± 2.05 |
| tg32 | **9.78 ± 0.35** |

Server, MTP on, thinking **off**, warm (3 measured requests after one discarded
warm-up, same prompt as the dense run):

| run | prompt t/s | eval t/s | draft accepted |
|---|---|---|---|
| 1 | 45.63 | 14.51 | 55/64 |
| 2 | 48.56 | 14.70 | 55/64 |
| 3 | 50.61 | 14.57 | 55/64 |

### Dense vs MoE on this laptop, and vs the existing model set

| model | backend | decode t/s |
|---|---|---|
| Qwopus 27B **dense** ROCmFP4, no MTP | HIP | 1.97 |
| Qwopus 27B **dense** ROCmFP4, MTP | HIP | 5.49 |
| ACE-SABER 35B-A3B **MoE** ROCmFP4, no MTP | HIP | 9.78 |
| ACE-SABER 35B-A3B **MoE** ROCmFP4, MTP | HIP | **14.6** |
| Gemma 4 26B-A4B MXFP4 (existing, `gemma4-26b-qat-mtp`) | Vulkan | 18.6 |
| Qwen3.6-35B-A3B UD-Q4_K_XL (existing, `qwen36-moe`) | Vulkan | ~22 |

Reading it:

- MoE is worth **5x** over dense at the no-MTP floor (1.97 → 9.78) and **2.7x**
  with MTP (5.49 → 14.6). Confirms the bandwidth argument: 3 B active params
  instead of 27 B dense.
- MTP acceptance is *higher* and more stable on the MoE (55/64 = 86%, identical
  across three runs) than on the dense model.
- **It is still slower than what's already installed.** 14.6 t/s vs ~18.6
  (Gemma 4 MXFP4) and ~22 (Qwen3.6-35B-A3B UD-Q4_K_XL) on plain Vulkan. So the
  open question is not speed — it's whether the ROCmFP4 quant buys *quality* at
  a comparable size (19.0 GB vs 21.7 GB for the Unsloth Qwen3.6-35B-A3B).
  That needs exam_v3 + the agentic task, not another bench run.
- These are **not** apples-to-apples on power profile: the existing Vulkan
  numbers in `machines/framework13.md` were taken on power-saver; this run's
  profile was not confirmed. Re-measure both on the same profile before putting
  the comparison in a blog post.

## 8. Known warnings (benign)

- `llama_sampler_backend_support: device 'ROCm0' does not have support for op
  TOP_K` — backend sampling falls back for top-k. Harmless.
- `n_ctx_seq (8192) < n_ctx_train (262144)` — expected, we pinned a small ctx.

## 9. Isolation rules used

- Model bind-mounted read-only from the model filesystem, never copied into
  Docker's overlay (14.8 GB, and the image is already 22 GB).
- Temporary container name + explicit `docker rm`; nothing left running.
- `llama-swap` config untouched on both boxes. On hopper, hermes depends on
  `llama-swap.service` — run experiments as a standalone container on a
  separate port, do not edit `llama-swap.yaml` (`-watch-config` would kill the
  in-flight production model).
- On Framework 13 `llama-fim.service` was stopped for the measurement and
  restarted afterwards.
