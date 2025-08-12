// internal/configsvc/renderer.go
package configsvc

import (
	"bytes"
	"fmt"
	"text/template"
	"wisp/internal/models"
)

// Убедитесь, что этот тип уже есть у вас. Если нет — добавьте простой каркас:
type Renderer struct {
	// здесь могут быть зависимости (repo, функции и т.п.)
}

// NewRenderer — конструктор по месту инициализации.
func NewRenderer() *Renderer { return &Renderer{} }

// RenderAll — если в проекте уже реализован, оставьте как есть.
// В противном случае пока можно вернуть ошибку, если нигде не используется.
// Лучше, конечно, чтобы RenderAll пользовался вашим репозиторием назначений.
func (r *Renderer) RenderAll(_ string, _ map[string]string) (map[string]string, error) {
	return nil, fmt.Errorf("RenderAll is not implemented in this renderer; use Builder.collectTemplates + RenderOne")
}

// RenderOne — реализация, которой не хватало.
// Поддерживаем типы шаблонов: "go" (по умолчанию).
func (r *Renderer) RenderOne(t models.Template, vars map[string]string) (string, error) {
	switch t.Type {
	case "", "go":
		return renderGoTemplate(t.Body, map[string]any{"vars": vars})
	// case "netjson":
	//     // сюда позже можно подключить адаптер netjsonconfig
	//     return "", fmt.Errorf("netjson templates are not supported yet")
	default:
		return "", fmt.Errorf("unknown template type: %s", t.Type)
	}
}

func renderGoTemplate(body string, data any) (string, error) {
	tpl, err := template.New("tpl").
		Option("missingkey=error").
		Funcs(template.FuncMap{
			// удобные функции, если нужно:
			"get": func(m map[string]string, k string) string { return m[k] },
			"has": func(m map[string]string, k string) bool { _, ok := m[k]; return ok },
		}).
		Parse(body)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
