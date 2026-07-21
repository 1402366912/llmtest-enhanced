#!/usr/bin/env python3
"""A/B benchmark for MTP/speculative decode on an OpenAI-compatible server."""

from __future__ import annotations

import argparse
import concurrent.futures
import json
import statistics
import threading
import time
from dataclasses import dataclass, asdict
from datetime import datetime, timezone
from pathlib import Path

import requests


DOMAIN_PROMPTS = [
    (
        "python",
        "实现一个生产可用的 Python 异步任务调度器：支持优先级、超时、取消、指数退避重试和优雅关闭。"
        "请给出完整代码、设计说明、复杂度分析和 pytest 测试，回答不少于 1200 字。",
    ),
    (
        "math",
        "从泰勒展开和凸性两个角度推导 log-sum-exp 的数值稳定形式，并证明它是 max 函数的平滑近似。"
        "进一步推导梯度与 Hessian，讨论温度趋近 0 和无穷时的极限，逐步写出公式和直觉解释。",
    ),
    (
        "distributed_systems",
        "为每秒百万事件的多租户日志平台设计端到端架构。需要覆盖分区、复制、一致性、背压、"
        "冷热分层、故障恢复、容量规划、可观测性和成本控制，并分析至少三种设计取舍。",
    ),
    (
        "science",
        "面向理工科本科生系统解释恒星从分子云坍缩到主序、红巨星以及最终残骸的完整演化。"
        "请联系质量、核反应、简并压、钱德拉塞卡极限和元素合成，避免只给结论。",
    ),
    (
        "history",
        "比较汉帝国与罗马帝国在税制、军队职业化、边疆治理、交通网络和地方精英整合方面的异同。"
        "要求给出有层次的长篇论证，区分事实、主流解释和存在争议的推断。",
    ),
    (
        "data_analysis",
        "某订阅产品最近三个月新增用户增长 30%，但收入只增长 5%，退款率和客服工单同时上升。"
        "请制定完整的数据分析方案：指标树、分群、因果假设、SQL 数据需求、实验设计、风险和决策标准。",
    ),
]


@dataclass
class Result:
    domain: str
    prompt_tokens: int
    output_tokens: int
    ttft_s: float
    decode_s: float
    total_s: float
    decode_tps: float
    tpot_ms: float
    started_at: float
    first_at: float
    ended_at: float


def normalize_url(url: str) -> str:
    url = url.rstrip("/")
    if url.endswith("/chat/completions"):
        return url
    if url.endswith("/v1"):
        return url + "/chat/completions"
    return url + "/v1/chat/completions"


def run_request(
    *,
    url: str,
    model: str,
    domain: str,
    prompt: str,
    max_tokens: int,
    ignore_eos: bool,
    barrier: threading.Barrier | None = None,
) -> Result:
    if barrier is not None:
        barrier.wait()
    body = {
        "model": model,
        "messages": [
            {"role": "system", "content": "You are a precise and thorough expert."},
            {"role": "user", "content": prompt},
        ],
        "max_tokens": max_tokens,
        "temperature": 0,
        "top_p": 1,
        "ignore_eos": ignore_eos,
        "stream": True,
        "stream_options": {"include_usage": True},
    }
    start = time.perf_counter()
    first = None
    end = None
    usage: dict = {}
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
        end = time.perf_counter()
    first = first or end
    output_tokens = int(usage.get("completion_tokens") or max_tokens)
    prompt_tokens = int(usage.get("prompt_tokens") or 0)
    decode_s = max(end - first, 0.001)
    token_intervals = max(output_tokens - 1, 1)
    return Result(
        domain=domain,
        prompt_tokens=prompt_tokens,
        output_tokens=output_tokens,
        ttft_s=round(first - start, 4),
        decode_s=round(decode_s, 4),
        total_s=round(end - start, 4),
        decode_tps=round(output_tokens / decode_s, 2),
        tpot_ms=round(decode_s * 1000 / token_intervals, 3),
        started_at=start,
        first_at=first,
        ended_at=end,
    )


def summarize(results: list[Result]) -> dict:
    wall_start = min(result.started_at for result in results)
    wall_first = min(result.first_at for result in results)
    wall_end = max(result.ended_at for result in results)
    total_output = sum(result.output_tokens for result in results)
    return {
        "requests": len(results),
        "total_prompt_tokens": sum(result.prompt_tokens for result in results),
        "total_output_tokens": total_output,
        "wall_s": round(wall_end - wall_start, 4),
        "aggregate_e2e_output_tps": round(total_output / (wall_end - wall_start), 2),
        "aggregate_decode_tps": round(total_output / max(wall_end - wall_first, 0.001), 2),
        "median_ttft_s": round(statistics.median(r.ttft_s for r in results), 4),
        "p95_ttft_s": round(sorted(r.ttft_s for r in results)[max(0, int(len(results) * 0.95) - 1)], 4),
        "median_request_decode_tps": round(
            statistics.median(r.decode_tps for r in results), 2
        ),
        "results": [asdict(result) for result in results],
    }


def fetch_spec_metrics(base_url: str) -> list[str]:
    root = base_url.split("/v1", 1)[0].rstrip("/")
    try:
        response = requests.get(root + "/metrics", timeout=10)
        response.raise_for_status()
    except requests.RequestException as exc:
        return [f"metrics_error: {exc}"]
    keys = ("spec", "draft", "accept")
    return [
        line
        for line in response.text.splitlines()
        if line and not line.startswith("#") and any(key in line.lower() for key in keys)
    ]


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", default="http://127.0.0.1:60015")
    parser.add_argument("--model", default="Qwen3.6-27B-AWQ")
    parser.add_argument("--domain-max-tokens", type=int, default=512)
    parser.add_argument("--stress-max-tokens", type=int, default=512)
    parser.add_argument("--concurrency", default="1,2,4,8")
    parser.add_argument("--label", required=True)
    parser.add_argument("--output-json", required=True)
    args = parser.parse_args()
    url = normalize_url(args.url)
    concurrency_levels = [int(value) for value in args.concurrency.split(",")]

    metrics_before = fetch_spec_metrics(args.url)
    natural_results = [
        run_request(
            url=url,
            model=args.model,
            domain=domain,
            prompt=prompt,
            max_tokens=args.domain_max_tokens,
            ignore_eos=False,
        )
        for domain, prompt in DOMAIN_PROMPTS
    ]

    stress = {}
    for concurrency in concurrency_levels:
        barrier = threading.Barrier(concurrency)
        with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as pool:
            futures = []
            for index in range(concurrency):
                domain, prompt = DOMAIN_PROMPTS[index % len(DOMAIN_PROMPTS)]
                futures.append(
                    pool.submit(
                        run_request,
                        url=url,
                        model=args.model,
                        domain=f"{domain}-{index}",
                        prompt=prompt + f"\n[独立压力测试请求 {args.label}-{concurrency}-{index}]",
                        max_tokens=args.stress_max_tokens,
                        ignore_eos=True,
                        barrier=barrier,
                    )
                )
            stress[str(concurrency)] = summarize([future.result() for future in futures])

    document = {
        "schema": "1cat-llmtest.mtp-decode.v1",
        "created_at": datetime.now(timezone.utc).isoformat(),
        "label": args.label,
        "model": args.model,
        "url": url,
        "domain_max_tokens": args.domain_max_tokens,
        "stress_max_tokens": args.stress_max_tokens,
        "natural_domains": summarize(natural_results),
        "concurrency": stress,
        "spec_metrics_before": metrics_before,
        "spec_metrics_after": fetch_spec_metrics(args.url),
    }
    output = Path(args.output_json)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(document, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(document, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
