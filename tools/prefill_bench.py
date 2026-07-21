#!/usr/bin/env python3
"""Isolated OpenAI-compatible prefill/decode benchmark.

This intentionally mirrors the timing semantics of
``本地大模型推理速度测试工具v2.2.html`` instead of the project's load-test
aggregates.  TTFT is measured from request start to the first non-empty
reasoning/content token, and prompt/completion counts come from API usage.
"""

import argparse
import json
import random
import statistics
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

import requests


WORDS = """algorithm artificial automation blockchain compute digital innovation quantum robotics software technology virtual network database mountain ocean forest planet climate wildlife ecosystem atmosphere renewable sustainable biological natural organic environment community society culture tradition diversity equality justice democracy freedom humanity civilization education heritage philosophy economy finance market investment enterprise commerce industry revenue strategy competition management resource capital prosperity research science discovery experiment theory hypothesis evidence analysis knowledge wisdom intelligence learning academic scholarship creative artistic imagination aesthetic expression inspiration design architecture literature poetry painting sculpture performance melody emotion passion empathy compassion mindfulness awareness consciousness perception intuition reflection meditation happiness serenity gratitude moment eternal temporal spatial dimension horizon infinity universe cosmos reality existence journey destiny evolution action progress development advancement achievement success excellence improvement transformation revolution growth expansion breakthrough pioneer connection relationship interaction collaboration communication cooperation harmony unity solidarity partnership network community integration bond""".split()

SUFFIX = (
    "Based on the words above, write a short philosophical essay discussing "
    "the meaning of existence, the nature of consciousness, and humanity's "
    "place in the universe. Use clear, coherent sentences."
)


def normalize_url(url):
    url = url.rstrip("/")
    if url.endswith("/chat/completions"):
        return url
    if url.endswith("/v1"):
        return url + "/chat/completions"
    return url + "/v1/chat/completions"


def parse_lengths(value, fallback):
    if not value:
        return [fallback]
    lengths = [int(item.strip()) for item in value.split(",") if item.strip()]
    if not lengths or any(length <= 20 for length in lengths):
        raise argparse.ArgumentTypeError("lengths must contain integers > 20")
    return lengths


def make_prompt(length, run, seed, suite_nonce):
    rng = random.Random(seed + length * 1009 + run * 9176)
    # Nonce must be the first token.  With prefix caching enabled, changing only
    # a suffix would still reuse almost the entire prompt and inflate Prefill TPS.
    words = [f"[Request-{suite_nonce}-seed{seed}-length{length}-run{run}]"]
    words.extend(rng.choice(WORDS) for _ in range(length - 20))
    return " ".join(words) + "\n" + SUFFIX


def output_token_count(usage):
    completion = int(usage.get("completion_tokens") or 0)
    reasoning = int(usage.get("reasoning_tokens") or 0)
    details = usage.get("completion_tokens_details") or {}
    reasoning = max(reasoning, int(details.get("reasoning_tokens") or 0))
    return completion, reasoning, completion + reasoning


def one_run(session, args, length, run):
    body = {
        "model": args.model,
        "messages": [
            {"role": "system", "content": args.system_message},
            {"role": "user", "content": make_prompt(length, run, args.seed, args.suite_nonce)},
        ],
        "max_tokens": args.output_length,
        "temperature": 0,
        "top_p": 0.1,
        "presence_penalty": 0,
        "frequency_penalty": 0,
        "ignore_eos": args.ignore_eos,
        "stream": True,
        "stream_options": {"include_usage": True},
    }

    start = time.perf_counter()
    first_token = None
    end = None
    usage = {}
    content_seen = False
    with session.post(args.url, json=body, stream=True, timeout=args.timeout) as response:
        response.raise_for_status()
        for raw_line in response.iter_lines(chunk_size=1, decode_unicode=True):
            if not raw_line:
                continue
            line = raw_line.strip()
            if line.startswith("data: "):
                line = line[6:]
            if not line or line == "[DONE]":
                continue
            try:
                data = json.loads(line)
            except json.JSONDecodeError:
                continue
            if data.get("usage"):
                usage = data["usage"]
            choices = data.get("choices") or []
            if not choices:
                continue
            choice = choices[0]
            delta = choice.get("delta") or {}
            token = (
                delta.get("reasoning_content")
                or delta.get("content")
                or (choice.get("message") or {}).get("content")
                or choice.get("text")
            )
            if token:
                content_seen = True
                if first_token is None:
                    first_token = time.perf_counter()
        end = time.perf_counter()

    if first_token is None:
        if not content_seen:
            raise RuntimeError("stream ended without a non-empty token")
        first_token = end

    prompt_tokens = int(usage.get("prompt_tokens") or length)
    completion_tokens, reasoning_tokens, generated_tokens = output_token_count(usage)
    prefill_ms = (first_token - start) * 1000
    decode_ms = max((end - first_token) * 1000, 1.0)
    return {
        "length": length,
        "run": run,
        "prompt_tokens": prompt_tokens,
        "prefill_ms": round(prefill_ms, 2),
        "prefill_tps": round(prompt_tokens / (prefill_ms / 1000), 2),
        "output_tokens": generated_tokens,
        "output_ms": round(decode_ms, 2),
        "output_tps": round(generated_tokens / (decode_ms / 1000), 2),
        "completion_tokens": completion_tokens,
        "reasoning_tokens": reasoning_tokens,
        "total_ms": round((end - start) * 1000, 2),
        "token_source": "API" if usage.get("prompt_tokens") else "fallback",
        "timing_source": "end_to_end_first_nonempty_token",
    }


