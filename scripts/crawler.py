#!/usr/bin/env python3
"""Simple crawler using ProxyPool as HTTP proxy.

Usage:
    python3 scripts/crawler.py [--count N] [--target URL] [--delay S]

Default: 50 requests to http://myip.ipip.net with 0.5s delay.
Each request goes through the proxypool (127.0.0.1:7890), which picks
a random upstream proxy from the pool. Watch the dashboard Connections
tab to see active connections in real time.
"""
import argparse
import time
import urllib.request
import urllib.error
import sys


PROXY = "http://127.0.0.1:7890"


def fetch(url: str, timeout: int = 10) -> tuple[int, str]:
    """Fetch a URL through the proxy pool. Returns (status, body)."""
    proxy_handler = urllib.request.ProxyHandler({"http": PROXY, "https": PROXY})
    opener = urllib.request.build_opener(proxy_handler)
    req = urllib.request.Request(url, headers={"User-Agent": "ProxyPool-Crawler/1.0"})
    try:
        with opener.open(req, timeout=timeout) as resp:
            body = resp.read().decode("utf-8", errors="replace").strip()
            return resp.status, body[:200]
    except urllib.error.HTTPError as e:
        return e.code, f"HTTPError: {e.reason}"
    except Exception as e:
        return 0, f"Error: {e}"


def main():
    parser = argparse.ArgumentParser(description="ProxyPool crawler demo")
    parser.add_argument("--count", type=int, default=50, help="Number of requests (default: 50)")
    parser.add_argument("--target", default="http://myip.ipip.net", help="Target URL")
    parser.add_argument("--delay", type=float, default=0.5, help="Delay between requests in seconds")
    args = parser.parse_args()

    print(f"ProxyPool Crawler")
    print(f"  Proxy:  {PROXY}")
    print(f"  Target: {args.target}")
    print(f"  Count:  {args.count}")
    print(f"  Delay:  {args.delay}s")
    print(f"{'='*60}")

    ok = fail = 0
    start = time.time()

    for i in range(1, args.count + 1):
        status, body = fetch(args.target)
        if 200 <= status < 400:
            ok += 1
            marker = "✓"
        else:
            fail += 1
            marker = "✗"

        elapsed = time.time() - start
        rate = i / elapsed if elapsed > 0 else 0
        print(f"  [{i:3d}/{args.count}] {marker} HTTP {status} | {body[:80]}")

        if i < args.count:
            time.sleep(args.delay)

    elapsed = time.time() - start
    print(f"{'='*60}")
    print(f"Done: {ok} ok, {fail} failed in {elapsed:.1f}s ({args.count/elapsed:.1f} req/s)")
    print(f"Check http://127.0.0.1:9090 → Connections tab for live stats")


if __name__ == "__main__":
    main()
