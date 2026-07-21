#!/usr/bin/env python3
import argparse
import json
import statistics
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed

import requests


def percentile(values, pct):
    if not values:
        return None
    vals = sorted(values)
    if len(vals) == 1:
        return vals[0]
    rank = (len(vals) - 1) * pct / 100.0
    lower = int(rank)
    upper = min(lower + 1, len(vals) - 1)
    weight = rank - lower
    return vals[lower] * (1 - weight) + vals[upper] * weight


class SpecMetrics:
    def __init__(self, drafts=0.0, draft_tokens=0.0, accepted_tokens=0.0):
        self.drafts = drafts
        self.draft_tokens = draft_tokens
        self.accepted_tokens = accepted_tokens

    def to_dict(self):
        return {
            "drafts": self.drafts,
            "draft_tokens": self.draft_tokens,
            "accepted_tokens": self.accepted_tokens,
        }


def fetch_spec_metrics(base_url, model_name):
    metrics_url = base_url.replace("/v1", "") + "/metrics"
    resp = requests.get(metrics_url, timeout=30)
    resp.raise_for_status()
    data = {"drafts": 0.0, "draft_tokens": 0.0, "accepted_tokens": 0.0}
    targets = {
        "vllm:spec_decode_num_drafts_total": "drafts",
        "vllm:spec_decode_num_draft_tokens_total": "draft_tokens",
        "vllm:spec_decode_num_accepted_tokens_total": "accepted_tokens",
    }
    for line in resp.text.splitlines():
        if not line or line.startswith("#"):
            continue
        metric = line.split("{", 1)[0]
        if metric not in targets:
            continue
        if f'model_name="{model_name}"' not in line:
            continue
        value = float(line.rsplit(" ", 1)[-1])
        data[targets[metric]] = value
    return SpecMetrics(**data)


def run_one(session, url, headers, payload, timeout):
    start = time.perf_counter()
    first_token_at = None
    usage = None
    chunks = 0

    with session.post(url, headers=headers, json=payload, stream=True, timeout=timeout) as resp:
        resp.raise_for_status()
        for raw_line in resp.iter_lines(decode_unicode=True):
            if not raw_line:
                continue
            line = raw_line.strip()
            if not line.startswith("data: "):
                continue
            data = line[6:]
            if data == "[DONE]":
                break
            obj = json.loads(data)
            choices = obj.get("choices") or []
            if choices:
                delta = choices[0].get("delta") or {}
                content = delta.get("content")
                if content and first_token_at is None:
                    first_token_at = time.perf_counter()
                if content:
                    chunks += 1
            if obj.get("usage"):
                usage = obj["usage"]

    end = time.perf_counter()
    ttft = None if first_token_at is None else first_token_at - start
    latency = end - start
    output_tokens = (usage or {}).get("completion_tokens", 0)
    input_tokens = (usage or {}).get("prompt_tokens", 0)
    decode_time = None
    decode_tps = None
    if ttft is not None:
        decode_time = max(latency - ttft, 1e-9)
        decode_tps = output_tokens / decode_time if output_tokens > 0 else 0.0

    return {
        "latency_s": latency,
        "ttft_s": ttft,
        "decode_time_s": decode_time,
        "decode_tps": decode_tps,
        "output_tokens": output_tokens,
        "input_tokens": input_tokens,
        "chunks": chunks,
    }


