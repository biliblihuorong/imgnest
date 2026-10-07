#!/usr/bin/env python3
"""Guard CI cache wiring and the full quality gate without extra dependencies.

Pass --compose to also validate Docker Compose's actual merged configuration.
"""
import importlib.util
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

    def test_go_builder_identity_is_immutable_and_shared(self):
        dev = (ROOT / "deploy/Dockerfile.dev").read_text().splitlines()[0]
        minio = (ROOT / "deploy/Dockerfile.minio-test").read_text()
        self.assertRegex(dev, r"^FROM golang:1\.27\.1-bookworm@sha256:[0-9a-f]{64} AS go-toolchain$")
        self.assertEqual(dev.split()[1], minio.splitlines()[0].split()[1])
        self.assertIn("rm -rf /go/pkg/mod /root/.cache/go-build", minio)

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


def load_scope():
    spec = importlib.util.spec_from_file_location("ci_scope", ROOT / "scripts/ci-scope.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class CIScopeTest(unittest.TestCase):
    def scopes(self, *paths):
        return load_scope().scopes(paths)

    def test_documentation_only_changes_skip_every_job(self):
        self.assertEqual(self.scopes("README.md", "docs/spec.md", "docs/planning/m5.md",
                                     ".claude/skills/lsky-api-compat/SKILL.md"),
                         {"backend": False, "frontend": False})

    def test_project_website_changes_skip_every_job(self):
        # website/ is a standalone VitePress workspace that no CI job builds.
        self.assertEqual(self.scopes("website/docs/guide/introduction.md", "website/package.json",
                                     "website/.vitepress/theme/home/HomePage.vue"),
                         {"backend": False, "frontend": False})
        self.assertEqual(self.scopes("website/docs/index.md", "internal/service/user.go"),
                         {"backend": True, "frontend": False})

    def test_vben_only_changes_skip_the_go_jobs(self):
        self.assertEqual(self.scopes("web-vben/src/App.vue", "web-vben/pnpm-lock.yaml", "docs/spec.md"),
                         {"backend": False, "frontend": True})

    def test_go_only_changes_skip_the_frontend_jobs(self):
        self.assertEqual(self.scopes("internal/service/user.go", "cmd/imgnest/main.go", "go.sum",
                                     ".golangci.yml"),
                         {"backend": True, "frontend": False})

    def test_go_inputs_inside_frontend_directories_run_the_go_jobs(self):
        # Both dist trees are embedded and web/ is hashed by the legacy source guard.
        for path in ("web-vben/embed.go", "web-vben/dist/.gitkeep", "web/src/App.vue", "web/README.md"):
            self.assertEqual(self.scopes(path), {"backend": True, "frontend": True}, path)

    def test_shared_and_unknown_paths_run_everything(self):
        for path in (".github/workflows/ci.yml", "scripts/ci-scope.py", "deploy/Dockerfile.dev",
                     "docs/openapi.yaml", "docs/planning/legacy-m5-source.sha256", "Makefile",
                     "internal/http/lsky/README.md", "new-top-level-file"):
            self.assertEqual(self.scopes(path), {"backend": True, "frontend": True}, path)

    def test_an_unknown_or_truncated_change_list_runs_everything(self):
        everything = {"backend": True, "frontend": True}
        self.assertEqual(self.scopes(), everything)
        self.assertEqual(self.scopes("", "  "), everything)
        # The pull request files API stops at 3000 entries.
        self.assertEqual(self.scopes(*["docs/spec.md"] * 3000), everything)

    def test_command_line_prints_step_outputs(self):
        def run(*args, stdin=""):
            return subprocess.run([sys.executable, str(ROOT / "scripts/ci-scope.py"), *args],
                                  input=stdin, capture_output=True, text=True, check=True).stdout
        self.assertEqual(run(stdin="docs/spec.md\nweb-vben/src/App.vue\n"),
                         "backend=false\nfrontend=true\n")
        self.assertEqual(run("--all", stdin="docs/spec.md\n"), "backend=true\nfrontend=true\n")


class CIScopeWiringTest(unittest.TestCase):
    def setUp(self):
        self.workflow = (ROOT / ".github/workflows/ci.yml").read_text()

    def test_go_and_frontend_jobs_follow_the_detected_scope(self):
        self.assertEqual(self.workflow.count("needs: changes"), 3)
        self.assertEqual(self.workflow.count("if: needs.changes.outputs.backend != 'false'"), 2)
        self.assertEqual(self.workflow.count("if: needs.changes.outputs.frontend != 'false'"), 1)

    def test_only_pull_requests_may_skip_jobs(self):
        # Pushes to main seed the caches every pull request restores from.
        self.assertIn('if [ "$GITHUB_EVENT_NAME" = pull_request ] && files=$(', self.workflow)
        self.assertIn('python3 scripts/ci-scope.py --all >> "$GITHUB_OUTPUT"', self.workflow)
        # A pipeline would hide a failing scope script behind tee's exit status.
        self.assertNotIn("ci-scope.py |", self.workflow)
        self.assertIn(".previous_filename // empty", self.workflow)
        self.assertNotIn("paths-ignore", self.workflow)


if __name__ == "__main__":
    unittest.main()
