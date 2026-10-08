#!/usr/bin/env python3
"""Decide which CI jobs a pull request's changed files need.

Reads one changed path per line on stdin and prints GitHub step outputs.
Pass --all to run everything regardless of the list. Only paths known to be
irrelevant to a job may skip it; anything unrecognised runs every job.
"""
import sys

# The pull request files API returns at most this many entries.
API_FILE_LIMIT = 3000
GO_ONLY_FILES = {"go.mod", "go.sum", ".golangci.yml"}


def is_documentation(path):
    # website/ is the standalone project site; it is deployed separately and
    # no CI job builds or embeds it.
    if path.startswith((".claude/", "website/")):
        return True
    return path.endswith(".md") and ("/" not in path or path.startswith("docs/"))


def is_vben_source(path):
    # embed.go and the embedded dist tree are inputs of the Go build.
    return (path.startswith("web-vben/") and not path.endswith(".go")
            and not path.startswith("web-vben/dist/"))


def is_go_source(path):
    if path in GO_ONLY_FILES:
        return True
    return path.startswith(("internal/", "cmd/")) and not path.endswith(".md")


def scopes(paths):
    paths = [path.strip() for path in paths if path.strip()]
    if not paths or len(paths) >= API_FILE_LIMIT:
        return {"backend": True, "frontend": True}
    relevant = [path for path in paths if not is_documentation(path)]
    return {
        "backend": any(not is_vben_source(path) for path in relevant),
        "frontend": any(not is_go_source(path) for path in relevant),
    }


def main():
    paths = [] if "--all" in sys.argv[1:] else sys.stdin.read().splitlines()
    for job, needed in scopes(paths).items():
        print(f"{job}={'true' if needed else 'false'}")


if __name__ == "__main__":
    main()
