package configsvc

import (
	"bytes"
	"fmt"
	"net"
	"sort"
	"text/template"
	"wisp/internal/ipam"
	"wisp/internal/models"
	"wisp/internal/owctrl"
)

type Builder struct {
	repo *Repo
	ipam *ipam.Repo       // опционально
	tpl  TemplateRenderer // опционально, если не задан — будет создан дефолтный рендерер
}

func NewBuilder(repo *Repo) *Builder { return &Builder{repo: repo} }
func NewBuilderWithIPAM(repo *Repo, ipam *ipam.Repo) *Builder {
	// совместимость: если рендерер явно не передали — создадим дефолтный
	return &Builder{
		repo: repo,
		ipam: ipam,
		tpl:  NewTemplateRenderer(repo),
	}
}

// Явный конструктор, если хочешь подменять рендерер
func NewBuilderWithIPAMAndRenderer(repo *Repo, ipam *ipam.Repo, tpl TemplateRenderer) *Builder {
	if tpl == nil {
		tpl = NewTemplateRenderer(repo)
	}
	return &Builder{repo: repo, ipam: ipam, tpl: tpl}
}

func (b *Builder) BuildConfig(d owctrl.DeviceFields) (map[string]string, error) {
	files := map[string]string{}

	// Groups
	grps, err := b.repo.GetDeviceGroups(d.UUID)
	if err != nil {
		return nil, err
	}
	gids := make([]uint, 0, len(grps))
	for _, g := range grps {
		gids = append(gids, g.ID)
	}

	// Vars: group -> device
	groupVars, err := b.repo.GetGroupVars(gids)
	if err != nil {
		return nil, err
	}
	devVars, err := b.repo.GetDeviceVars(d.UUID)
	if err != nil {
		return nil, err
	}
	mergedVars := map[string]string{}
	for k, v := range groupVars {
		mergedVars[k] = v
	}
	for k, v := range devVars {
		mergedVars[k] = v
	}

	// IPAM group prefix vars
	var firstGroupPrefix *models.Prefix
	if b.ipam != nil && len(grps) > 0 {
		if pfx, err := b.ipam.FirstGroupPrefix(grps[0].ID); err == nil {
			firstGroupPrefix = pfx
			mergedVars["ipam_group_prefix_cidr"] = pfx.CIDR
			if _, nw, e := net.ParseCIDR(pfx.CIDR); e == nil {
				ones, _ := nw.Mask.Size()
				mergedVars["ipam_group_prefix_len"] = fmt.Sprintf("%d", ones)
				mergedVars["ipam_group_prefix_network"] = nw.IP.String()
				if gw := firstUsableIPv4(nw); gw != "" {
					mergedVars["ipam_group_prefix_gw"] = gw
				}
				mergedVars["ipam_group_prefix_netmask"] = net.IP(nw.Mask).String()
			}
		}
	}

	// IPAM device IP (если есть в том же префиксе)
	if b.ipam != nil && firstGroupPrefix != nil {
		ips, _ := b.ipam.DeviceIPs(d.UUID)
		for _, rec := range ips {
			if rec.PrefixID == firstGroupPrefix.ID {
				mergedVars["ipv4_address"] = rec.Address
				if _, nw, e := net.ParseCIDR(firstGroupPrefix.CIDR); e == nil {
					mergedVars["ipv4_netmask"] = net.IP(nw.Mask).String()
					if gw := firstUsableIPv4(nw); gw != "" {
						mergedVars["ipv4_gateway"] = gw
					}
				}
				break
			}
		}
	}

	// Templates: group then device (device overrides by path)
	gTpls, err := b.repo.TemplatesByGroupIDs(gids)
	if err != nil {
		return nil, err
	}
	dAs, err := b.repo.ListAssignments(d.UUID)
	if err != nil {
		return nil, err
	}
	dTIDs := make([]uint, 0, len(dAs))
	for _, a := range dAs {
		dTIDs = append(dTIDs, a.TemplateID)
	}
	dTpls, err := b.repo.TemplatesByIDs(dTIDs)
	if err != nil {
		return nil, err
	}

	sort.Slice(gTpls, func(i, j int) bool { return gTpls[i].ID < gTpls[j].ID })
	sort.Slice(dTpls, func(i, j int) bool { return dTpls[i].ID < dTpls[j].ID })

	data := map[string]any{
		"device": map[string]any{
			"uuid":    d.UUID,
			"name":    d.Name,
			"backend": d.Backend,
			"mac":     d.MAC,
		},
		"vars":   mergedVars,
		"groups": grps,
	}

	renderInto := func(tpls []models.Template) error {
		for _, tpl := range tpls {
			content, err := render(tpl.Body, data)
			if err != nil {
				return fmt.Errorf("template %d render: %w", tpl.ID, err)
			}
			files[tpl.Path] = content
		}
		return nil
	}

	if err := renderInto(gTpls); err != nil {
		return nil, err
	}
	if err := renderInto(dTpls); err != nil {
		return nil, err
	}

	if len(files) == 0 {
		files["etc/config/system"] = fmt.Sprintf(
			"config system 'system'\n  option hostname '%s'\n  option timezone 'UTC'\n",
			safe(d.Name),
		)
	}

	files["etc/openwisp/device.meta"] = fmt.Sprintf("uuid=%s\nmac=%s\nbackend=%s\n", d.UUID, d.MAC, d.Backend)
	files["etc/openwisp/managed_by_openwisp_go.md"] = "This device is managed by OpenWISP-Go controller.\n"

	return files, nil
}

func render(body string, data any) (string, error) {
	tpl, err := template.New("cfg").Option("missingkey=error").Parse(body)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func safe(s string) string { return bytes.NewBufferString(s).String() }

// IPv4 helpers
func firstUsableIPv4(nw *net.IPNet) string {
	ip := nw.IP.To4()
	if ip == nil {
		return ""
	}
	u := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
	u++ // network + 1
	return net.IPv4(byte(u>>24), byte(u>>16), byte(u>>8), byte(u)).String()
}
