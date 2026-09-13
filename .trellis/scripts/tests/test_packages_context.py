"""Regression coverage for package spec discovery and public context outputs.

Run: python3 -B -m unittest discover -s .trellis/scripts/tests -v
"""

from __future__ import annotations

import json
from pathlib import Path
import sys
from tempfile import TemporaryDirectory
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from common.packages_context import (
    get_context_packages_json,
    get_context_packages_text,
    get_packages_info,
    get_packages_section,
)


class PackagesContextTests(unittest.TestCase):
    def setUp(self):
        temporary = TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        # Isolate active-task identity from the host session, while retaining
        # real config parsing, filesystem discovery, and task.json loading.
        task_patch = patch("common.packages_context.get_current_task", return_value=None)
        self.current_task = task_patch.start()
        self.addCleanup(task_patch.stop)

    def write(self, relative_path, content="# Spec\n"):
        path = self.root / relative_path
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")

    def configure_packages(self, session=""):
        self.write(
            ".trellis/config.yaml",
            "default_package: flat\n"
            "packages:\n"
            "  flat:\n"
            "    path: app/flat/service\n"
            "  layered:\n"
            "    path: ../layered\n"
            "    git: true\n"
            "  mixed:\n"
            "    path: packages/mixed\n"
            "    type: submodule\n"
            "  missing:\n"
            "    path: packages/missing\n"
            + session,
        )
        self.write(".trellis/spec/flat/index.md")
        self.write(".trellis/spec/flat/topic.md")
        self.write(".trellis/spec/layered/backend/index.md")
        self.write(".trellis/spec/mixed/index.md")
        self.write(".trellis/spec/mixed/security/index.md")
        self.write(".trellis/spec/mixed/api/index.md")
        self.write(".trellis/spec/guides/index.md")

    def test_monorepo_json_reports_root_index_separately_from_real_layers(self):
        self.configure_packages()
        output = json.loads(json.dumps(get_context_packages_json(self.root)))
        self.assertEqual(output["mode"], "monorepo")
        self.assertEqual(output["defaultPackage"], "flat")
        self.assertIsNone(output["specScope"])
        self.assertIsNone(output["activeTaskPackage"])
        self.assertEqual(output["packages"], get_packages_info(self.root))
        packages = {pkg["name"]: pkg for pkg in output["packages"]}
        expected = {
            "flat": (".trellis/spec/flat/index.md", []),
            "layered": (None, ["backend"]),
            "mixed": (".trellis/spec/mixed/index.md", ["api", "security"]),
            "missing": (None, []),
        }
        self.assertEqual(set(packages), set(expected))
        for name, (index, layers) in expected.items():
            with self.subTest(package=name):
                self.assertEqual(packages[name]["specIndex"], index)
                self.assertEqual(packages[name]["specLayers"], layers)
                if index:
                    self.assertTrue((self.root / index).is_file())
        self.assertTrue(packages["flat"]["default"])
        self.assertTrue(packages["layered"]["isGitRepo"])
        self.assertEqual(packages["layered"]["path"], "../layered")
        self.assertTrue(packages["mixed"]["isSubmodule"])

    def test_monorepo_text_distinguishes_flat_layered_mixed_and_missing(self):
        self.configure_packages()
        text = get_context_packages_text(self.root)
        self.assertIn(
            "### flat (default)\nPath: app/flat/service\n"
            "Spec index: .trellis/spec/flat/index.md\n\n",
            text,
        )
        self.assertIn(
            "### layered [git repo]\nPath: ../layered\n"
            "Spec layers: backend\n"
            "  - .trellis/spec/layered/backend/index.md\n\n",
            text,
        )
        self.assertIn(
            "### mixed [submodule]\nPath: packages/mixed\n"
            "Spec index: .trellis/spec/mixed/index.md\n"
            "Spec layers: api, security\n"
            "  - .trellis/spec/mixed/api/index.md\n"
            "  - .trellis/spec/mixed/security/index.md\n\n",
            text,
        )
        self.assertIn("Path: packages/missing\nSpec: not configured\n", text)
        self.assertEqual(text.count("Spec: not configured"), 1)
        self.assertIn(
            "### Shared Guides (always included)\n"
            "Path: .trellis/spec/guides/index.md\n",
            text,
        )

    def test_compact_section_keeps_metadata_and_exposes_root_indexes(self):
        self.configure_packages()
        section = get_packages_section(self.root)
        rows = {line.split()[1]: line for line in section.splitlines() if line.startswith("- ")}
        self.assertIn("(spec: .trellis/spec/flat/index.md)", rows["flat"])
        self.assertTrue(rows["flat"].endswith("  *"))
        self.assertNotIn("[", rows["flat"])
        self.assertIn("[backend]  (git repo)", rows["layered"])
        self.assertNotIn("(spec:", rows["layered"])
        self.assertIn(
            "[api, security]  (spec: .trellis/spec/mixed/index.md)  (submodule)",
            rows["mixed"],
        )
        self.assertNotIn("(spec:", rows["missing"])
        self.assertIn("Default package: flat", section)

    def test_missing_root_index_requires_a_file(self):
        self.configure_packages()
        for layout in ("absent", "topic-only", "index-directory"):
            with self.subTest(layout=layout):
                if layout == "topic-only":
                    self.write(".trellis/spec/missing/topic.md")
                elif layout == "index-directory":
                    (self.root / ".trellis/spec/missing/index.md").mkdir()
                packages = get_packages_info(self.root)
                missing = next(pkg for pkg in packages if pkg["name"] == "missing")
                self.assertIsNone(missing["specIndex"])
                self.assertNotIn(
                    "Spec index: .trellis/spec/missing/index.md",
                    get_context_packages_text(self.root),
                )
                self.assertNotIn(
                    "(spec: .trellis/spec/missing/index.md)", get_packages_section(self.root)
                )

    def test_scope_annotations_preserve_discovery_and_shared_guides(self):
        cases = (
            ("", None, None, None),
            ("  spec_scope:\n    - flat\n", None, ["flat"], {"flat"}),
            ("  spec_scope: active_task\n", "layered", "active_task", {"layered"}),
            ("  spec_scope: active_task\n", None, "active_task", {"flat"}),
            ("  spec_scope: active_task\n", "unknown", "active_task", {"flat"}),
            ("  spec_scope:\n    - unknown\n", "mixed", ["unknown"], {"mixed"}),
            ("  spec_scope:\n    - unknown\n", None, ["unknown"], {"flat"}),
        )
        for session, task_package, scope, allowed in cases:
            with self.subTest(scope=scope, task_package=task_package):
                self.configure_packages("session:\n" + session if session else "")
                if task_package:
                    self.write(
                        ".trellis/tasks/active/task.json", json.dumps({"package": task_package})
                    )
                    self.current_task.return_value = ".trellis/tasks/active"
                else:
                    self.current_task.return_value = None
                output = get_context_packages_json(self.root)
                self.assertEqual(output["specScope"], scope)
                self.assertEqual(output["activeTaskPackage"], task_package)
                self.assertEqual(len(output["packages"]), 4)
                text = get_context_packages_text(self.root)
                for pkg in output["packages"]:
                    heading = next(
                        line for line in text.splitlines()
                        if line.startswith(f"### {pkg['name']}")
                    )
                    self.assertEqual(
                        "(out of scope)" in heading,
                        allowed is not None and pkg["name"] not in allowed,
                    )
                self.assertIn("Spec index: .trellis/spec/flat/index.md", text)
                self.assertIn("Spec index: .trellis/spec/mixed/index.md", text)
                self.assertIn("### Shared Guides (always included)", text)

    def test_single_repo_layered_output_is_preserved(self):
        self.write(".trellis/spec/frontend/index.md")
        self.write(".trellis/spec/backend/index.md")
        self.write(".trellis/spec/guides/index.md")
        self.assertEqual(get_packages_info(self.root), [])
        self.assertEqual(get_context_packages_json(self.root), {
            "mode": "single-repo", "specLayers": ["backend", "frontend"], "specIndex": None,
        })
        self.assertEqual(
            get_context_packages_text(self.root),
            "Single-repo project (no packages configured)\n\nSpec layers: backend, frontend",
        )
        self.assertEqual(
            get_packages_section(self.root),
            "## PACKAGES\n(single-repo mode)\nSpec layers: backend, frontend",
        )

    def test_single_repo_flat_and_mixed_expose_the_real_root_index(self):
        self.write(".trellis/spec/index.md")
        self.write(".trellis/spec/guides/index.md")
        for layers in ([], ["backend"]):
            with self.subTest(layers=layers):
                if layers:
                    self.write(".trellis/spec/backend/index.md")
                self.assertEqual(get_context_packages_json(self.root), {
                    "mode": "single-repo",
                    "specLayers": layers,
                    "specIndex": ".trellis/spec/index.md",
                })
                for output in (
                    get_context_packages_text(self.root), get_packages_section(self.root)
                ):
                    self.assertIn("Spec index: .trellis/spec/index.md", output)
                    self.assertEqual("Spec layers:" in output, bool(layers))
                    if layers:
                        self.assertIn("Spec layers: backend", output)

    def test_single_repo_without_specs_is_preserved(self):
        self.assertEqual(get_context_packages_json(self.root), {
            "mode": "single-repo", "specLayers": [], "specIndex": None,
        })
        self.assertEqual(
            get_context_packages_text(self.root), "Single-repo project (no packages configured)\n"
        )
        self.assertEqual(get_packages_section(self.root), "## PACKAGES\n(single-repo mode)")


if __name__ == "__main__":
    unittest.main()
