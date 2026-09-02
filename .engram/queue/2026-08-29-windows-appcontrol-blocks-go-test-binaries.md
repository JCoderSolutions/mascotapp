---
type: constraint
score: 4
topic_key: mascotapp/ops/windows-appcontrol-go-tests
task: T-01-002 (spike testcontainers)
reviewed: 2026-09-01 (T-01-035, pase de curaduria)
disposition: descartado -- su causa raiz se probo FALSA y el que la corrige ya esta guardado
superseded_by: 2026-08-31-smart-app-control-no-es-defender.md  # causa raiz FALSA: era Smart App Control, no Defender ni el scratch
rationale: "Se presenta como fallo de test y no lo es. Y el arreglo obvio (reubicar el scratch) NO es el arreglo: es vaciarlo. Sin esto, alguien pierde una tarde depurando código que está bien, o se conforma con una cura que reaparece."
---

**En este host Windows, Application Control bloquea los binarios de test de Go recién
linkeados bajo `%LOCALAPPDATA%\Temp`.**

El síntoma engaña:

```
fork/exec C:\Users\Jose\AppData\Local\Temp\go-build.../dbtest.test.exe:
  An Application Control policy has blocked this file.
FAIL  github.com/.../internal/db/dbtest  1.307s
FAIL
```

`go test` sale con **exit 1** y la palabra `FAIL`. Parece que el test se rompió. No se
rompió: **la compilación anduvo y la ejecución nunca arrancó**. Ni un test corrió.

## Cómo se detecta

La política evalúa **por hash del binario**. Eso explica lo desconcertante: el mismo paquete
pasa, editás una línea, recompila, y ahora falla — porque el binario es otro archivo. Si
ves un `FAIL` que aparece justo después de un cambio inocuo y sin ninguna línea `--- FAIL`
de un test concreto, mirá el mensaje completo antes de tocar el código.

## Arreglo — y el diagnóstico equivocado del primer intento

**Primer intento, y estaba mal:** mover el scratch de Go fuera de `%LOCALAPPDATA%\Temp`, a
un directorio dentro del repo. Funcionó unas cuantas corridas y lo di por resuelto.

**Volvió, dentro del repo, tres corridas seguidas, en dos formas distintas:**

```
fork/exec .../.gotmp/go-build.../b001/db.test.exe:
    An Application Control policy has blocked this file.
go: unlinkat .../.gotmp/go-build.../b001/db.test.exe:
    The process cannot access the file because it is being used by another process.
```

**La causa real: directorios de build viejos que se acumulan en el scratch.** Una vez que uno
guarda un binario bloqueado o tomado por otro proceso, todas las corridas siguientes lo
heredan. Borrar `.gotmp` y reintentar lo curó al instante.

El arreglo verdadero es **vaciar el scratch antes de cada corrida**, no reubicarlo:

```make
GOTMPDIR_API := $(CURDIR)/apps/api/.gotmp

test-api:
	@rm -rf $(GOTMPDIR_API) && mkdir -p $(GOTMPDIR_API)
	cd apps/api && GOTMPDIR=$(GOTMPDIR_API) go test ./...
```

Vaciar no cuesta nada: la caché de build real de Go es `GOCACHE`, que queda intacta, así que
no fuerza ninguna recompilación. Validado en 4 corridas cacheadas y 3 con link forzado:
**0 incidentes**.

La tentación obvia es un `|| true` o un retry en el Makefile. **No.** Eso también se tragaría
los fallos de verdad, y una puerta de calidad que a veces miente deja de ser una puerta.

## Lo que casi me como

Cuando el spike de `T-01-002` tiró ese `FAIL`, la lectura fácil era "el spike falló, hay que
pasar al fallback de compose" — que es exactamente lo que el proposal tenía escrito como plan
B. Habríamos abandonado testcontainers, que **funciona perfecto**, por una política de
seguridad del sistema operativo. Leer el mensaje entero antes de creerle al `FAIL` valió la
fase entera.

Relacionado: [[2026-08-29-go-get-no-resuelve-subpaquetes]].
