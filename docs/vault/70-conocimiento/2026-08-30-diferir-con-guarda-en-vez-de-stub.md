---
type: convention
score: 4
topic_key: mascotapp/convention/defer-with-a-guard-not-a-stub
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-0baa00784ecb7128
task: T-01-012
rationale: "El stub que desbloquea un target es la deuda técnica que nadie borra, porque para cuando estorba ya nadie recuerda que era temporal."
---

# Cuando una herramienta todavía no tiene de qué agarrarse: diferir con guarda, no poner un stub

`sqlc generate` **falla** con un directorio de queries vacío — verificado contra
sqlc v1.31.1: `error parsing queries: no queries contained in paths`. Y la
primera query necesita la primera tabla, que llegaba 21 tareas después.

Las dos salidas obvias son malas:

1. **Escribir una query placeholder** (`SELECT 1 AS ok`) para que el target
   quede en verde. Funciona, y es **código que miente sobre por qué existe**.
   Nadie lo va a borrar: dentro de dos meses parece una query de healthcheck
   deliberada.
2. **Cablearlo igual y que rompa.** Rompe el build de todos hasta que llegue la
   primera tabla.

## La tercera salida

**Diferir el cableado, y escribir el test que hace la deferencia imposible de
olvidar.**

```
query dir vacío  ⟺  sqlc ausente de make generate, de CI y del devcontainer
```

El test afirma la equivalencia en **las dos direcciones**. Hoy pasa con las dos
mitades vacías. El día que aparece el primer `.sql`, se pone rojo y nombra
exactamente qué falta cablear.

O sea: **el RED de la tarea siguiente se construye en la tarea actual.** No es
un TODO, no es un comentario, no es una línea comentada — es un test que falla.

## La forma general

Es la misma idea que el ledger `Pending` de [[2026-08-30-un-suite-verde-sobre-cero-tablas]],
aplicada a build wiring en vez de a schema. Las dos responden a la misma
pregunta: *¿cómo dejo constancia de algo que todavía no se puede hacer, sin que
la constancia sea un comentario que nadie lee?*

La respuesta es siempre la misma: **una assertion que hoy pasa y mañana falla
sola.**