def bench_concurrency(args, concurrency):
    url = args.base_url.rstrip("/") + "/chat/completions"
    headers = {
        "Authorization": f"Bearer {args.api_key}",
        "Content-Type": "application/json",
    }
    payload = {
        "model": args.model,
        "messages": [
            {"role": "system", "content": args.system_prompt},
            {"role": "user", "content": args.user_prompt},
        ],
        "temperature": args.temperature,
        "top_p": args.top_p,
        "max_tokens": args.max_tokens,
        "stream": True,
        "stream_options": {"include_usage": True},
    }
    if args.min_tokens > 0:
        payload["min_tokens"] = args.min_tokens

    spec_before = fetch_spec_metrics(args.base_url, args.model)
    session = requests.Session()
    wall_start = time.perf_counter()
    results = []
    lock = threading.Lock()

    def worker():
        result = run_one(session, url, headers, payload, args.timeout)
        with lock:
            results.append(result)
        return result

    with ThreadPoolExecutor(max_workers=concurrency) as pool:
        futures = [pool.submit(worker) for _ in range(args.requests_per_level)]
        for future in as_completed(futures):
            future.result()

    wall_end = time.perf_counter()
    session.close()
    spec_after = fetch_spec_metrics(args.base_url, args.model)

    latencies = [r["latency_s"] for r in results]
    ttfts = [r["ttft_s"] for r in results if r["ttft_s"] is not None]
    decode_tps_values = [r["decode_tps"] for r in results if r["decode_tps"] is not None]
    total_output_tokens = sum(r["output_tokens"] for r in results)
    total_input_tokens = sum(r["input_tokens"] for r in results)
    wall_time = wall_end - wall_start
    spec_delta = SpecMetrics(
        drafts=spec_after.drafts - spec_before.drafts,
        draft_tokens=spec_after.draft_tokens - spec_before.draft_tokens,
        accepted_tokens=spec_after.accepted_tokens - spec_before.accepted_tokens,
    )

    return {
        "concurrency": concurrency,
        "requests": len(results),
        "wall_time_s": wall_time,
        "avg_latency_s": statistics.mean(latencies),
        "p50_latency_s": percentile(latencies, 50),
        "p90_latency_s": percentile(latencies, 90),
        "p99_latency_s": percentile(latencies, 99),
        "avg_ttft_s": statistics.mean(ttfts) if ttfts else None,
        "p50_ttft_s": percentile(ttfts, 50) if ttfts else None,
        "avg_decode_tps_per_request": statistics.mean(decode_tps_values) if decode_tps_values else None,
        "p50_decode_tps_per_request": percentile(decode_tps_values, 50) if decode_tps_values else None,
        "aggregate_decode_tps": total_output_tokens / wall_time if wall_time > 0 else None,
        "aggregate_total_tps": (total_output_tokens + total_input_tokens) / wall_time if wall_time > 0 else None,
        "avg_output_tokens": total_output_tokens / len(results) if results else 0,
        "avg_input_tokens": total_input_tokens / len(results) if results else 0,
        "spec_delta": spec_delta.to_dict(),
        "spec_accept_rate": (
            spec_delta.accepted_tokens / spec_delta.draft_tokens
            if spec_delta.draft_tokens > 0
            else None
        ),
    }


def main():
    parser = argparse.ArgumentParser(description="Manual long-decode benchmark for vLLM.")
    parser.add_argument("--base-url", default="http://127.0.0.1:60015/v1")
    parser.add_argument("--api-key", default="test")
    parser.add_argument("--model", required=True)
    parser.add_argument("--concurrency-levels", default="1,2,4,8")
    parser.add_argument("--requests-per-level", type=int, default=8)
    parser.add_argument("--max-tokens", type=int, default=1000)
    parser.add_argument("--min-tokens", type=int, default=0)
    parser.add_argument("--temperature", type=float, default=0.0)
    parser.add_argument("--top-p", type=float, default=1.0)
    parser.add_argument("--timeout", type=int, default=900)
    parser.add_argument(
        "--system-prompt",
        default="You are a coding assistant. Output code only.",
    )
    parser.add_argument(
        "--user-prompt",
        default=(
            "Write a complete Python module that implements a tiny in-memory key value store "
            "with TTL, background cleanup thread, simple CLI demo, unit tests, and detailed "
            "docstrings. Output code only."
        ),
    )
    parser.add_argument("--output", default="")
    args = parser.parse_args()

    results = []
    for item in args.concurrency_levels.split(","):
        concurrency = int(item.strip())
        if args.requests_per_level < concurrency:
            raise SystemExit(
                f"requests-per-level ({args.requests_per_level}) must be >= concurrency ({concurrency})"
            )
        print(f"Running concurrency={concurrency} ...", flush=True)
        results.append(bench_concurrency(args, concurrency))

    output = {
        "model": args.model,
        "base_url": args.base_url,
        "max_tokens": args.max_tokens,
        "min_tokens": args.min_tokens,
        "requests_per_level": args.requests_per_level,
        "results": results,
    }

    text = json.dumps(output, ensure_ascii=False, indent=2)
    print(text)
    if args.output:
        with open(args.output, "w", encoding="utf-8") as f:
            f.write(text)


if __name__ == "__main__":
    main()
