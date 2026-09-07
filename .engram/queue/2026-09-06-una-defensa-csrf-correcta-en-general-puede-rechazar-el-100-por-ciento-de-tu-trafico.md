---
type: architecture
score: 5
topic_key: mascotapp/security/origin-is-the-control-not-sec-fetch-site
task: T-02-025
status: pendiente-de-aprobacion
rationale: "Este es el error que MENOS se detecta en revisión, porque la regla incorrecta suena mejor que la correcta. 'Rechazá cross-site' es lo que cualquiera esperaría leer en una defensa CSRF, y en esta arquitectura rechaza el 100% del tráfico legítimo. Va a volver a aparecer cada vez que alguien toque csrf.go, agregue un endpoint que cambie estado, o revise este código sin recordar que los orígenes están separados por decisión. Y aplica más allá de este proyecto: toda regla de seguridad escrita sobre 'lo que hace un atacante' hay que verificarla contra lo que hace el tráfico normal PRIMERO."
---

# Una defensa CSRF correcta en general puede rechazar el 100% de tu tráfico

(T-02-025, MascotApp Fase 02.)

## La regla que suena bien

`Sec-Fetch-Site` describe de dónde vino un request. Un formulario posteado desde otro sitio llega
como `cross-site`. Entonces la defensa CSRF se escribe sola:

```go
if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
    forbidden(w)
    return
}
```

Es exactamente lo que uno espera leer en un middleware CSRF. Un revisor asiente y sigue.

## Por qué acá rechaza todo

**Los orígenes separados están decididos** (web en un origen, API en otro). Entonces **todos** los
requests legítimos del navegador desde la web app son cross-site. Esa regla los rechaza a los
**cien por cien**.

El bug no se ve en revisión porque la regla equivocada **suena mejor que la correcta**. Y no se ve
en un test de "el atacante es rechazado", porque el atacante **sí** es rechazado. Se ve solamente
en un test que afirme que el usuario legítimo **pasa**.

## Lo que sí es correcto

| Cabecera | Rol |
|---|---|
| `Origin` | **el control**. El navegador la pone en cross-site y JavaScript no la puede forjar (*forbidden header name*) |
| `Sec-Fetch-Site` | **solo el respaldo** para un request que legítimamente no trae `Origin` |

El orden, y es exhaustivo a propósito:

1. método seguro → pasa;
2. **`Origin` presente → lo contesta la allowlist, y NADIE más**;
3. sin `Origin`, `Sec-Fetch-Site: same-origin` → pasa;
4. cualquier otra cosa → rechazo (fail closed).

### El paso 2 no puede caerse al 3

Si un `Origin` rechazado se cayera a mirar `Sec-Fetch-Site`, un cliente que **no es navegador**
—que puede poner esa cabecera en lo que quiera, porque solo los navegadores están atados a la
regla de *forbidden header names*— **rescataría un origen que la allowlist acaba de rechazar**.

Encadenar chequeos como alternativas ("probá uno, si falla probá el otro") convierte dos controles
en el más débil de los dos.

## La lección general

> **Toda regla de seguridad escrita sobre "lo que hace un atacante" hay que verificarla contra lo
> que hace el TRÁFICO NORMAL, primero.**
>
> Una regla que rechaza correctamente al atacante y también a todos los usuarios no es una regla
> estricta: es una caída. Y su test de seguridad pasa igual.

Corolario de testing, que es lo que lo agarró acá: **el test que vale es el que afirma que el
camino legítimo funciona bajo las condiciones que más se parecen a un ataque.** El mutante que
implementa "rechazá cross-site" lo mata **un solo test** —el que dice *"un origen permitido pasa
aunque `Sec-Fetch-Site` diga cross-site"*— y los otros trece del archivo pasan tranquilos.

## Dónde más aplica en este proyecto

- cualquier endpoint nuevo que cambie estado hereda esto sin tocarlo, porque el middleware es uno
- el rol `app_public` del catálogo: las reglas se escriben sobre qué NO puede leer, y hace falta la
  contraparte de que el catálogo legítimo SÍ se lee
- el rate limiting por cuenta (P2-D10): la regla se escribe sobre el atacante que enumera, y la
  contraparte es que un usuario que se equivoca dos veces de contraseña no queda afuera
