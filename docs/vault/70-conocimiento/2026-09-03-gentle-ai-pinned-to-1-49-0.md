---
type: constraint
score: 4
topic_key: mascotapp/ops/gentle-ai-pinned-until-phase-02-closes
task: T-02-005
status: guardado
observation_id: obs-7b890b7ace39e1e4
rationale: "Una herramienta de orquestación que va tres majors atrás genera presión constante para actualizar, y cada sesión nueva ve `gentle-ai update` reportando 2.5.0 sin saber por qué nadie lo subió. Sin la razón escrita, alguien la sube a mitad de la cadena de PRs y descubre después si el store del runtime sobrevivió. La decisión importa menos que el criterio: no se cambia de major una herramienta que lleva estado de la fase en curso."
---

# `gentle-ai` fijado en 1.49.0 hasta cerrar la Fase 02

Decisión del usuario, 2026-09-03. `gentle-ai update` reporta `latest: 2.5.0` — **no se
sube.**

## El riesgo concreto, no genérico

El store del runtime SDD vive en `.git/gentle-ai/sdd-runtime/`**`v1`**`/<change>/records/`
y contiene la cadena de objetivos de los PRs ya cerrados de la fase (`objective/advance`
encadenados por `previous_revision`). Ese `v1` en la ruta es un número de esquema. Un
binario 2.x puede no leerlo, y el momento de descubrirlo no es a mitad de una cadena de 24
PRs con 5 cerrados.

Además, el instalador publicado de 2.x es un one-liner `irm … | iex`: curl a shell, que ya
está en la lista `deny` del proyecto y es una acción de cadena de suministro.

## Qué NO tiene 1.49.0 — verificado ejecutándolo, no leyendo docs

- **No existe el comando `review`.** Ni `review status --contract
  gentle-ai.review-integration/v2 --agent claude-code --next-transition`, ni
  `review mode enable|disable|status`. Todo el contrato RDD que describe el `CLAUDE.md`
  global es una superficie de 2.x.
- **No existe `sdd-attempt`.** El registro de intentos se escribe internamente en el store;
  no hay CLI para settlear ni resetear.

Los skills instalados en `~/.claude` (`sdd-*`, `review-*`, `jd-*`) **están adelantados
respecto del binario**: nombran comandos que esta versión no tiene. Un agente que siga el
skill al pie de la letra va a invocar comandos inexistentes.

## Por qué esto no bloquea nada

Lo que el flujo realmente usa —`sdd-status` y `sdd-continue`— funciona: reporta
`apply: ready`, `blockedReasons: []`. RDD está apagado y encenderlo es decisión del usuario,
nunca del agente. TDD, la suite, mutation testing, presupuestos por PR y commits no
dependen del ledger en absoluto.

## El orden correcto el día que haga falta RDD

**Primero subir de versión y verificar que el store sigue legible; después encender.**
Nunca al revés — encender RDD contra un binario que no tiene el comando produce un fallo
que se lee como un problema del repositorio y no lo es.

La regla general, que es lo transferible: **no se cambia de major una herramienta que lleva
estado de la unidad de trabajo en curso.** Se cambia entre unidades, con el store
verificado, o no se cambia.

Relacionado: [[2026-09-03-tool-absence-must-fail-loudly]] — la otra mitad de la misma
lección, que la salud de las herramientas se chequea ejecutándolas al inicio de la sesión.
