#!/usr/bin/env python3
"""Render the manifest for Workfloor hosts supporting automation agent tools."""
from pathlib import Path
source = (Path(__file__).resolve().parent.parent / "manifest.yaml").read_text()
needle = "surfaces: [kanban-task, office-task]"
if source.count(needle) != 2:
    raise SystemExit("Expected both notification tool declarations")
print(source.replace(needle, "surfaces: [kanban-task, office-task, automation]"), end="")
