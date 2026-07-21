#!/usr/bin/env python3
"""Benchmark staggered arrivals, fair prefills, and threshold boundaries."""

import argparse
import json
import random
import threading
import time
from pathlib import Path
from typing import Any

import requests


WORDS = """algorithm compute digital innovation quantum robotics software
technology virtual network database mountain ocean forest planet climate
wildlife ecosystem atmosphere renewable sustainable biological natural organic
environment community society culture tradition diversity equality justice
democracy freedom humanity civilization education heritage philosophy economy
finance market investment enterprise commerce industry revenue strategy
competition management resource capital prosperity research science discovery
experiment theory hypothesis evidence analysis knowledge wisdom intelligence
learning academic scholarship creative artistic imagination aesthetic expression
inspiration design architecture literature poetry painting sculpture performance
melody emotion passion empathy compassion mindfulness awareness consciousness
perception intuition reflection meditation happiness serenity gratitude moment
eternal temporal spatial dimension horizon infinity universe cosmos reality
existence journey destiny evolution action progress development advancement
achievement success excellence improvement transformation revolution growth
expansion breakthrough pioneer connection relationship interaction collaboration
communication cooperation harmony unity solidarity partnership integration
bond""".split()


def generated_messages(length: int, nonce: str) -> list[dict[str, str]]:
    rng = random.Random(f"{nonce}-{length}")
    words = [f"[scheduler-staggered-{nonce}-{length}]"]
    words.extend(rng.choice(WORDS) for _ in range(max(1, length - 20)))
    return [
        {"role": "system", "content": "You are a helpful assistant."},
        {
            "role": "user",
            "content": " ".join(words)
            + "\nWrite a concise answer about existence and consciousness.",
        },
    ]


def token_count(tokenize_url: str, model: str, messages: list[dict[str, str]]) -> int:
    response = requests.post(
        tokenize_url,
        json={"model": model, "messages": messages},
        timeout=120,
    )
    response.raise_for_status()
    return int(response.json()["count"])


def exact_token_messages(
    tokenize_url: str,
    model: str,
    target_tokens: int,
    nonce: str,
) -> list[dict[str, str]]:
    """Build chat messages with exactly target_tokens after chat templating."""
    prefix = f"[scheduler-boundary-{nonce}]\n"
    filler = max(1, target_tokens - 32)
    for _ in range(8):
        messages = [
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": prefix + (" a" * filler)},
        ]
        actual = token_count(tokenize_url, model, messages)
        if actual == target_tokens:
            return messages
        filler += target_tokens - actual
        if filler < 1:
            break
    raise RuntimeError(
        f"Could not construct exact prompt: target={target_tokens}, last={actual}"
    )


def stream_request(
    url: str,
    model: str,
    messages: list[dict[str, str]],
    output_tokens: int,
    suite_zero: float,
    request_id: str,
    scheduled_offset_s: float,
    ignore_eos: bool,
) -> dict[str, Any]:
    body = {
        "model": model,
        "messages": messages,
        "max_tokens": output_tokens,
        "temperature": 0,
        "ignore_eos": ignore_eos,
        "stream": True,
        "stream_options": {"include_usage": True},
    }
    started = time.perf_counter()
    first = None
    usage: dict[str, Any] = {}
    error = None
    try:
        with requests.post(url, json=body, stream=True, timeout=1200) as response:
            response.raise_for_status()
            for raw in response.iter_lines(chunk_size=1, decode_unicode=True):
                if not raw:
                    continue
                line = raw[6:] if raw.startswith("data: ") else raw
                if line == "[DONE]":
                    continue
                try:
                    event = json.loads(line)
                except json.JSONDecodeError:
                    continue
                usage = event.get("usage") or usage
                choices = event.get("choices") or []
                if choices:
                    delta = choices[0].get("delta") or {}
                    if first is None and (
                        delta.get("content") or delta.get("reasoning_content")
                    ):
                        first = time.perf_counter()
    except Exception as exc:  # Preserve the other concurrent request results.
        error = f"{type(exc).__name__}: {exc}"
    ended = time.perf_counter()
    first = first or ended
    prompt_tokens = int(usage.get("prompt_tokens") or 0)
    generated = int(usage.get("completion_tokens") or 0)
    ttft_s = max(first - started, 0.0)
    decode_s = max(ended - first, 0.001)
    return {
        "request_id": request_id,
        "scheduled_offset_s": round(scheduled_offset_s, 3),
        "actual_start_offset_s": round(started - suite_zero, 3),
        "first_token_offset_s": round(first - suite_zero, 3),
        "finish_offset_s": round(ended - suite_zero, 3),
        "prompt_tokens": prompt_tokens,
        "output_tokens": generated,
        "ttft_s": round(ttft_s, 3),
        "effective_prompt_tps": round(prompt_tokens / max(ttft_s, 0.001), 2),
        "decode_s": round(decode_s, 3),
        "decode_tps": round(generated / decode_s, 2),
        "total_s": round(ended - started, 3),
        "error": error,
    }


