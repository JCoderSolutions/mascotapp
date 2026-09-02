---
type: domain
score: 5
topic_key: mascotapp/domain/historical-readability-from-the-renderer
approved: 2026-09-01 por el usuario (T-01-035, cierre de fase)
observation_id: obs-069b75a9882b4f07
task: T-01-024
rationale: "El requisito es sobre una RESPUESTA REGISTRADA, no sobre la tabla de versiones. Afirmarlo desde el lado de versiones lo implica; afirmarlo desde el camino del renderer lo muestra."
---

# La legibilidad historica se afirma desde el camino del RENDERER

§4.4 regla 1 dice que una respuesta enviada siempre se renderiza contra la version con la que
se lleno. Hay dos formas de "probarlo" y solo una lo prueba.

**Lo que NO alcanza:** que v1 conserve `has_other_pets` despues de que v2 lo tire. Eso es
necesario y es una propiedad de la tabla de **versiones**. El requisito es sobre una **RESPUESTA
REGISTRADA**, y una respuesta es legible solo si el camino que el renderer realmente camina
—**submission → SU PROPIA version → los campos de esa version**— resuelve **cada clave** que el
documento de respuestas lleva.

**Como se afirma:** recorrer la forma de §4.4 (`sections → rows → fields`) y matchear por
**`field.id`**, nunca el documento como texto. Un match textual encuentra el id en una etiqueta,
en una referencia de logica condicional o en un valor, y reporta un campo que no esta.

**Y la anti-vacuidad es la que sostiene todo.** La asercion natural es "cero claves sin
resolver", y esa es exactamente la forma que pasa por accidente: un camino equivocado dentro del
documento devuelve cero filas y **se lee como prueba**. El contrapeso es correr el **mismo**
resolver contra **v2** y exigir que reporte **exactamente** la clave que v2 tiro. Tres de los
cinco mutantes de la ronda mueren en esa unica asercion — el que hace que v2 conserve el campo,
el que saca la respuesta del documento, y el que resuelve contra la version mas nueva en vez de
contra la registrada.

**El ultimo es el bug que el requisito existe para prevenir**, y es el mas facil de escribir sin
darse cuenta: `JOIN form_template_versions ON template_id = ... ORDER BY version DESC LIMIT 1`
en vez de `ON id = s.template_version_id`. Renderiza preguntas que el adoptante nunca vio.

Y todo esto es legible solo mientras la version **siga existiendo**: la durabilidad la dan el
trigger de inmutabilidad de 00007 (para las publicadas) y `ON DELETE RESTRICT` (para un draft
que junto respuestas). Ver [[una-capa-que-contesta-primero-vacia-el-test-de-abajo]] — de esas
dos capas, solo una se estaba ejercitando.
