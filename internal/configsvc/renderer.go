// internal/configsvc/renderer.go
package configsvc

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"wisp/internal/models"
)

// TemplateRenderer — общий интерфейс рендеринга.
type TemplateRenderer interface {
	// Построить финальные файлы для устройства (с учётом порядка).
	RenderAll(deviceUUID string, vars map[string]string) (map[string]string, error)
	// Построить файлы по одному шаблону (может вернуть несколько путей для netjson).
	RenderOneFiles(t models.Template, vars map[string]string) (map[string]string, error)
}

// NewTemplateRenderer возвращает композитный рендерер (go + netjson).
func NewTemplateRenderer(r *Repo) TemplateRenderer {
	return &compositeRenderer{
		repo: r,
		goR:  &goRenderer{},
		njR:  &netjsonRenderer{bin: os.Getenv("NETJSONCONFIG_BIN")},
	}
}

/* ───────────────────────── composite ───────────────────────── */

type compositeRenderer struct {
	repo *Repo
	goR  *goRenderer
	njR  *netjsonRenderer
}

func (c *compositeRenderer) RenderAll(deviceUUID string, vars map[string]string) (map[string]string, error) {
	tpls, err := c.repo.ResolvedTemplatesForDevice(deviceUUID)
	if err != nil {
		return nil, fmt.Errorf("resolve templates: %w", err)
	}
	out := make(map[string]string, len(tpls))
	for _, t := range tpls {
		m, err := c.RenderOneFiles(t, vars)
		if err != nil {
			return nil, fmt.Errorf("template %d (%s): %w", t.ID, t.Name, err)
		}
		for p, v := range m {
			out[p] = v // более поздний перезапишет предыдущий путь
		}
	}
	return out, nil
}

func (c *compositeRenderer) RenderOneFiles(t models.Template, vars map[string]string) (map[string]string, error) {
	tt := strings.ToLower(strings.TrimSpace(t.Type))
	switch tt {
	case "", "go":
		s, err := c.goR.render(t.Body, map[string]any{"vars": vars})
		if err != nil {
			return nil, err
		}
		path := strings.TrimLeft(strings.TrimSpace(t.Path), "/")
		if path == "" {
			return nil, fmt.Errorf("go-template %d has empty path", t.ID)
		}
		return map[string]string{path: s}, nil

	case "netjson":
		return c.njR.render(t, vars)

	default:
		return nil, fmt.Errorf("unknown template type: %s", t.Type)
	}
}

/* ───────────────────────── go renderer ───────────────────────── */

type goRenderer struct{}

func (g *goRenderer) render(body string, data any) (string, error) {
	tpl, err := template.New("tpl").
		Option("missingkey=zero").
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

/* ───────────────────────── netjson renderer (минимально) ─────────────────────────
   В проде лучше вызывать netjsonconfig и распаковывать вывод в набор файлов.
   Здесь: 1) подставим vars в тело как go-template; 2) если бинарь недоступен — вернём один файл по t.Path.
*/

type netjsonRenderer struct{ bin string }

func (n *netjsonRenderer) render(t models.Template, vars map[string]string) (map[string]string, error) {
	// сначала подставим vars как go-template (часто NetJSON содержит плейсхолдеры {{ .vars.* }})
	gr := &goRenderer{}
	netjsonBody, err := gr.render(t.Body, map[string]any{"vars": vars})
	if err != nil {
		return nil, fmt.Errorf("netjson preprocess: %w", err)
	}

	// если netjsonconfig не настроен — отдадим один файл в t.Path (достаточно для system/простых кейсов)
	bin := n.bin
	if bin == "" {
		bin = "netjsonconfig"
	}
	if _, err := exec.LookPath(bin); err != nil || strings.TrimSpace(t.Path) != "" {
		path := strings.TrimLeft(strings.TrimSpace(t.Path), "/")
		if path == "" {
			return nil, fmt.Errorf("netjson: no binary and empty path; set template.path or install netjsonconfig")
		}
		return map[string]string{path: netjsonBody}, nil
	}

	// TODO: полный пайплайн через netjsonconfig (опционально)
	// Пример идеи:
	//   cmd := exec.Command(bin, "--backend", "openwrt", "--method", "generate")
	//   cmd.Stdin = strings.NewReader(netjsonBody)
	//   out, err := cmd.Output()
	//   // out -> распарсить/распаковать в map[path]content
	// Пока опустим.

	return nil, fmt.Errorf("netjson renderer is not fully configured")
}