def run_one(
    url: str,
    model: str,
    messages: list[dict[str, str]],
    output_tokens: int,
    request_id: str,
    ignore_eos: bool,
) -> dict[str, Any]:
    zero = time.perf_counter()
    return stream_request(
        url,
        model,
        messages,
        output_tokens,
        zero,
        request_id,
        0.0,
        ignore_eos,
    )


def staggered_suite(args: argparse.Namespace, nonce: str) -> dict[str, Any]:
    idle_short = run_one(
        args.url,
        args.model,
        generated_messages(args.short_length, nonce + "-idle-short"),
        args.short_output,
        "idle-short",
        True,
    )
    idle_long = run_one(
        args.url,
        args.model,
        generated_messages(args.long_length, nonce + "-idle-long"),
        1,
        "idle-long",
        False,
    )

    suite_zero = time.perf_counter()
    results: list[dict[str, Any] | None] = [None] * args.request_count
    threads = []

    def worker(index: int) -> None:
        offset = index * args.stagger_delay
        remaining = suite_zero + offset - time.perf_counter()
        if remaining > 0:
            time.sleep(remaining)
        is_long = index == 0
        length = args.long_length if is_long else args.short_length
        output = 1 if is_long else args.short_output
        results[index] = stream_request(
            args.url,
            args.model,
            generated_messages(length, f"{nonce}-stagger-{index + 1}"),
            output,
            suite_zero,
            f"request-{index + 1}-{'long' if is_long else 'short'}",
            offset,
            True if not is_long else False,
        )

    for index in range(args.request_count):
        thread = threading.Thread(target=worker, args=(index,), daemon=False)
        thread.start()
        threads.append(thread)
    for thread in threads:
        thread.join()

    return {
        "mode": "staggered",
        "policy": {
            "request_count": args.request_count,
            "stagger_delay_s": args.stagger_delay,
            "request_1_prompt_words": args.long_length,
            "request_2_to_n_prompt_words": args.short_length,
            "short_output_tokens": args.short_output,
        },
        "idle_short": idle_short,
        "idle_long": idle_long,
        "requests": results,
        "suite_wall_s": round(time.perf_counter() - suite_zero, 3),
    }


def fairness_suite(args: argparse.Namespace, nonce: str) -> dict[str, Any]:
    """Submit equal exact-token prefills at fixed staggered arrival times."""
    tokenize_url = args.url.rsplit("/v1/chat/completions", 1)[0] + "/tokenize"
    messages = [
        exact_token_messages(
            tokenize_url,
            args.model,
            args.fairness_prompt_tokens,
            f"{nonce}-fairness-{index + 1}",
        )
        for index in range(args.request_count)
    ]
    suite_zero = time.perf_counter()
    results: list[dict[str, Any] | None] = [None] * args.request_count
    threads = []

    def worker(index: int) -> None:
        offset = index * args.stagger_delay
        remaining = suite_zero + offset - time.perf_counter()
        if remaining > 0:
            time.sleep(remaining)
        results[index] = stream_request(
            args.url,
            args.model,
            messages[index],
            args.fairness_output,
            suite_zero,
            f"fairness-{args.fairness_prompt_tokens}-{index + 1}",
            offset,
            args.fairness_output > 1,
        )

    for index in range(args.request_count):
        thread = threading.Thread(target=worker, args=(index,), daemon=False)
        thread.start()
        threads.append(thread)
    for thread in threads:
        thread.join()

    return {
        "mode": "fairness",
        "policy": {
            "request_count": args.request_count,
            "stagger_delay_s": args.stagger_delay,
            "exact_prompt_tokens": args.fairness_prompt_tokens,
            "output_tokens": args.fairness_output,
        },
        "requests": results,
        "suite_wall_s": round(time.perf_counter() - suite_zero, 3),
    }


