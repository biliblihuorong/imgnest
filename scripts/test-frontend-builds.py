#!/usr/bin/env python3
"""Exercise Make's frontend/release dependency routing without invoking Docker."""
import pathlib
import subprocess
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]


class FrontendBuildRoutingTest(unittest.TestCase):
    def commands(self, target):
        result = subprocess.run(
            ["make", "--no-print-directory", "--dry-run", target],
            cwd=ROOT, text=True, capture_output=True, check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        return result.stdout

    def test_legacy_build_is_frozen_and_independent(self):
        commands = self.commands("fe-build-legacy")
        self.assertIn("cd web && pnpm install --frozen-lockfile", commands)
        self.assertIn("pnpm build", commands)
        self.assertNotIn("gen:api", commands)
        self.assertNotIn("web-vben", commands)

    def test_vben_build_uses_its_own_install_and_output(self):
        commands = self.commands("fe-build-vben")
        self.assertIn("cd web-vben && pnpm install --frozen-lockfile", commands)
        self.assertIn("pnpm build", commands)
        self.assertNotIn("cd web &&", commands)
        self.assertNotIn("gen:api", commands)

    def test_release_legacy_selects_default_go_build(self):
        commands = self.commands("release-legacy")
        self.assertIn("-o bin/imgnest-legacy ./cmd/imgnest", commands)
        self.assertNotIn("-tags", commands)
        self.assertNotIn("web-vben", commands)

    def test_release_vben_selects_tagged_go_build(self):
        commands = self.commands("release-vben")
        self.assertIn("go build -tags vben -trimpath -o bin/imgnest-vben ./cmd/imgnest", commands)
        self.assertNotIn("cd web &&", commands)

    def test_default_release_and_frontend_build_preserve_legacy(self):
        self.assertEqual(self.commands("release"), self.commands("release-legacy"))
        self.assertEqual(self.commands("fe-build"), self.commands("fe-build-legacy"))


if __name__ == "__main__":
    unittest.main()
