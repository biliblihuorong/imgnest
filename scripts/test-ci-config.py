#!/usr/bin/env python3
"""Guard CI cache wiring and the full quality gate without extra dependencies.

Pass --compose to also validate Docker Compose's actual merged configuration.
"""
import json
import os
import pathlib
import subprocess
import sys
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
CHECK_COMPOSE = "--compose" in sys.argv
if CHECK_COMPOSE:
    sys.argv.remove("--compose")


class CICacheConfigTest(unittest.TestCase):
    def setUp(self):
        self.workflow = (ROOT / ".github/workflows/ci.yml").read_text()

    def test_full_quality_gate_is_preserved(self):
        for command in (
            "go test -race -shuffle=on -count=1 -timeout 40m ./...",
            "go vet ./...",
            "go build -trimpath -o /tmp/imgnest ./cmd/imgnest",
            "go build -tags vben -trimpath -o /tmp/imgnest-vben ./cmd/imgnest",
            "app: [web, web-vben]", "pnpm typecheck", "pnpm test",
            "pnpm vitest run", "pnpm build",
        ):
            self.assertIn(command, self.workflow)
        self.assertNotIn("continue-on-error", self.workflow)
        self.assertIn("contents: read", self.workflow)
        self.assertNotIn("packages: write", self.workflow)

    def test_minio_has_an_independent_image_cache(self):
        self.assertIn("file: deploy/Dockerfile.minio-test", self.workflow)
        self.assertIn("cache-from: type=gha,scope=minio-test", self.workflow)
        self.assertIn("cache-to: type=gha,scope=minio-test,mode=max", self.workflow)
        self.assertIn("up -d --no-build --wait postgres minio", self.workflow)

    def test_runtime_cache_is_restored_for_each_go_job(self):
        self.assertEqual(self.workflow.count("uses: actions/cache@v5"), 2)
        for job in ("backend", "lint"):
            self.assertIn(f"imgnest-{job}-v1-", self.workflow)
        self.assertIn("${{ github.sha }}", self.workflow)
        self.assertIn("deploy/compose.ci.yaml", self.workflow)
        self.assertIn('sudo chown -R "$(id -u):$(id -g)" "$IMGNEST_CI_CACHE_DIR"', self.workflow)

    def test_linter_is_a_pinned_binary_in_an_optional_image_target(self):
        dockerfile = (ROOT / "deploy/Dockerfile.dev").read_text()
        self.assertIn("target: lint", self.workflow)
        self.assertIn("dev golangci-lint run ./...", self.workflow)
        self.assertNotIn("go run github.com/golangci", self.workflow)
        self.assertIn("go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0", dockerfile)
        self.assertIn("FROM dev-base AS lint", dockerfile)
        self.assertTrue(dockerfile.rstrip().endswith("FROM dev-base AS dev"))

    def test_cheap_graph_check_precedes_full_test(self):
        self.assertLess(self.workflow.index("- name: Frontend selection graphs"),
                        self.workflow.index("- name: Test"))

    def test_ci_override_does_not_change_local_compose(self):
        local = (ROOT / "deploy/compose.dev.yaml").read_text()
        self.assertIn("go-modules:/go/pkg/mod", local)
        self.assertIn("go-build:/root/.cache/go-build", local)
        override = ROOT / "deploy/compose.ci.yaml"
        self.assertTrue(override.is_file(), "CI-only Compose override is missing")
        content = override.read_text()
        self.assertIn("${IMGNEST_CI_CACHE_DIR:?", content)
        self.assertNotIn("POSTGRES", content)
        self.assertNotIn("IMGNEST_TEST_", content)

    @unittest.skipUnless(CHECK_COMPOSE, "use --compose in CI to inspect actual Docker merge")
    def test_actual_compose_merge(self):
        config = json.loads(subprocess.check_output(
            ["docker", "compose", "-f", "deploy/compose.dev.yaml", "-f",
             "deploy/compose.ci.yaml", "config", "--format", "json"],
            cwd=ROOT, text=True,
        ))
        services = config["services"]
        mounts = {v["target"]: v for v in services["dev"]["volumes"]}
        for target, subdir in (("/go/pkg/mod", "go-modules"),
                               ("/root/.cache/go-build", "go-build"),
                               ("/root/.cache/golangci-lint", "golangci-lint")):
            self.assertEqual(mounts[target]["type"], "bind")
            self.assertEqual(mounts[target]["source"],
                             str(pathlib.Path(os.environ["IMGNEST_CI_CACHE_DIR"]) / subdir))
        self.assertEqual(services["dev"]["image"], os.environ["IMGNEST_CI_DEV_IMAGE"])
        self.assertEqual(services["minio"]["image"], "imgnest-ci-minio")
        env = services["dev"]["environment"]
        self.assertIn("host=postgres", env["IMGNEST_TEST_POSTGRES_DSN"])
        self.assertEqual(env["IMGNEST_TEST_S3_ENDPOINT"], "http://minio:9000")
        self.assertIn("healthcheck", services["postgres"])
        self.assertIn("tmpfs", services["minio"])


if __name__ == "__main__":
    unittest.main()
