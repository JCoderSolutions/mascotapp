---
type: convention
score: 4
topic_key: mascotapp/convention/mutation-runs-need-a-positive-signal
task: T-02-024
status: pendiente-de-aprobacion
relacionado: obs-9e00e3a6413dc7d2
rationale: "La mutación es la herramienta que más veces me corrigió a mí en esta fase — cuatro veces en tres días. Su valor entero descansa en que un mutante que sobrevive significa algo. Un mutante que no compila produce exactamente la misma salida que un filtro mal escrito, y si lo contás como sobreviviente sacás la conclusión opuesta a la verdadera: 'el test no cubre esto' cuando en realidad el código nunca corrió. Estuve a un renglón de cometerlo. La regla es de tres palabras y evita invalidar toda una campaña de mutación."
---

# Un mutante que no compila no es un mutante que sobrevive

(T-02-024, MascotApp Fase 02. Relacionado con `obs-9e00e3a6413dc7d2`.)

## Lo que pasó

Mutante seis de siete: hacer que el middleware acepte **cualquier** esquema de `Authorization`,
no solo `Bearer`.

```
### M5 any auth scheme
###   (nada)
```

Mi filtro era `rg "^--- FAIL|^ok "`. No matcheó nada. La lectura refleja es **"el mutante
sobrevivió"** — o sea, *"ningún test cubre el esquema de autenticación"*.

Era falso. El mutante dejaba `scheme` declarada y sin usar:

```
internal/httpapi/middleware_auth.go:181:2: declared and not used: scheme
FAIL [build failed]
```

**El código mutado nunca corrió.** Reescrito con `_` en vez de `scheme`, corrió y murió en el
subtest `wrong_scheme`.

## Por qué es peligroso y no solo molesto

Un mutante que no compila y un test que no cubre nada producen **la misma salida** bajo un filtro
que solo busca `FAIL` y `ok`. Y las conclusiones son opuestas:

| Salida | Lo que parece | Lo que puede ser |
|---|---|---|
| ni `FAIL` ni `ok` | el test no cubre esa rama | el mutante nunca compiló |

Si lo contás mal, la reacción es **escribir un test de más** para cubrir algo que ya estaba
cubierto — y peor, quedarte con la creencia de que la suite tiene un agujero donde no lo hay.

## La regla

> **Una corrida de mutación necesita una señal POSITIVA, no la ausencia de una negativa.**
>
> Un mutante murió si viste `FAIL`. Sobrevivió si viste `ok`. **Cualquier otra cosa —silencio
> incluido— es una corrida que no te contó nada**, y hay que ir a leer la salida cruda antes de
> anotar un resultado.

Corolario operativo: cuando un mutante no imprime **nada**, sospechá de la compilación primero.
En Go, las mutaciones que borran el uso de una variable o de un import son las que más lo
provocan, y son de las más comunes al mutar guardas.

## Es el mismo error, con otra ropa

Ya está guardado en Engram que *un pipe se come el código de salida*, y que `ok <paquete>` no
prueba que un test **nombrado** corrió. Esto es la tercera cara del mismo poliedro:

> Un filtro sobre la salida solo ve lo que sabe buscar. Todo lo que no matchea se ve igual que
> "no pasó nada", y "no pasó nada" nunca es un resultado.

Lo que salva en los tres casos es lo mismo: **capturar la salida cruda a un archivo y leerla**,
en vez de confiar en el grep que escribiste antes de saber qué iba a salir.
