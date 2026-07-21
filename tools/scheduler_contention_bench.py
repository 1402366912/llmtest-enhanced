#!/usr/bin/env python3
"""Measure short-request TTFT while a long prefill is running."""

import argparse
import json
import random
import threading
import time
from pathlib import Path

import requests


WORDS = "algorithm compute digital innovation quantum robotics software technology virtual network database mountain ocean forest planet climate wildlife ecosystem atmosphere renewable sustainable biological natural organic environment community society culture tradition diversity equality justice democracy freedom humanity civilization education heritage philosophy economy finance market investment enterprise commerce industry revenue strategy competition management resource capital prosperity research science discovery experiment theory hypothesis evidence analysis knowledge wisdom intelligence learning academic scholarship creative artistic imagination aesthetic expression inspiration design architecture literature poetry painting sculpture performance melody emotion passion empathy compassion mindfulness awareness consciousness perception intuition reflection meditation happiness serenity gratitude moment eternal temporal spatial dimension horizon infinity universe cosmos reality existence journey destiny evolution action progress development advancement achievement success excellence improvement transformation revolution growth expansion breakthrough pioneer connection relationship interaction collaboration communication cooperation harmony unity solidarity partnership network community integration bond".split()


def make_prompt(length: int, nonce: str) -> str:
    rng = random.Random(f"{nonce}-{length}")
    words = [f"[scheduler-bench-{nonce}-{length}]"]
    words.extend(rng.choice(WORDS) for _ in range(max(1, length - 20)))
    return " ".join(words) + "\nWrite a concise answer about existence and consciousness."


def request(
    url: str,
    model: str,
    length: int,
    output_tokens: int,
    nonce: str,
    ignore_eos: bool = False,
):
    body = {
        "model": model,
        "messages": [
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": make_prompt(length, nonce)},
        ],
        "max_tokens": output_tokens,
        "temperature": 0,
        "ignore_eos": ignore_eos,
        "stream": True,
        "stream_options": {"include_usage": True},
    }
    started = time.perf_counter()
    first = None
    usage = {}
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
    ended = time.perf_counter()
    first = first or ended
    generated = int(usage.get("completion_tokens") or output_tokens)
    decode_s = max(ended - first, 0.001)
    return {
        "requested_prompt_words": length,
        "prompt_tokens": int(usage.get("prompt_tokens") or 0),
        "output_tokens": generated,
        "ttft_s": round(first - started, 3),
        "total_s": round(ended - started, 3),
        "decode_tps": round(generated / decode_s, 2),
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", default="http://127.0.0.1:60015/v1/chat/completions")
    parser.add_argument("--model", default="Qwen3.6-27B-AWQ")
    parser.add_argument("--long-length", type=int, default=50000)
    parser.add_argument("--short-length", type=int, default=381)
    parser.add_argument("--inject-delay", type=float, default=2.0)
    parser.add_argument("--short-output", type=int, default=16)
    parser.add_argument("--ignore-eos", action="store_true")
    parser.add_argument("--output-json")
    args = parser.parse_args()
    nonce = f"{time.time_ns():x}"

    idle = request(
        args.url,
        args.model,
        args.short_length,
        args.short_output,
        nonce + "-idle",
        args.ignore_eos,
    )

    holder = {}
    thread = threading.Thread(
        target=lambda: holder.setdefault(
            "long",
            request(args.url, args.model, args.long_length, 1, nonce + "-long"),
        ),
        daemon=False,
    )
    thread.start()
    time.sleep(args.inject_delay)
    injected = request(
        args.url,
        args.model,
        args.short_length,
        args.short_output,
        nonce + "-inject",
        args.ignore_eos,
    )
    thread.join()

    result = {
        "idle_short": idle,
        "long": holder["long"],
        "injected_short": injected,
        "inject_delay_s": args.inject_delay,
    }
    rendered = json.dumps(result, ensure_ascii=False, indent=2)
    if args.output_json:
        output_path = Path(args.output_json)
        output_path.parent.mkdir(parents=True, exist_ok=True)
        output_path.write_text(rendered + "\n", encoding="utf-8")
    print(rendered)


if __name__ == "__main__":
    main()
