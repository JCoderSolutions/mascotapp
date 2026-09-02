# ADR-0010 — Tablas append-only, y por qué hacen falta las cuatro capas

- **Fecha:** 2026-09-01
- **Estado:** aceptado
- **Fase:** 01

> **Nota de alcance.** El tablero de la Fase 01 describía esto como *"las tres capas"*. Son
> **cuatro**. La cuarta —el trigger de `TRUNCATE`— se agregó cuando se comprobó que ninguna de las
> otras tres ve un truncate, y no es una variante de la tercera: un trigger `FOR EACH ROW` no
> dispara sobre `TRUNCATE` porque `TRUNCATE` no visita ninguna fila.

## Contexto

Tres tablas del esquema son **el registro de lo que pasó**: `pet_status_history` (LT-5),
`application_events` y `audit_log`. Un registro de lo que pasó **que se puede reescribir no es un
registro de lo que pasó**.

"Append-only" suena a una sola decisión y no lo es, porque **cada mecanismo disponible para
imponerlo detiene a un actor distinto** — y ninguno detiene a todos:

- un `REVOKE UPDATE` no le hace nada a un rol con `BYPASSRLS`;
- una política no la ve un rol con `BYPASSRLS` **ni el owner** si falta `FORCE`;
- ningún mecanismo a nivel de fila ve un `TRUNCATE`, porque no visita filas;
- y **cero filas afectadas** —lo que devuelve la ausencia de una política— es un resultado que el
  código que llama lee como éxito.

En Neon el rol con `BYPASSRLS` no es hipotético: `neon_superuser` lo tiene.

## Decisión

**Una tabla append-only lleva CUATRO capas, y cada una existe porque detiene a un actor que las
otras no.**

| Capa | Detiene a | Cómo |
|---|---|---|
| 1 — políticas por comando | el rol de aplicación ordinario | `FOR SELECT` y `FOR INSERT`, y **la AUSENCIA** de una `FOR UPDATE`/`FOR DELETE`. Bajo `FORCE`, un comando sin política permisiva alcanza cero filas, **también para el owner**. |
| 2 — grants revocados | el mismo rol, antes de llegar a la política | `REVOKE UPDATE, DELETE, TRUNCATE` |
| 3 — trigger `BEFORE UPDATE OR DELETE` de fila | un rol con `BYPASSRLS` | RLS se saltea; **los triggers no** |
| 4 — trigger `BEFORE TRUNCATE` de sentencia | a todos, sobre la única escritura que ninguna capa de fila ve | `FOR EACH STATEMENT` |

Y **una política `FOR ALL` está prohibida en estas tablas**: al partirla en dos por comando, la
inferencia de `WITH CHECK` a partir de `USING` desaparece, así que cada cláusula se escribe
explícita.

## Alternativas descartadas

| Alternativa | Por qué no |
|---|---|
| Solo revocar los grants | No sobrevive a `BYPASSRLS`, ni a que alguien restaure el grant. |
| Solo el trigger | Correcto pero solitario: cualquier error en la función de trigger abre la tabla entera, y no hay nada abajo. |
| Confiar en que la capa de aplicación no escriba `UPDATE` | Es exactamente lo que [[ADR-0002]] existe para no hacer. |
| Un solo trigger `BEFORE UPDATE OR DELETE OR TRUNCATE` | **No compila como uno solo**: `TRUNCATE` exige `FOR EACH STATEMENT` y los otros dos `FOR EACH ROW`. Son dos triggers por obligación, no por gusto. |

## Consecuencias

**A favor**

- La inmutabilidad sobrevive a un rol mal aprovisionado, que es el modo de falla que ninguna
  revisión de código detecta.
- **El trigger convierte un cero-filas silencioso en un error ruidoso.** El `RAISE` de plpgsql da
  `P0001`, distinto **a propósito** del `42501` del grant, para que un test pueda decir **cuál
  capa** contestó.

**En contra, y se acepta**

- Cuatro mecanismos por tabla es repetición, y hay que escribirla entera cada vez.
- **Una fila mal escrita no se corrige: se corrige APPENDEANDO** un evento que la contradiga. Es
  la propiedad, no un efecto lateral, y hay que decírselo al producto.
- **Append-only cuyo padre cascadea es TEATRO.** `app_tenant` tiene `DELETE` sobre `pets` y sobre
  `adoption_applications`: con un `ON DELETE CASCADE`, el refugio borra el rastro **borrando al
  padre**, sin tocar nunca la tabla protegida ni disparar ningún trigger. Por eso toda referencia
  desde una tabla append-only hacia su padre va con **`ON DELETE RESTRICT`**. El agujero nunca
  está en la columna que estabas mirando.

**Cómo se prueba que las cuatro capas son cuatro, y no una más tres adornos:** una ronda de
mutación saca **exactamente una** capa por mutante, y se mira **cuál test murió**. Si dos capas
mueren en el mismo test y en ningún otro, hay una sola propiedad afirmada. En `application_events`
y en `audit_log` las cuatro murieron en tests distintos.

**Y la capa 1 no se puede ejercitar**, porque su enforcement es una **ausencia**: los triggers
contestan primero, así que ninguna sonda de comportamiento distingue una política `UPDATE`
ausente de una presente. El catálogo es el único lugar donde vive esa respuesta, y ahí se afirma.

**La suite es enumerada, no escrita a mano.** El conjunto append-only está declarado
(`Schema.AppendOnly`) y la suite lo recorre. Agregar una cuarta tabla append-only es agregar su
nombre y su fixture; nada más.

## Condiciones de reapertura

1. Que una tabla append-only necesite **borrado por retención** (§5.4). Hoy la purga es
   minimización de datos sobre filas que se quedan, no borrado — y eso está abierto como decisión
   de producto.
2. Que PostgreSQL agregue una forma de política que vea `TRUNCATE`. Eliminaría la capa 4.
3. Que el proveedor deje de exponer un rol con `BYPASSRLS`. Eliminaría el argumento de la capa 3
   — **y solo para ese proveedor**, que es un motivo pobre para sacar una capa.
