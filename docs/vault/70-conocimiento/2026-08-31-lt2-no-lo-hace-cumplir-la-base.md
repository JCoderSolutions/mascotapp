---
type: domain
score: 5
topic_key: mascotapp/domain/shelter-verification
approved: 2026-08-31 por el usuario
observation_id: obs-85415c0b8700bec6
task: T-01-022
rationale: "Un requisito duro de MVP que nadie implemento, con una suite verde que fijaba lo contrario. Sin esto se lanza creyendo que la verificacion anda."
---

# LT-2 no lo hace cumplir la base, y la suite venia fijando lo contrario

**§1.1 del plan pone `pending_verification` como requisito duro de MVP:** *"un refugio no puede
publicar hasta ser verificado manualmente"*. Es la mitigacion de LT-2, cuyo riesgo declarado es
que la plataforma se vuelva vehiculo de estafa.

Estado real al cerrar T-01-022:

- `shelters.status` existe y su default es `pending_verification`. **Nadie la lee.**
- `public_catalog` sobre `pets` filtra por `status`, `published_at` y `deleted_at`, y por nada
  mas.
- Por lo tanto **un refugio sin verificar publica y su animal sale en el catalogo publico.**

Y lo que lo hacia invisible: **cada test de catalogo del paquete publica desde un refugio sin
verificar y afirma que el pet SI se ve.** El verde no decia "la verificacion anda"; decia "el
catalogo muestra lo publicado", y nadie habia notado que esas dos frases se habian separado.

## Por que no se arregla con un test

La condicion necesita `EXISTS (SELECT 1 FROM shelters s WHERE s.id = pets.shelter_id AND
s.status = 'verified')`. Y el subquery de una politica **exige que el rol tenga `SELECT` sobre
la tabla referenciada** (T-01-020, mutante M13). `app_public` no tiene ni grant ni politica
sobre `shelters`.

Asi que agregar la condicion sola **no angosta el catalogo: lo rompe**, con un error de
permisos en toda lectura publica. Verificado con un mutante que se lleva puestos diez casos.

**Las dos mitades solo pueden aterrizar juntas**, y eso es una migracion.

## La leccion que sobrevive a este bug puntual

**Un requisito de producto que ninguna capa lee es indistinguible de uno que no existe, y una
suite verde puede estar fijando activamente su ausencia.** El default de la columna daba una
falsa sensacion de cobertura: estaba bien elegido y no lo consultaba nadie.

Mientras tanto queda `TestPublicCatalog_DoesNotYetEnforceShelterVerification` como test de
**caracterizacion**: deja el estado actual por escrito, falla el dia que la condicion aterrice
con instrucciones para invertirlo, y afirma que las dos mitades no pueden aterrizar por
separado.

Relacionado: [[2026-08-31-un-exists-de-politica-tambien-exige-el-grant]].
