---
type: convention
score: 3
topic_key: mascotapp/convention/commit-messages
task: handoff multi-agente
approved: 2026-09-02 (el usuario pidio explicitamente guardarlo)
observation_id: obs-62d33d3023d8afae
rationale: "Toda herramienta que toque este repo escribe commits. Sin la convencion escrita, cada agente nuevo inventa la suya y el historial deja de ser navegable."
---

Los commits de MascotApp siguen **Conventional Commits**, y la convencion esta
escrita en dos lugares ejecutables, no en la memoria de quien conduce:
`docs/vault/99-plantillas/plantilla-commit.md` (la regla completa) y `.gitmessage`
(el andamiaje que abre `git commit`, activable con
`git config commit.template .gitmessage`).

Formato: `<tipo>(<alcance>): <T-FF-NNN> <descripcion en imperativo>`, resumen de
72 caracteres como maximo, sin punto final, cuerpo opcional que explica el POR QUE.

**Tres reglas del proyecto que no vienen del estandar:**

1. **El ID de tarea va al principio de la descripcion.** Es lo unico que permite
   ir del historial al tablero de `docs/vault/30-fases/` y de vuelta.
2. **El mensaje va en ingles** aunque la conversacion y el vault sean en espanol.
   Un mensaje de commit es un artefacto tecnico, y la invariante 4 del proyecto
   dice que los artefactos tecnicos van en ingles.
3. **Sin `Co-Authored-By` y sin ninguna atribucion a IA.** Es regla explicita del
   usuario y vale por encima del default de la herramienta: varias herramientas
   agregan la linea sola, y hay que quitarla.

Hoy la convencion se sostiene por disciplina. Si se quiere hacer cumplir de
verdad, el camino es `commitlint` con `@commitlint/config-conventional` como
puerta bloqueante en CI. Esta anotado como opcion, no como decision tomada.
