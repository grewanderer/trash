// internal/owctrl/renderer_iface.go
package owctrl

import "wisp/internal/models"

// TemplateRenderer — общий контракт рендера шаблонов.
type TemplateRenderer interface {
	// Универсальный многофайловый рендер одного шаблона
	RenderOneFiles(t models.Template, vars map[string]string) (map[string]string, error)

	// Для простых (go) шаблонов — вернуть 1 файл (контент), путь берётся из t.Path
	RenderOne(t models.Template, vars map[string]string) (string, error)

	// (опц.) если где-то используется сборка «всё сразу»
	RenderAll(deviceUUID string, vars map[string]string) (map[string]string, error)
}