def boundary_suite(args: argparse.Namespace, nonce: str) -> dict[str, Any]:
    tokenize_url = args.url.rsplit("/v1/chat/completions", 1)[0] + "/tokenize"
    targets = [int(value) for value in args.boundary_targets.split(",")]
    cases = []
    for target in targets:
        idle_messages = exact_token_messages(
            tokenize_url, args.model, target, f"{nonce}-idle-{target}"
        )
        injected_messages = exact_token_messages(
            tokenize_url, args.model, target, f"{nonce}-inject-{target}"
        )
        idle = run_one(
            args.url,
            args.model,
            idle_messages,
            args.boundary_output,
            f"boundary-{target}-idle",
            True,
        )

        suite_zero = time.perf_counter()
        holder: dict[str, Any] = {}

        def long_worker() -> None:
            holder["long"] = stream_request(
                args.url,
                args.model,
                generated_messages(
                    args.long_length, f"{nonce}-boundary-long-{target}"
                ),
                1,
                suite_zero,
                f"long-for-boundary-{target}",
                0.0,
                False,
            )

        thread = threading.Thread(target=long_worker, daemon=False)
        thread.start()
        time.sleep(max(0.0, suite_zero + args.boundary_inject_delay - time.perf_counter()))
        injected = stream_request(
            args.url,
            args.model,
            injected_messages,
            args.boundary_output,
            suite_zero,
            f"boundary-{target}-injected",
            args.boundary_inject_delay,
            True,
        )
        thread.join()
        cases.append(
            {
                "target_prompt_tokens": target,
                "scheduler_class": "long" if target > args.long_threshold else "short",
                "idle": idle,
                "injected": injected,
                "competing_long": holder["long"],
                "case_wall_s": round(time.perf_counter() - suite_zero, 3),
            }
        )
    return {
        "mode": "boundary",
        "long_threshold": args.long_threshold,
        "targets": targets,
        "inject_delay_s": args.boundary_inject_delay,
        "boundary_output_tokens": args.boundary_output,
        "cases": cases,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--mode", choices=("staggered", "fairness", "boundary"), required=True
    )
    parser.add_argument(
        "--url", default="http://127.0.0.1:60015/v1/chat/completions"
    )
    parser.add_argument("--model", default="Qwen3.6-27B-AWQ")
    parser.add_argument("--long-length", type=int, default=50000)
    parser.add_argument("--short-length", type=int, default=381)
    parser.add_argument("--short-output", type=int, default=128)
    parser.add_argument("--request-count", type=int, default=8)
    parser.add_argument("--stagger-delay", type=float, default=3.0)
    parser.add_argument("--fairness-prompt-tokens", type=int, default=10000)
    parser.add_argument("--fairness-output", type=int, default=1)
    parser.add_argument("--long-threshold", type=int, default=7840)
    parser.add_argument("--boundary-targets", default="7839,7840,7841")
    parser.add_argument("--boundary-output", type=int, default=32)
    parser.add_argument("--boundary-inject-delay", type=float, default=3.0)
    parser.add_argument("--output-json", required=True)
    args = parser.parse_args()
    nonce = f"{time.time_ns():x}"
    if args.mode == "staggered":
        result = staggered_suite(args, nonce)
    elif args.mode == "fairness":
        result = fairness_suite(args, nonce)
    else:
        result = boundary_suite(args, nonce)
    result["created_at_unix"] = time.time()
    rendered = json.dumps(result, ensure_ascii=False, indent=2)
    Path(args.output_json).write_text(rendered + "\n", encoding="utf-8")
    print(rendered)


if __name__ == "__main__":
    main()
