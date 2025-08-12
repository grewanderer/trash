package configsvc

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"text/template"
	"wisp/internal/models"
)

type Renderer struct{}

func NewRenderer() *Renderer { return &Renderer{} }

func (r *Renderer) RenderAll(_ string, _ map[string]string) (map[string]string, error) {
	return nil, fmt.Errorf("RenderAll not used; builder calls RenderOne sequentially")
}

func (r *Renderer) RenderOne(t models.Template, vars map[string]string) (string, error) {
	switch t.Type {
	case "", "go":
		return renderGoTemplate(t.Body, map[string]any{"vars": vars})
	case "netjson":
		// 1) подставим vars в JSON-шаблон как в go-template (если нужно)
		jsonWithVars, err := renderGoTemplate(t.Body, map[string]any{"vars": vars})
		if err != nil {
			return "", fmt.Errorf("pre-template: %w", err)
		}
		// 2) прогон через netjsonconfig
		uci, err := renderViaNetjsonconfig(jsonWithVars)
		if err != nil {
			return "", err
		}
		return uci, nil
	default:
		return "", fmt.Errorf("unknown template type: %s", t.Type)
	}
}

func renderGoTemplate(body string, data any) (string, error) {
	tpl, err := template.New("tpl").
		Option("missingkey=error").
		Funcs(template.FuncMap{
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

func renderViaNetjsonconfig(netjson string) (string, error) {
	bin := os.Getenv("NETJSONCONFIG_BIN")
	if bin == "" {
		bin = "netjsonconfig"
	}

	cmd := exec.Command(bin, "-b", "openwrt", "-o", "uci")
	cmd.Stdin = bytes.NewBufferString(netjson)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("netjsonconfig error: %v: %s", err, stderr.String())
	}
	return out.String(), nil
}
