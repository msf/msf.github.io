# This is how SLOW Local LLMs Are On My Framework 13 AMD Strix Point

*February 2026 -- co-authored with Claude Opus 4.6.

*Previous: [I benchmarked 8 local LLMs writing Go on my Framework 13 AMD Strix Point](benchmarking-local-llms-go-coding.md)*

Yes, the title is clickbaity :>. Veritasium has [a great video](https://www.youtube.com/watch?v=S2xHZPH5Sng) about why clickbait is unreasonably effective and I've been dying to try it on a technical post. The irony is that the actual content is the opposite of clickbait -- every claim backed by a shell command, every number derived from first principles. If the title got you here, the data should keep you.

This post was co-authored with Claude, unapologetically so. Not "AI-assisted" in the sense of "I asked it to polish my draft" -- I mean Claude ran the benchmarks, dug through sysfs for hardware evidence, made claims I challenged, got corrected when the numbers didn't add up, and wrote sections that I then rewrote or pushed back on. The back-and-forth is the point.  These agents are extremely powerful and productive! When Claude suggested I should always benchmark on performance mode, I called that out and the data proved me right (tighter variance on power-saver). When I asked about memory bandwidth, Claude initially guessed wrong about my RAM type (assumed soldered LPDDR5x, it's SO-DIMM DDR5) and I made them go find the actual kernel evidence. The result is better than either of us would produce alone. It is extremely notable the improvements and how little they (the top models, Opus-4.6 gpt-5.3) hallucinate today.

This is Part 2 of my local LLM benchmarking. [Part 1](benchmarking-local-llms-go-coding.md) was about whether local models can write Go. This one is about *why* my Framework 13 gets the numbers it gets, what the hard ceiling is, and how to verify all of this yourself.

I went down this rabbit hole because I wanted to try ROCm instead of Vulkan, hoping to make things faster. Spoiler: it didn't for the workload that matters. But I learned a lot about what actually determines inference speed on an AMD APU.

## The hardware

Framework Laptop 13, Ryzen AI 9 HX 370, Radeon 890M (iGPU), 64GB DDR5 (2x32GB SO-DIMMs, upgradeable to 96GB). Ubuntu 24.04, kernel 6.17.

Everything below is backed by commands you can run yourself. Claude originally wrote "I got burned making claims I couldn't back up" here, which -- fair, but I kept saying "bullshit, show me evidence! prove it or find reliable references" until the numbers actually checked out. That's how this whole post works: Claude did the dirty work, I poked holes, challenged, we fix it together.

### CPU: 12 cores, 24 threads

```
$ cat /proc/cpuinfo | grep "model name" | head -1
AMD Ryzen AI 9 HX 370 w/ Radeon 890M

$ nproc
24
```

### GPU: 16 Compute Units, RDNA 3.5

```
$ cat /sys/class/kfd/kfd/topology/nodes/1/properties | grep -E 'gfx_target|simd'
simd_count 32
simd_per_cu 2              # 32 SIMDs / 2 per CU = 16 CUs
gfx_target_version 110500  # = gfx1150
```

### Memory: the only number that matters

For text generation (the thing you stare at while the model types), performance is almost entirely determined by **memory bandwidth**. Each token requires reading most of the model's weights from memory. The GPU isn't compute-bound -- it's waiting for data.

```
$ dmesg | grep "RAM width"
[drm] RAM width 128bits DDR5

$ cat /sys/class/drm/card1/device/pp_dpm_mclk
0: 1000Mhz
1: 2400Mhz
2: 2800Mhz
```

128-bit bus. Max memory clock 2800 MHz. DDR5 is double data rate, so 2800 MHz = 5600 MT/s = DDR5-5600.

**Theoretical max bandwidth: 5600 × 128 / 8 = 89.6 GB/s.** That's the wall. No software can exceed it.

## Vulkan vs ROCm vs CPU

This laptop has one GPU, one pool of RAM, and three completely different software paths to use them. I tested all three.

