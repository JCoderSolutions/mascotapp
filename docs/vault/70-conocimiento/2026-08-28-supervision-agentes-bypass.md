---
type: constraint
score: 5
topic_key: mascotapp/ops/agent-supervision
task: T-00-017
status: guardado
observation_id: obs-86bec7dbc4378a13
rationale: "Determina qué barreras son reales y cuáles son ilusión. Sin esto se confía en protecciones que no existen."
---

Qué se hace cumplir de verdad en Claude Code, verificado contra la documentación
oficial (agosto 2026):

- **Reglas `deny`: bloquean en TODOS los modos, incluido `bypassPermissions`.**
- **Reglas `ask`: fuerzan prompt al humano incluso en bypass.**
- **Reglas `allow`: NO tienen ningún efecto en bypass.** Un allowlist ahí es inútil.
- **Los bloqueos del modo plan NO se aplican** en sesiones con bypass disponible:
  el modo plan es advertencia, no barrera.
- Escrituras a rutas protegidas (`.git`, `.claude`) están permitidas en bypass.
- Hooks `PreToolUse`: **no documentado** si disparan en bypass. Hay que probarlo, no asumirlo.
  Bloquean con `exit 2` o `permissionDecision: "deny"`; **`exit 1` NO bloquea**.
- Cortacircuitos nativo: `rm`/`rmdir` sobre raíz, directorios de primer nivel, home,
  raíces de unidad Windows, **y el cwd y sus padres** → pregunta incluso en bypass, y
  ningún `allow` ni hook `"allow"` lo aprueba.

**Trampa encontrada en la práctica:** las reglas Bash **sin `*` final son coincidencia
exacta**. `Bash(rm -rf /)` no cubre `rm -rf ./apps` ni nada parecido.

**Límite honesto:** las reglas comparan la *cadena del comando*, no su efecto. Un
`make clean` que internamente corre `rm -rf` no se bloquea. Son barandas contra error del
modelo, **no un sandbox**. El aislamiento en contenedor es lo único que acota el daño.

Diseño adoptado: `Bash(rm *)` denegado por completo; todo borrado legítimo pasa por un
target de `Makefile` bajo revisión de código.
