---
type: constraint
score: 3
topic_key: mascotapp/convention/go-get-subpackages
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-79c957e4c6787855
task: T-01-002 (spike testcontainers)
rationale: "Rompe la premisa de 'batchear las dependencias en un solo go get para un solo prompt de permiso', que es una convención deliberada de este proyecto."
---

**`go get` sobre la raíz de un módulo NO resuelve las dependencias de sus subpaquetes.**

`T-01-001` batcheó las cinco dependencias en un solo comando, a propósito: `go get` está
detrás de una regla `ask`, así que el usuario recibe **un** prompt en vez de ocho.

```bash
go get github.com/jackc/pgx/v5 github.com/pressly/goose/v3 \
       github.com/testcontainers/testcontainers-go \
       github.com/testcontainers/testcontainers-go/modules/postgres \
       github.com/google/uuid
```

Verde. `go build ./...` limpio. Pero el primer archivo que importó `pgx/v5/pgxpool` explotó:

```
missing go.sum entry for module providing package github.com/jackc/puddle/v2
  (imported by github.com/jackc/pgx/v5/pgxpool)
```

`puddle/v2` lo necesita **`pgxpool`**, que es un subpaquete. `go get github.com/jackc/pgx/v5`
resuelve lo que necesita el paquete raíz, no lo que necesita cada subpaquete del módulo.

## La regla

Cuando batchees dependencias por adelantado, **listá los subpaquetes que vas a importar de
verdad**, no los módulos:

```bash
go get github.com/jackc/pgx/v5/pgxpool   # no github.com/jackc/pgx/v5
```

Consecuencia práctica para este proyecto: la premisa de "un solo prompt" solo se cumple si
sabés de antemano qué subpaquetes vas a usar. Acá costó un segundo prompt. Si el batch
hubiera nombrado `pgx/v5/pgxpool` desde el principio, habría sido uno solo.

Ojo con el otro lado del mismo tema: estas dependencias entran como `// indirect` hasta que
un archivo las importa, y `go mod tidy` **las borra todas** mientras nadie las use.
Verificado con `go mod tidy -diff`.
