#!/usr/bin/env python3
"""Generate the vault index of Engram memory candidates.

Reads the frontmatter of every file in `.engram/queue/` and writes
`docs/vault/20-arquitectura/indice-engram.md`.

Why this exists: Engram is a semantic index that only Claude Code can reach.
Any other agent (Kiro, OpenCode, a cold session without MCP) sees the repository
and nothing else. This script projects the memory layer into the vault so the
repository stays the operating truth, per rule IA-4 of the master plan.

Run with `make engram-index`. Regenerating is idempotent: the output is a pure
function of the queue, so a diff means the queue actually changed.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
QUEUE_DIR = REPO_ROOT / ".engram" / "queue"
OUTPUT = REPO_ROOT / "docs" / "vault" / "20-arquitectura" / "indice-engram.md"

FRONTMATTER_RE = re.compile(r"\A---\r?\n(.*?)\r?\n---\r?\n(.*)\Z", re.DOTALL)
SCALAR_RE = re.compile(r"^([a-z_]+):\s*(.*)$")


def parse(path: Path) -> dict[str, str] | None:
    """Return the frontmatter of one queue file plus a one-line summary."""
    match = FRONTMATTER_RE.match(path.read_text(encoding="utf-8"))
    if match is None:
        return None

    entry: dict[str, str] = {}
    for line in match.group(1).splitlines():
        scalar = SCALAR_RE.match(line)
        if scalar:
            entry[scalar.group(1)] = scalar.group(2).strip().strip('"')

    entry["file"] = path.name
    entry["summary"] = summarize(match.group(2))
    return entry


def summarize(body: str, limit: int = 130) -> str:
    """First paragraph of the body, unwrapped and flattened to one table cell.

    Queue files are hard-wrapped markdown, so a single line is usually half a
    sentence. Collect the whole paragraph, then truncate once.
    """
    paragraph: list[str] = []
    for raw in body.splitlines():
        line = raw.strip()
        if line.startswith(("#", "---", "|", ">")):
            continue
        if not line:
            if paragraph:
                break
            continue
        paragraph.append(line)

    if not paragraph:
        return "—"

    text = " ".join(paragraph).replace("**", "").replace("|", "/").replace("`", "")
    text = re.sub(r"\s+", " ", text).strip()
    return text if len(text) <= limit else text[: limit - 1].rstrip(" ,.;:") + "…"


def render(entries: list[dict[str, str]]) -> str:
    saved = [e for e in entries if e.get("observation_id")]
    pending = [e for e in entries if not e.get("observation_id")]

    lines = [
        "# Índice de memoria — Engram",
        "",
        "> **Generado por `make engram-index`. No editar a mano.**",
        "> Fuente: el frontmatter de `.engram/queue/*.md`.",
        "",
        "Engram es un índice semántico al que **solo llega Claude Code por MCP**.",
        "Cualquier otro agente —Kiro, OpenCode, una sesión fría sin MCP— ve el",
        "repositorio y nada más. Este archivo proyecta esa capa dentro del vault para",
        "que el repositorio siga siendo la verdad operativa (regla IA-4 del plan maestro).",
        "",
        "**El texto completo de cada decisión vive en su archivo de cola**, que está",
        "versionado. El `observation_id` es la misma nota dentro de Engram; sirve para",
        "trazabilidad, no es requisito para leerla.",
        "",
        f"- Candidatos totales: **{len(entries)}**",
        f"- Aprobados y guardados en Engram: **{len(saved)}**",
        f"- Pendientes de aprobación explícita del usuario: **{len(pending)}**",
        "",
    ]

    if pending:
        lines += [
            "## Pendientes de aprobación",
            "",
            "Nada de esto está en Engram todavía. Requiere el sí explícito del usuario",
            "antes de `mem_save` (§6.6 del plan maestro).",
            "",
            "| Tarea | Tipo | Score | Archivo | Qué dice |",
            "|---|---|---|---|---|",
        ]
        for e in sorted(pending, key=lambda x: x["file"]):
            lines.append(
                f"| `{e.get('task', '—')}` | {e.get('type', '—')} | {e.get('score', '—')} "
                f"| [{e['file']}](../../../.engram/queue/{e['file']}) | {e['summary']} |"
            )
        lines.append("")

    lines += ["## Guardadas", ""]

    # Every topic_key is unique, so grouping by the full key yields 70 sections of
    # one row each. Group by its namespace instead — the taxonomy of the plan.
    by_namespace: dict[str, list[dict[str, str]]] = {}
    for entry in saved:
        topic = entry.get("topic_key", "sin-topic")
        segments = topic.split("/")
        namespace = "/".join(segments[:2]) if len(segments) > 2 else topic
        by_namespace.setdefault(namespace, []).append(entry)

    for namespace in sorted(by_namespace):
        group = by_namespace[namespace]
        lines += [
            f"### `{namespace}/*` — {len(group)}",
            "",
            "| Tarea | `topic_key` | Score | Archivo | `observation_id` | Qué dice |",
            "|---|---|---|---|---|---|",
        ]
        for e in sorted(group, key=lambda x: (x.get("task", ""), x["file"])):
            leaf = e.get("topic_key", "—").split("/")[-1]
            lines.append(
                f"| `{e.get('task', '—')}` | `…/{leaf}` | {e.get('score', '—')} "
                f"| [{e['file']}](../../../.engram/queue/{e['file']}) "
                f"| `{e['observation_id']}` | {e['summary']} |"
            )
        lines.append("")

    return "\n".join(lines)


def main() -> int:
    if not QUEUE_DIR.is_dir():
        print(f"queue directory not found: {QUEUE_DIR}", file=sys.stderr)
        return 1

    entries = []
    for path in sorted(QUEUE_DIR.glob("*.md")):
        entry = parse(path)
        if entry is None:
            print(f"warning: no frontmatter in {path.name}", file=sys.stderr)
            continue
        entries.append(entry)

    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    OUTPUT.write_text(render(entries) + "\n", encoding="utf-8")
    print(f"wrote {OUTPUT.relative_to(REPO_ROOT)} ({len(entries)} entries)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
