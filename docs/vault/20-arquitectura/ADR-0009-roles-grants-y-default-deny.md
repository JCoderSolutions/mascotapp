# ADR-0009 — Roles y grants por migración, contraseñas fuera de ellas, y default-deny

- **Fecha:** 2026-09-01
- **Estado:** aceptado
- **Fase:** 01

## Contexto

[[ADR-0002]] fija dos roles de aplicación —`app_tenant` y `app_public`— y dice que la app **nunca**
se conecta como owner. Falta decidir tres cosas que parecen de plomería y no lo son: quién crea
esos roles, cómo llegan sus contraseñas, y qué privilegios tienen sobre una tabla **nueva**.

Tres hechos que decidieron la forma:

1. **RLS no aplica a roles con `BYPASSRLS`,** y en Neon eso no es hipotético: `neon_superuser` lo
   tiene. Un rol mal aprovisionado anula la capa 3 entera.
2. **`ALTER ROLE ... PASSWORD` es una sentencia de utilidad y no acepta parámetros de vínculo** —
   la misma trampa que `SET LOCAL` en [[ADR-0008]]. Ponerla en una migración obliga a escribir la
   contraseña en un archivo versionado.
3. **Un privilegio que nunca se otorgó y uno que se revocó se ven iguales desde el catálogo.** La
   diferencia está en si alguien lo pensó.

## Decisión

**Los roles y los grants los crea una migración. Las contraseñas no.**

**Roles** — `00001` crea `app_tenant` y `app_public` con `NOBYPASSRLS` explícito, y un test
afirma que el rol con el que la aplicación se conecta **no puede** saltear RLS. La afirmación no
es sobre lo que la migración escribió: es sobre lo que el catálogo dice del rol que efectivamente
conecta.

**Contraseñas** — nunca en una migración. El bootstrap corre fuera de goose, como owner, y el
valor llega como parámetro de vínculo:

```sql
SELECT set_config('app.bootstrap_password', $1, true);
DO $$ BEGIN EXECUTE format('ALTER ROLE app_tenant PASSWORD %L',
                           current_setting('app.bootstrap_password')); END $$;
```

`format('%L')` hace el quoting del lado del servidor. Un test afirma que **ninguna migración
contiene un literal de contraseña**.

**Grants** — **por tabla, siempre. Nunca `ON ALL TABLES`, nunca `ALTER DEFAULT PRIVILEGES`.** Una
tabla nueva es **inalcanzable** para `app_tenant` hasta que una migración la otorgue a propósito.

Y cada migración escribe su `REVOKE` **antes** del `GRANT`, incluido `FROM PUBLIC`: un privilegio
que nunca se otorgó igual se lee deliberado, y `PUBLIC` es el destinatario por defecto que la
gente olvida.

## Alternativas descartadas

| Alternativa | Por qué no |
|---|---|
| `GRANT ... ON ALL TABLES IN SCHEMA public` | Una tabla nueva queda otorgada **sola**, antes de que nadie decida su política. El orden entre la migración que crea la tabla y la que otorga deja de importar, y con él la posibilidad de que un test note la omisión. |
| `ALTER DEFAULT PRIVILEGES` | Peor: es invisible. El grant no aparece en ninguna migración; aparece solo en el efecto. |
| Contraseñas en una migración, con un placeholder sustituido en despliegue | Un archivo versionado con forma de contraseña termina teniendo una contraseña. |
| Crear los roles a mano en la consola de Neon | El entorno de test y el de producción divergen desde el día uno, y nada verifica que coincidan. |

## Consecuencias

**A favor**

- **Default-deny de verdad.** Olvidarse de otorgar una tabla nueva la deja inútil, no expuesta. El
  meta-test del catálogo falla ruidosamente sobre la omisión.
- La cadena de privilegios de cada tabla está **en el diff de la migración que la crea**, que es
  donde se revisa.
- El repositorio no contiene contraseñas, y un test lo afirma en vez de confiarlo.

**En contra, y se acepta**

- **Cada migración tiene que acordarse de sus grants.** Es trabajo repetido y es el punto: el
  trabajo repetido es visible, el default silencioso no.
- **Un grant se puede escribir mal y verse bien.** Por eso el inventario afirma privilegios
  `true` **y** `false` por rol y tabla, con la razón de cada uno escrita al lado.
- **Un `REVOKE` sobre un privilegio que nadie otorgó es un no-op**, y un mutante que lo borre
  **sobrevive** — está probado. Se queda igual: es el guard que dispara el día que el esquema
  adquiera default privileges. Lo que cerró la clase fue una aserción **enumerada** sobre el
  catálogo (ningún rol de aplicación tiene `TRUNCATE` sobre ninguna tabla), no las tres filas
  escritas a mano que había.

**Y un grant que no es sobre una tabla, que casi se olvida:** `bigserial` no es un tipo — es
`bigint` más una **secuencia** más un default que llama `nextval`. Un grant de tabla no dice nada
sobre esa secuencia, así que `audit_log` necesita `GRANT USAGE ON SEQUENCE`. Sin él, el tenant
puede insertar **solo cuando provee el id**: invisible en desarrollo, aparece en la primera
escritura real. Y `USAGE`, no `ALL` — `ALL` incluye `UPDATE`, que es `setval`, con lo que un
tenant puede mover el contador y reordenar su propia historia.

## Condiciones de reapertura

1. Que la Fase 02 necesite un tercer rol (`app_auth` para `refresh_tokens`). Suma un rol, no
   cambia la regla.
2. Que la Fase 10 traiga un panel de superadmin. Necesita su propio rol con su propio conjunto de
   grants, nunca el owner.
3. **Escalada de privilegios DENTRO del tenant (hallazgo B1 de Judgment Day, confirmado en vivo):
   con grants a nivel tabla, `app_tenant` puede ponerse `status = 'verified'`, subirse la cuota y
   ponerse `role = 'owner'` en su propia fila.** El arreglo son grants **a nivel columna**, y qué
   columnas puede escribir un tenant depende de endpoints que la Fase 03 todavía no escribió.
   Diferido por decisión del usuario a Fase 02/03 — y es la reapertura más probable de este ADR.
