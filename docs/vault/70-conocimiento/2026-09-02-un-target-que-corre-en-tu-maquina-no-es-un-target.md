---
type: convention
score: 4
topic_key: mascotapp/convention/makefile-tool-provenance
task: PR-02-01 (T-02-001/002)
approved: 2026-09-02 (aprobacion explicita del usuario)
observation_id: obs-011873b42b4352bd
rationale: "El guard convierte una clase entera de bug 'funciona en mi maquina' en un fallo de CI. Sin entender por que rechaza interpretes, el proximo lo ensancha y lo desarma."
---

`TestDevcontainerInstallsEveryToolTheMakefileInvokes` (`apps/api/internal/db/wiring_test.go`)
exige que **toda** herramienta que invoca un recipe del Makefile este instalada por
`.devcontainer/postCreate.sh`. Y construye su lista de instalados **exclusivamente** desde
lineas `go install <modulo>@<version>`, tomando el ultimo segmento del path como nombre del
binario.

**La consecuencia no es obvia y muerde: un interprete no puede satisfacer ese guard nunca.**
Agregar un recipe que corre `python`, `node` o `bash script.sh` lo pone rojo por construccion,
no por olvido de instalarlo.

Paso de verdad el 2026-09-02: se agrego `engram-index: python scripts/engram-index.py`, paso en
el host del autor y reventó el guard. **El guard tenia razon** — ese target se rompia en cualquier
devcontainer limpio.

**La salida correcta es cambiar el target, no el guard.** Se reescribio el generador en Go
(`scripts/engram-index.go` con `//go:build ignore`, corriendo por `go run`), porque `go` ya esta
en el conjunto ambiente. La paridad se probo diffeando la salida contra la version anterior
commiteada: byte-identica.

Ensanchar el guard para que acepte el cambio es la misma movida que bajarle el estimado a un PR
para que entre en el presupuesto: el numero entra y la barrera deja de significar algo.
