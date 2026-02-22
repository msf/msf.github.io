# EVM Foundations (Days 1–7)

> Goal: build solid mental models of EVM execution, storage, state, and gas—validated by small, real traces on your node.

---

## Prereqs (install any you’re missing)

* **Geth** (or your node with debug/trace APIs): [https://geth.ethereum.org/](https://geth.ethereum.org/)
* **Foundry (cast/forge)**: [https://book.getfoundry.sh/](https://book.getfoundry.sh/)
* **Solidity compiler (solc)**: [https://docs.soliditylang.org/en/latest/installing-solidity.html](https://docs.soliditylang.org/en/latest/installing-solidity.html)

---

## Day 1 — Big-picture EVM & “living spec”

**Read**

* EVM overview: [https://ethereum.org/en/developers/docs/evm/](https://ethereum.org/en/developers/docs/evm/)
* Execution-specs (EELS) README: [https://github.com/ethereum/execution-specs](https://github.com/ethereum/execution-specs)
* Keep these open while learning:

  * Opcodes (reference): [https://ethereum.org/en/developers/docs/evm/opcodes/](https://ethereum.org/en/developers/docs/evm/opcodes/)
  * Opcodes (interactive, gas, traces): [https://www.evm.codes/](https://www.evm.codes/)

**Output**

* 1-page notes: account model, call stack vs. memory vs. storage, gas lifecycle, where env/state come from.

---

## Day 2 — Yellow Paper (surgical read)

**Read (targeted)**

* Yellow Paper PDF: [https://ethereum.github.io/yellowpaper/paper.pdf](https://ethereum.github.io/yellowpaper/paper.pdf)
  Focus: State transition (transactions→blocks), Execution model, Gas, Memory, and the Opcode appendix.

**Output**

* Bullet list of invariants (e.g., call/revert behavior, exceptional halts, memory expansion rules).

---

## Day 3 — Opcodes by category + minimal bytecode

**Read**

* Opcode groups (stack/mem/storage/call/flow/log/precompiles) via: [https://www.evm.codes/](https://www.evm.codes/)

**Hands-on**

```bash
# Tiny contract → bytecode & opcodes
printf 'pragma solidity ^0.8.20; contract T { function f(uint x) public pure returns(uint){ return x+1; } }' > T.sol
solc --bin --opcodes T.sol
```

**Output**

* 10–15 opcode “flashcards” with purpose + gotcha (e.g., `CALL` gas stipend, `JUMPDEST`, `PUSH0`).

---

## Day 4 — Trace one real mainnet tx end-to-end

**Read**

* JSON-RPC basics: [https://ethereum.org/en/developers/docs/apis/json-rpc/](https://ethereum.org/en/developers/docs/apis/json-rpc/)
* Geth tracing overview: [https://geth.ethereum.org/docs/developers/evm-tracing](https://geth.ethereum.org/docs/developers/evm-tracing)

**Hands-on (pick a tx hash you care about)**

```bash
# Inspect tx quickly
cast tx 0xYOUR_TX_HASH --json | jq '.'

# Geth-style deep trace (call graph)
curl -s -X POST http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  --data '{"jsonrpc":"2.0","id":1,"method":"debug_traceTransaction",
           "params":["0xYOUR_TX_HASH", {"tracer":"callTracer","timeout":"30s"}]}' | jq '.'

# Optional: step-by-step VM trace
curl -s -X POST http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  --data '{"jsonrpc":"2.0","id":1,"method":"debug_traceTransaction",
           "params":["0xYOUR_TX_HASH", {"tracer":"vmTrace","timeout":"30s"}]}' | jq '.'
```

**Output**

* Short write-up mapping call tree ↔ opcodes/memory/storage deltas for one internal call.

---

## Day 5 — Storage layout, mappings, and proofs

**Read**

* Solidity storage layout: [https://docs.soliditylang.org/en/latest/internals/layout_in_storage.html](https://docs.soliditylang.org/en/latest/internals/layout_in_storage.html)
  (mappings/dynamic arrays hashing formula)
* `eth_getProof`: [https://ethereum.org/en/developers/docs/apis/json-rpc/#eth_getproof](https://ethereum.org/en/developers/docs/apis/json-rpc/#eth_getproof)

**Hands-on (ERC-20 balance slot example)**

* For `mapping(address⇒uint)` at slot `p` (often `0`): storage slot = `keccak256(pad32(addr) ++ pad32(p))`

```bash
# Read a raw storage slot once you've computed it
cast storage 0xTOKEN_ADDRESS 0xCOMPUTED_SLOT
# Or raw JSON-RPC
cast rpc eth_getStorageAt 0xTOKEN_ADDRESS 0xCOMPUTED_SLOT latest

# Get Merkle proof for account + specific storage keys
cast rpc eth_getProof 0xTOKEN_ADDRESS '["0xCOMPUTED_SLOT"]' latest | jq '.'
```

**Output**

* One example showing: computed slot → raw 32-byte value → decoded balance (uint256).

---

## Day 6 — Tries & encoding (just enough to reason about state)

**Read**

* Merkle–Patricia Trie: [https://ethereum.org/en/developers/docs/data-structures-and-encoding/patricia-merkle-trie/](https://ethereum.org/en/developers/docs/data-structures-and-encoding/patricia-merkle-trie/)
* RLP: [https://ethereum.org/en/developers/docs/data-structures-and-encoding/rlp/](https://ethereum.org/en/developers/docs/data-structures-and-encoding/rlp/)

**Hands-on (light)**

```bash
# Fetch a block header and tx for context
cast block 17000000 --json | jq '.'
cast tx 0xYOUR_TX_HASH --json | jq '.'
```

**Output**

* Diagram that names the tries (state / tx / receipt), and where your Day-5 storage proof sits in the state trie.

---

## Day 7 — Gas you must actually know (post-fork reality)

**Read (EIPs)**

* SSTORE pricing: [https://eips.ethereum.org/EIPS/eip-2200](https://eips.ethereum.org/EIPS/eip-2200)
* Warm/cold access: [https://eips.ethereum.org/EIPS/eip-2929](https://eips.ethereum.org/EIPS/eip-2929)
* Refund reductions: [https://eips.ethereum.org/EIPS/eip-3529](https://eips.ethereum.org/EIPS/eip-3529)
* Access lists: [https://eips.ethereum.org/EIPS/eip-2930](https://eips.ethereum.org/EIPS/eip-2930)
* Newer opcodes to recognize:
  `PUSH0` (Shanghai) [https://eips.ethereum.org/EIPS/eip-3855](https://eips.ethereum.org/EIPS/eip-3855)
  `MCOPY` (Shanghai) [https://eips.ethereum.org/EIPS/eip-5656](https://eips.ethereum.org/EIPS/eip-5656)
  `TLOAD`/`TSTORE` (Cancun) [https://eips.ethereum.org/EIPS/eip-1153](https://eips.ethereum.org/EIPS/eip-1153)

**Hands-on**

```bash
# Compare gas behavior via traces for two txs:
# (1) cold SLOAD/SSTORE vs (2) warmed access or no-op SSTORE
# Use callTracer/vmTrace from Day 4 and note gas deltas at the op level.
```

**Output**

* Table with 3 rows: memory expansion cost example, SSTORE (0→non-0 vs non-0→non-0), cold→warm access delta.

---

## Optional quick references

* EVM From Scratch (stepwise, multi-lang): [https://www.evm-from-scratch.xyz/](https://www.evm-from-scratch.xyz/)
* EIP index (when you hear an opcode/fork name): [https://eips.ethereum.org/](https://eips.ethereum.org/)
* evm.codes playground (paste bytecode, step it): [https://www.evm.codes/playground](https://www.evm.codes/playground)

---

## What’s next (use your codebase/tests)

* Map your C++ EVMC/evmONE boundaries, then replay the Day-4 tx with your internal parallel re-execution + tracers.
* Cross-check side effects and gas accounting against geth’s traces.

