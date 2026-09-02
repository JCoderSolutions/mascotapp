# Plantilla de commit — Conventional Commits

Regla del proyecto, obligatoria para humanos y agentes. La plantilla ejecutable
vive en [`.gitmessage`](../../../.gitmessage) en la raíz del repositorio.

---

## Formato

```
<tipo>(<alcance opcional>): <T-FF-NNN> <descripción en imperativo>

<cuerpo opcional: el POR QUÉ, no el qué>

<pie opcional: BREAKING CHANGE, refs>
```

**Ejemplo real de este repositorio:**

```
feat(api): T-02-004 tenant resolution middleware

The shelter_id comes from the JWT claim and never from the URL or a
header, because anything the client controls is not an identity.
```

---

## Reglas

1. **La primera línea es un resumen, no una novela.** Máximo 72 caracteres,
   en **imperativo presente** (`add`, no `added` ni `adds`), **sin punto final**.
2. **El tipo va en minúscula** y siempre lleva `:` después.
3. **El ID de tarea va al principio de la descripción** cuando el commit cierra
   o avanza una tarea del tablero: `feat(auth): T-02-004 …`. Es lo que permite
   ir del historial al tablero y del tablero al historial.
4. **El cuerpo explica el porqué.** El *qué* ya está en el diff. Si el commit no
   tiene un porqué que valga la pena, no necesita cuerpo.
5. **En inglés.** Es la invariante 4 del proyecto: todo artefacto técnico va en
   inglés, y un mensaje de commit es un artefacto técnico. La documentación del
   vault es lo que va en español.
6. **Sin `Co-Authored-By` y sin ninguna atribución a IA.** Regla explícita del
   usuario; vale por encima del default de cualquier herramienta.
7. **Un commit, una unidad de trabajo.** Si el mensaje necesita un "y", casi
   siempre son dos commits.

---

## Tipos

| Tipo | Cuándo |
|---|---|
| `feat` | Funcionalidad nueva visible desde afuera |
| `fix` | Corrección de un bug |
| `docs` | Solo documentación: vault, ADRs, README, specs |
| `test` | Solo tests: agregar, corregir o reforzar |
| `refactor` | Cambio interno sin alterar comportamiento observable |
| `perf` | Cambio cuyo objetivo declarado es rendimiento |
| `build` | Migraciones, generación de código, dependencias, empaquetado |
| `ci` | Workflows y puertas de calidad |
| `chore` | Andamiaje, tooling, configuración — nada de lo anterior |
| `revert` | Revierte un commit previo; nombralo en el cuerpo |

## Alcances de este proyecto

`api` · `web` · `db` · `auth` · `domain` · `media` · `forms` · `catalog` ·
`sdd` · `vault` · `ci` · `deps`

El alcance es **opcional**. Se pone cuando acota de verdad; no se inventa uno
para llenar el paréntesis.

---

## Cambios que rompen compatibilidad

Dos formas, ambas válidas. Usá el pie cuando quieras explicar la migración:

```
feat(api)!: T-05-012 drop pet.age in favour of birth_date_estimate
```

```
feat(api): T-05-012 replace pet.age with birth_date_estimate

BREAKING CHANGE: `age` disappears from the pet payload. Clients compute
it from `birth_date_estimate` and `age_precision`.
```

---

## Activar la plantilla en git

Una sola vez por clon:

```bash
git config commit.template .gitmessage
```

A partir de ahí, `git commit` sin `-m` abre el editor con el andamiaje y las
reglas comentadas al pie.

## Verificación

Hoy la convención se sostiene por disciplina y por esta plantilla. Si en algún
momento se quiere hacer cumplir de verdad, el camino es `commitlint` con
`@commitlint/config-conventional` como puerta bloqueante en
`.github/workflows/ci.yml`. Está anotado como opción, no como decisión tomada.