def summarize(length, results):
    return {
        "length": length,
        "runs": len(results),
        "median_prefill_tps": round(statistics.median(r["prefill_tps"] for r in results), 2),
        "median_prefill_ms": round(statistics.median(r["prefill_ms"] for r in results), 2),
        "median_output_tps": round(statistics.median(r["output_tps"] for r in results), 2),
        "median_output_ms": round(statistics.median(r["output_ms"] for r in results), 2),
        "results": results,
    }


def main():
    parser = argparse.ArgumentParser(
        description="HTML-equivalent isolated prefill/decode benchmark"
    )
    parser.add_argument("--url", required=True, help="API root, /v1, or full chat/completions URL")
    parser.add_argument("--model", default="Qwen3.6-27B-AWQ")
    parser.add_argument("--length", type=int, default=10000, help="single length (compatibility option)")
    parser.add_argument("--lengths", help="comma-separated suite, e.g. 10000,50000,90000,130000")
    parser.add_argument("--output-length", type=int, default=128)
    parser.add_argument(
        "--ignore-eos", action="store_true",
        help="force generation to output-length for comparable decode timing",
    )
    parser.add_argument("--runs", type=int, default=1, help="formal runs per length")
    parser.add_argument(
        "--warmup-runs", type=int, default=1,
        help="warm up once at the first length before the entire suite",
    )
    parser.add_argument("--seed", type=int, default=20260720)
    parser.add_argument(
        "--nonce",
        help="unique prompt-prefix nonce; default is generated per process to defeat prefix cache",
    )
    parser.add_argument("--timeout", type=float, default=1200)
    parser.add_argument("--label", default="")
    parser.add_argument("--system-message", default="You are a helpful assistant.")
    parser.add_argument("--output-json", help="write the complete result document to this path")
    args = parser.parse_args()
    args.url = normalize_url(args.url)
    args.suite_nonce = args.nonce or f"{time.time_ns():x}-{random.SystemRandom().getrandbits(32):08x}"
    lengths = parse_lengths(args.lengths, args.length)
    if args.runs < 1 or args.warmup_runs < 0:
        parser.error("runs must be >= 1 and warmup-runs must be >= 0")

    session = requests.Session()
    warmups = []
    for index in range(args.warmup_runs):
        result = one_run(session, args, lengths[0], -(index + 1))
        warmups.append(result)
        print(json.dumps({"warmup": True, **result}, ensure_ascii=False), flush=True)

    summaries = []
    for length in lengths:
        results = []
        for run in range(1, args.runs + 1):
            result = one_run(session, args, length, run)
            results.append(result)
            print(json.dumps(result, ensure_ascii=False), flush=True)
        summary = summarize(length, results)
        summaries.append(summary)
        print(json.dumps({"summary": summary}, ensure_ascii=False), flush=True)

    document = {
        "schema": "1cat-llmtest.isolated-prefill.v1",
        "created_at": datetime.now(timezone.utc).isoformat(),
        "label": args.label,
        "url": args.url,
        "model": args.model,
        "lengths": lengths,
        "output_length": args.output_length,
        "runs_per_length": args.runs,
        "suite_nonce": args.suite_nonce,
        "warmup_policy": "first_length_once_before_suite",
        "warmups": warmups,
        "summaries": summaries,
    }
    print(json.dumps({"final": document}, ensure_ascii=False), flush=True)
    if args.output_json:
        path = Path(args.output_json)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(document, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        sys.exit(130)
