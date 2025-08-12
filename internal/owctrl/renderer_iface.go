// internal/owctrl/renderer_iface.go
package owctrl

import "wisp/internal/models"

// TemplateRenderer — общий контракт рендера шаблонов.
type TemplateRenderer interface {
	// RenderAll — вернуть сразу набор файлов (path->body) для устройства (если используется).
	RenderAll(deviceUUID string, vars map[string]string) (map[string]string, error)

	// RenderOne — отрендерить один шаблон и вернуть готовый текст для его path.
	// (Если в будущем появятся «многофайловые» шаблоны — добавим RenderOneFiles).
	RenderOne(t models.Template, vars map[string]string) (string, error)
}