Quick primer on the software stack (skip if you don't care):

- **Vulkan** (RADV driver): Graphics API repurposed for compute. Talks to `/dev/dri/*`. What my desktop compositor also uses. Comes with Mesa, no setup needed.
- **ROCm/HIP**: AMD's answer to CUDA. Pure compute, talks to `/dev/kfd`. Separate from Vulkan entirely. Requires either system packages or pre-built binaries from [lemonade-sdk](https://github.com/lemonade-sdk/llamacpp-rocm/releases) (which bundle all the ROCm libs, no install needed).
- **CPU**: AVX-512 on Zen5. Accesses memory through the Infinity Fabric at roughly half the GPU's bandwidth. Not competitive.

### Benchmark results

llama-bench, Vulkan RADV, Qwen3-8B Q4_K_M (4.68 GiB, dense):

| Profile | pp512 (t/s) | tg128 (t/s) |
|---------|------------|------------|
| power-saver (battery) | 146 ± 21 | 9.87 ± 0.03 |
| **performance (AC)** | **322 ± 9** | **13.41 ± 0.21** |

gpt-oss-20B MXFP4 MoE (11.27 GiB):

| Profile | pp512 (t/s) | tg128 (t/s) |
|---------|------------|------------|
| power-saver (battery) | 234 ± 3 | 17.44 ± 0.14 |
| **performance (AC)** | **390 ± 5** | **23.43 ± 0.11** |

Power profile matters enormously: **pp more than doubles, tg gains 34-36%.** If you're benchmarking on battery, you're measuring your power governor, not your hardware.

The MoE model (gpt-oss-20B) achieves higher tg despite being 2.4x larger because MoE only reads active expert weights per token, not the full 11.27 GiB.

### I also tested ROCm and CPU

On power-saver, Qwen3-8B:

| Backend | pp512 (t/s) | tg128 (t/s) |
|---------|------------|------------|
| Vulkan (RADV) | 146 | **9.87** |
| ROCm (HIP) | **207** | 4.76 |
| CPU (24 threads) | 132 | 2.59 |

**ROCm is 41% faster at prompt processing** but **Vulkan is 2x faster at text generation**. Since tg is the user-facing latency, **Vulkan wins for interactive use**. ROCm is worth it for RAG/long-context workloads where prompt processing dominates.

CPU is 4x slower than Vulkan on tg. Why? The GPU and CPU share the same physical DDR5, but:

```
 GPU ──(direct wide path)──→ Memory Controller ──→ DDR5
 CPU ──(Infinity Fabric)────→ Memory Controller ──→ DDR5
        ↑ ~half bandwidth
```

The Infinity Fabric is a packet-based on-die interconnect designed for AMD's multi-chiplet server CPUs. Great for Threadripper. Bottleneck on a monolithic APU. The GPU gets a fatter pipe to the memory controller because GPU workloads are bandwidth-hungry by design. This is why `ngl 99` matters (offload all layers to GPU memory) even though it's the same physical RAM -- you're choosing which internal bus reads the data.

### About `--no-mmap`

The [Strix Halo wiki](https://strixhalo.wiki/AI/llamacpp-performance) recommends always disabling mmap. I tested it:

| mmap | tg128 (t/s) |
|------|-------------|
| enabled (default) | 11.44 ± 0.47 |
| disabled | 11.26 ± 0.22 |

Within noise. For Vulkan, mmap doesn't affect inference speed. The warning is mainly about ROCm, where mmap causes model loading (not inference) to be catastrophically slow on large models. For Vulkan, don't bother.

## Where these numbers come from

For a dense model, each generated token reads approximately the full model weights from memory. So we can derive real-world bandwidth from the benchmark:

```
                          power-saver         performance (AC)
Qwen3-8B Q4_K_M:
  4.68 GiB × 1.074 ×      9.87 t/s            13.41 t/s
  =                       49.6 GB/s            67.4 GB/s
  Utilization:            49.6/89.6 = 55%      67.4/89.6 = 75%
```

55% on battery, 75% plugged in. The difference isn't software -- it's the memory controller and GPU clocking higher on the performance profile.

This also explains why gpt-oss-20b (a MoE model at 11.27 GiB total) gets *faster* tg (23.4 t/s on AC) than Qwen3-8B despite being larger: MoE only reads the active expert weights per token, not the full model. The bytes-per-token is much lower than the file size suggests.

## What's the ceiling

If someone wrote perfect Vulkan shaders with 100% bandwidth utilization (physically impossible):

    89.6 / (4.68 × 1.074) = 17.8 t/s

At a realistic ~85% (excellent, pushing the limits):

    89.6 × 0.85 / (4.68 × 1.074) = 15.2 t/s

**Current on performance profile: 13.4 t/s (75%). Realistic ceiling: ~15 t/s (85%). Absolute ceiling: ~18 t/s (100%).**

We're already at 75% utilization plugged in. The remaining software headroom is ~13-15% -- maybe 2 more tokens per second from future driver/shader improvements. The 128-bit DDR5-5600 bus at 89.6 GB/s is the hard wall.

Or so I thought.

## Speculative decoding: the cheat code

I was digging around the docs directory of llama.cpp and found [speculative decoding](https://github.com/ggml-org/llama.cpp/blob/master/docs/speculative.md) and it completely changed the picture. The idea: a tiny "draft" model generates candidate tokens quickly, then the big model verifies them all in one batch. Verification is a prompt-processing operation (pp), not token generation (tg). On this hardware, pp is 24x faster than tg. That asymmetry is exactly what speculative decoding exploits.

**Qwen3-0.6B** (610 MB) drafting for **Qwen3-8B** (4.68 GiB). Both fit in memory trivially at 5.3 GiB total. All measurements on performance profile, using the factorial prompt:

| Config | tg (t/s) | vs baseline |
|--------|----------|-------------|
| Qwen3-8B alone (baseline) | 12.9 | -- |
| + Qwen3-0.6B draft, draft-max=4 | 21.2 | **+64%** |
| + Qwen3-0.6B draft, draft-max=8 | 22.0 | **+71%** |
| + Qwen3-0.6B draft, draft-max=16 | 22.9 | **+78%** |
| + Qwen3-0.6B draft, draft-max=32 | 23.5 | **+82%** |

On a harder coding prompt (wordfreq, longer output), the gain drops to +40% (12.6 → 17.6 t/s) because the tiny draft model predicts complex code less accurately. Still massive.

I also tested Qwen2.5-Coder-7B with Qwen2.5-Coder-3B as draft: only +36% (8.6 → 11.7 t/s). The draft model needs to be much smaller than the target -- 3B drafting for 7B isn't enough asymmetry. The 0.6B drafting for 8B (13x size ratio) is the sweet spot.


The command is dead simple:

```bash
llama-cli -hf Qwen/Qwen3-8B-GGUF:Q4_K_M \
          -hfrd Qwen/Qwen3-0.6B-GGUF:Q8_0 \
          --draft-max 16 -ngl 99 -ngld 99 -fa 1
```

This matters more than anything in the "what would change the game" table. No hardware changes, no driver hacking, 610 MB of extra memory. The bigger the target model, the more it helps -- a Qwen3-14B with 0.6B draft could be the practical sweet spot for this laptop.

### What would actually change the game (revised)

| Change | Impact on tg |
|--------|-------------|
| **Speculative decoding (0.6B draft)** | **+40-82%** (measured, depends on task) |
| Plugged in + performance profile | **+36% tg, +120% pp** (measured) |
| Better drivers/shaders (software) | +13-15% theoretical max remaining |
| DDR5-6400 SO-DIMMs (if supported) | +14% |
| 256-bit memory bus (different chip) | +100% |
| `amd_iommu=off` kernel param | +1-2% |
| `tuned accelerator-performance` | +3-5% on pp, marginal on tg |

Speculative decoding doesn't break the memory bandwidth wall -- it works *around* it by amortizing the cost. Instead of reading 4.68 GiB per token, you read 4.68 GiB once and verify multiple tokens in batch. The wall is still 89.6 GB/s, but you're getting more tokens per trip to memory.

## Power profile: it matters more than you think

The initial benchmarks were run on battery in power-saver mode. When I plugged in and switched to performance:

```
$ powerprofilesctl get
performance

$ cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor
performance
```

tg jumped from 9.87 to 13.41 t/s (+36%) and pp from 146 to 322 t/s (+120%). The memory controller clocks up on performance profile -- this isn't just about CPU/GPU frequency.

Claude was suggesting I should always benchmark on performance profile. But in reality, power-saver gave tighter measurements: tg variance was ±0.03 (0.3%) on power-saver vs ±0.21 (1.6%) on performance. When clocks are clamped low, they're stable. On performance mode, thermal throttling introduces jitter -- on a laptop, you're one sustained benchmark away from hitting thermal limits and seeing clocks drop mid-run.

What matters is that you **benchmark in a consistent, documented environment**. Both profiles tell you something real: power-saver gives you the stable floor with tight error bars, performance gives you the practical peak on AC. I'd rather have a mode with no frequency scaling at all, but even on servers that's increasingly hard to come by.

Part 1's coding benchmarks were all on power-saver. The scores (pass/fail) wouldn't change, but the wall times would be faster plugged in.

## Comparison to Apple Silicon

Not the same price bracket, not the same product category, but useful for understanding why memory architecture matters:

| Platform | Bus | Mem BW | Est. tg 8B Q4 | Max RAM |
|----------|-----|--------|---------------|---------|
| Framework 13 (DDR5-5600) | 128-bit | 89.6 GB/s | ~13 t/s | 96 GB |
| MacBook Pro 16" M1 Pro | 256-bit | 200 GB/s | ~22 t/s | 32 GB |
| MacBook Pro 16" M3 Pro | 192-bit | 150 GB/s | ~16 t/s | 36 GB |
| MacBook Pro 16" M3 Max | 512-bit | 400 GB/s | ~45 t/s | 64 GB |

Apple wins on bandwidth because they solder wider LPDDR5 buses and their unified memory architecture gives both CPU and GPU full bandwidth (no Infinity Fabric bottleneck). The Framework's advantage is upgradeable SO-DIMMs: 96GB lets you run models that don't fit on a 36GB Mac.

The M3 Pro is interesting: Apple *narrowed* the bus from 256-bit (M1/M2 Pro) to 192-bit, so the 2023 M3 Pro is actually *slower* at tg than the 2021 M1 Pro. Bandwidth regression in a newer product.

## How to reproduce everything

```bash
# Check your hardware
dmesg | grep "RAM width"
cat /sys/class/drm/card1/device/pp_dpm_mclk
cat /sys/class/kfd/kfd/topology/nodes/1/properties | grep -E 'gfx_target|simd'
powerprofilesctl get

# Vulkan benchmark (comes with llama.cpp release)
llama-bench -m model.gguf -ngl 99 -fa 1 -p 512 -n 128

# tg-only (skip prompt processing)
llama-bench -m model.gguf -ngl 99 -fa 1 -p 0 -n 256

# Speculative decoding (the big win)
llama-cli -hf Qwen/Qwen3-8B-GGUF:Q4_K_M \
          -hfrd Qwen/Qwen3-0.6B-GGUF:Q8_0 \
          --draft-max 16 -ngl 99 -ngld 99 -fa 1

# CPU-only baseline
llama-bench -m model.gguf -ngl 0 -t $(nproc) -fa 1 -p 512 -n 128

# ROCm (with lemonade pre-built binaries, no install needed)
LD_LIBRARY_PATH=./llama-rocm:./llama-rocm/hipblaslt \
ROCBLAS_TENSILE_LIBPATH=./llama-rocm/rocblas/library \
  ./llama-rocm/llama-bench -m model.gguf -ngl 99 -mmp 0 -fa 1 -p 512 -n 128

# Derive your bandwidth utilization
# model_size_GB × tg_tokens_per_sec = effective_bandwidth
# Compare to: memory_MT_per_sec × bus_bits / 8 = theoretical_max
```

## What's next

- Test Qwen3-14B + Qwen3-0.6B draft. The bigger the target, the more speculative decoding helps. This could be the practical sweet spot for this laptop.
- Benchmark the new models: Qwen3-Coder-30B-A3B, Qwen2.5-Coder-14B. See [Part 1](benchmarking-local-llms-go-coding.md) for the coding benchmark these will run against.
- Re-run the Part 1 coding exam with speculative decoding enabled -- wall times should drop significantly.

---

*Built with llama.cpp (commit 612db61 / 2026-02-10 for Vulkan, commit a0c91e8 / 2026-02-21 for ROCm via lemonade-sdk). Framework 13, Ubuntu 24.04, kernel 6.17.0.*

*[Part 1: I benchmarked 8 local LLMs writing Go on my Framework 13 AMD Strix Point](benchmarking-local-llms-go-coding.md)*
