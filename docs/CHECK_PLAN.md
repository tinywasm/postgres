# Plan de Migración a Tinywasm Postgres

Este plan detalla los pasos necesarios para renombrar el módulo, actualizar el repositorio remoto y re-organizar los archivos de prueba.

## 1. Cambio de Nombre del Módulo

- Actualizar `go.mod` para reflejar el nuevo path del módulo.
- **De:** `module github.com/cdvelop/postgre`
- **A:** `module github.com/tinywasm/postgres`
- **Actualizar importaciones**: Reemplazar todas las ocurrencias de `"github.com/cdvelop/postgre"` por `"github.com/tinywasm/postgres"` en todo el proyecto (incluyendo tests).

## 2. Re-ubicación de Pruebas

- Mover el archivo de pruebas `postgres_translate_test.go` al directorio `tests/`.
- Cambiar el paquete de `package postgre_test` a `package tests` para mantener consistencia con `adapter_test.go`.
- Esto mantiene la consistencia con el resto de las pruebas ubicadas en ese directorio.

## 3. Actualización de Referencias Internas

- Revisar y actualizar cualquier importación interna en archivos de prueba o código que use el path antiguo (si aplica).

## 4. Configuración del Repositorio Remoto (GitHub)

- Cambiar la URL del remoto `origin` para apuntar a la nueva organización.
- Comando sugerido:
  ```bash
  git remote set-url origin https://github.com/tinywasm/postgres.git
  ```

## Tareas Detalladas

- [ ] Modificar `go.mod` (Línea 1).
- [ ] Ejecutar `go mod tidy` para limpiar dependencias.
- [ ] Mover `postgres_translate_test.go` -> `tests/postgres_translate_test.go`.
- [ ] Verificar que las pruebas sigan pasando: `go test ./...`.
- [ ] Actualizar el remoto de Git.
