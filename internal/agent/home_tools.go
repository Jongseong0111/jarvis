package agent

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/genai"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/internal/notion"
)

type homeTools struct {
	port HomePort
}

// HomeTools 는 집정리 도구 목록을 만든다. homeURL 이 있으면 노션 링크 도구도 포함한다.
func HomeTools(port HomePort, homeURL string) []Tool {
	h := homeTools{port: port}
	tools := []Tool{
		h.listZones(),
		h.listLocations(),
		h.listItems(),
		h.searchItem(),
		h.listCategories(),
		h.addLocation(),
		h.addItem(),
		h.addItems(),
		h.updateItem(),
		h.deleteItem(),
		h.deleteLocation(),
	}
	if homeURL != "" {
		tools = append(tools, linkTool(homeURL))
	}
	return tools
}

// linkTool 은 집 정리 노션 페이지 링크를 보여주는 읽기 도구다.
func linkTool(url string) Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "show_notion",
			Description: "집 정리 노션 페이지 링크를 보여준다. 사용자가 노션 페이지를 보고 싶어하거나 링크를 달라고 할 때 사용.",
			Parameters:  objSchema(map[string]*genai.Schema{}),
		},
		Run: func(_ context.Context, _ map[string]any) (string, error) {
			return "집 정리 노션 페이지: " + url, nil
		},
	}
}

// --- 읽기 도구 ---

func (h homeTools) listZones() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "list_zones",
			Description: "집의 구역(큰 그룹: 거실, 안방, 베란다 등) 목록을 조회한다.",
			Parameters:  objSchema(map[string]*genai.Schema{}),
		},
		Run: func(ctx context.Context, _ map[string]any) (string, error) {
			locs, err := h.port.Locations(ctx)
			if err != nil {
				return "", err
			}
			seen := map[string]bool{}
			var zones []string
			for _, l := range locs {
				if l.Zone != "" && !seen[l.Zone] {
					seen[l.Zone] = true
					zones = append(zones, l.Zone)
				}
			}
			if len(zones) == 0 {
				return "등록된 구역이 없어.", nil
			}
			sort.Strings(zones)
			return "구역: " + strings.Join(zones, ", "), nil
		},
	}
}

func (h homeTools) listLocations() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "list_locations",
			Description: "장소(물건 두는 자리: 트롤리, 수납장 등) 목록을 조회한다. zone 을 주면 그 구역만.",
			Parameters: objSchema(map[string]*genai.Schema{
				"zone": strSchema("구역으로 필터(선택). 예: 거실"),
			}),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			locs, err := h.port.Locations(ctx)
			if err != nil {
				return "", err
			}
			zone := strArg(args, "zone")
			byZone := map[string][]string{}
			for _, l := range locs {
				if zone != "" && !strings.EqualFold(l.Zone, zone) {
					continue
				}
				byZone[l.Zone] = append(byZone[l.Zone], l.Name)
			}
			if len(byZone) == 0 {
				return "해당하는 장소가 없어.", nil
			}
			return formatByZone(byZone), nil
		},
	}
}

func (h homeTools) listItems() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "list_items",
			Description: "등록된 물건 목록을 조회한다. zone(구역) 또는 location(장소 이름)으로 필터할 수 있다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"zone":     strSchema("구역으로 필터(선택). 예: 거실"),
				"location": strSchema("장소 이름으로 필터(선택). 예: 아기 트롤리"),
			}),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			items, err := h.port.Items(ctx)
			if err != nil {
				return "", err
			}
			locs, err := h.port.Locations(ctx)
			if err != nil {
				return "", err
			}
			byID := indexLocations(locs)
			zone := strArg(args, "zone")
			locName := strArg(args, "location")

			var lines []string
			for _, it := range items {
				loc := byID[it.LocationID]
				if zone != "" && !strings.EqualFold(it.Zone, zone) && !strings.EqualFold(loc.Zone, zone) {
					continue
				}
				if locName != "" && !strings.EqualFold(loc.Name, locName) {
					continue
				}
				lines = append(lines, formatItemLine(it, loc))
			}
			if len(lines) == 0 {
				return "해당하는 물건이 없어.", nil
			}
			return strings.Join(lines, "\n"), nil
		},
	}
}

func (h homeTools) searchItem() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "search_item",
			Description: "이름으로 물건의 현재 위치를 찾는다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"name": strSchema("찾을 물건 이름. 예: 체온계"),
			}, "name"),
		},
		Run: func(ctx context.Context, args map[string]any) (string, error) {
			name := strArg(args, "name")
			items, err := h.port.SearchItems(ctx, name)
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return fmt.Sprintf("'%s'을(를) 못 찾았어.", name), nil
			}
			locs, err := h.port.Locations(ctx)
			if err != nil {
				return "", err
			}
			byID := indexLocations(locs)
			var lines []string
			for _, it := range items {
				lines = append(lines, formatItemLine(it, byID[it.LocationID]))
			}
			return strings.Join(lines, "\n"), nil
		},
	}
}

func (h homeTools) listCategories() Tool {
	return Tool{
		Decl: &genai.FunctionDeclaration{
			Name:        "list_categories",
			Description: "카테고리(물건 분류) 목록과 기본 보관 위치를 조회한다.",
			Parameters:  objSchema(map[string]*genai.Schema{}),
		},
		Run: func(ctx context.Context, _ map[string]any) (string, error) {
			cats, err := h.port.Categories(ctx)
			if err != nil {
				return "", err
			}
			if len(cats) == 0 {
				return "등록된 카테고리가 없어.", nil
			}
			locs, err := h.port.Locations(ctx)
			if err != nil {
				return "", err
			}
			byID := indexLocations(locs)
			var lines []string
			for _, c := range cats {
				if def := byID[c.DefaultLocationID]; def.Name != "" {
					lines = append(lines, fmt.Sprintf("%s (기본: %s)", c.Name, def.Name))
				} else {
					lines = append(lines, c.Name)
				}
			}
			return strings.Join(lines, "\n"), nil
		},
	}
}

// --- 쓰기 도구 (변경안 생성) ---

func (h homeTools) addLocation() Tool {
	return Tool{
		Write: true,
		Decl: &genai.FunctionDeclaration{
			Name:        "add_location",
			Description: "새 장소(자리)를 특정 구역에 등록한다. 예: '팬트리'를 '거실복도' 구역에.",
			Parameters: objSchema(map[string]*genai.Schema{
				"name": strSchema("새 장소 이름. 예: 팬트리"),
				"zone": strSchema("구역. 예: 거실복도 (없던 구역도 새로 만들 수 있음)"),
			}, "name", "zone"),
		},
		Propose: func(ctx context.Context, args map[string]any) (domain.ChangeProposal, error) {
			name := strings.TrimSpace(strArg(args, "name"))
			zone := strings.TrimSpace(strArg(args, "zone"))
			if name == "" || zone == "" {
				return domain.ChangeProposal{}, fmt.Errorf("장소 이름과 구역이 필요해")
			}
			locs, err := h.port.Locations(ctx)
			if err != nil {
				return domain.ChangeProposal{}, err
			}
			if findLocation(locs, name) != nil {
				return domain.ChangeProposal{}, fmt.Errorf("'%s' 장소는 이미 있어", name)
			}
			return domain.ChangeProposal{
				Op:      "add_location",
				Summary: fmt.Sprintf("장소 추가\n이름: %s\n구역: %s", name, zone),
				Fields:  map[string]string{"name": name, "zone": zone},
			}, nil
		},
	}
}

func (h homeTools) addItem() Tool {
	return Tool{
		Write: true,
		Decl: &genai.FunctionDeclaration{
			Name:        "add_item",
			Description: "물건을 특정 장소에 등록한다. location 은 반드시 등록된 장소 이름이어야 한다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"name":     strSchema("물건 이름. 예: 체온계"),
				"location": strSchema("등록된 장소 이름. 예: 아기 트롤리"),
				"category": strSchema("카테고리 이름(선택). 예: 아기상비약"),
				"quantity": intSchema("수량(선택)"),
			}, "name", "location"),
		},
		Propose: func(ctx context.Context, args map[string]any) (domain.ChangeProposal, error) {
			name := strings.TrimSpace(strArg(args, "name"))
			locName := strings.TrimSpace(strArg(args, "location"))
			if name == "" || locName == "" {
				return domain.ChangeProposal{}, fmt.Errorf("물건 이름과 장소가 필요해")
			}
			locs, err := h.port.Locations(ctx)
			if err != nil {
				return domain.ChangeProposal{}, err
			}
			loc := findLocation(locs, locName)
			if loc == nil {
				return domain.ChangeProposal{}, fmt.Errorf("'%s' 장소를 못 찾았어. 등록된 장소: %s", locName, strings.Join(locationNames(locs), ", "))
			}
			fields := map[string]string{
				"name":          name,
				"location_id":   loc.ID,
				"location_name": loc.Name,
				"zone":          loc.Zone,
			}
			summary := fmt.Sprintf("물건 추가\n품목: %s\n위치: %s", name, locWithZone(*loc))

			if catName := strings.TrimSpace(strArg(args, "category")); catName != "" {
				cats, err := h.port.Categories(ctx)
				if err != nil {
					return domain.ChangeProposal{}, err
				}
				if cat := findCategory(cats, catName); cat != nil {
					fields["category_id"] = cat.ID
					fields["category_name"] = cat.Name
					summary += "\n카테고리: " + cat.Name
				}
			}
			if q := intArg(args, "quantity"); q != nil {
				fields["quantity"] = strconv.Itoa(*q)
				summary += fmt.Sprintf("\n수량: %d", *q)
			}
			return domain.ChangeProposal{Op: "add_item", Summary: summary, Fields: fields}, nil
		},
	}
}

func (h homeTools) addItems() Tool {
	return Tool{
		Write: true,
		Decl: &genai.FunctionDeclaration{
			Name:        "add_items",
			Description: "여러 물건을 한 번에 등록한다. 한 메시지에 물건이 2개 이상이면 이 도구를 쓴다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"items": arraySchema(objSchema(map[string]*genai.Schema{
					"name":     strSchema("물건 이름"),
					"location": strSchema("등록된 장소 이름"),
					"category": strSchema("카테고리 이름(선택)"),
					"quantity": intSchema("수량(선택)"),
				}, "name", "location")),
			}, "items"),
		},
		Propose: func(ctx context.Context, args map[string]any) (domain.ChangeProposal, error) {
			raw, _ := args["items"].([]any)
			if len(raw) == 0 {
				return domain.ChangeProposal{}, fmt.Errorf("추가할 물건이 없어")
			}
			locs, err := h.port.Locations(ctx)
			if err != nil {
				return domain.ChangeProposal{}, err
			}
			cats, err := h.port.Categories(ctx)
			if err != nil {
				return domain.ChangeProposal{}, err
			}
			var items []map[string]string
			var lines []string
			for _, r := range raw {
				m, _ := r.(map[string]any)
				name := strings.TrimSpace(strArg(m, "name"))
				locName := strings.TrimSpace(strArg(m, "location"))
				if name == "" || locName == "" {
					return domain.ChangeProposal{}, fmt.Errorf("물건 이름과 장소가 필요해")
				}
				loc := findLocation(locs, locName)
				if loc == nil {
					return domain.ChangeProposal{}, fmt.Errorf("'%s' 장소를 못 찾았어. 등록된 장소: %s", locName, strings.Join(locationNames(locs), ", "))
				}
				f := map[string]string{"name": name, "location_id": loc.ID, "location_name": loc.Name, "zone": loc.Zone}
				line := fmt.Sprintf("• %s → %s", name, locWithZone(*loc))
				if catName := strings.TrimSpace(strArg(m, "category")); catName != "" {
					if cat := findCategory(cats, catName); cat != nil {
						f["category_id"] = cat.ID
						f["category_name"] = cat.Name
					}
				}
				if q := intArg(m, "quantity"); q != nil {
					f["quantity"] = strconv.Itoa(*q)
					line += fmt.Sprintf(" (%d개)", *q)
				}
				items = append(items, f)
				lines = append(lines, line)
			}
			return domain.ChangeProposal{
				Op:      "add_items",
				Summary: "물건 일괄 추가\n" + strings.Join(lines, "\n"),
				Items:   items,
			}, nil
		},
	}
}

func (h homeTools) updateItem() Tool {
	return Tool{
		Write: true,
		Decl: &genai.FunctionDeclaration{
			Name:        "update_item",
			Description: "물건의 위치를 옮기거나 수량을 바꾼다. 수량은 최종 값(절대값)으로 넣는다. 'N개 썼다'면 먼저 search_item 으로 현재 수량을 확인한 뒤 줄어든 값을 넣어라.",
			Parameters: objSchema(map[string]*genai.Schema{
				"name":     strSchema("바꿀 물건 이름"),
				"location": strSchema("옮길 장소 이름(선택)"),
				"quantity": intSchema("최종 수량(선택)"),
			}, "name"),
		},
		Propose: func(ctx context.Context, args map[string]any) (domain.ChangeProposal, error) {
			item, err := h.resolveItem(ctx, strArg(args, "name"))
			if err != nil {
				return domain.ChangeProposal{}, err
			}
			fields := map[string]string{"item_id": item.ID, "item_name": item.Name}
			var changes []string
			if locName := strings.TrimSpace(strArg(args, "location")); locName != "" {
				locs, err := h.port.Locations(ctx)
				if err != nil {
					return domain.ChangeProposal{}, err
				}
				loc := findLocation(locs, locName)
				if loc == nil {
					return domain.ChangeProposal{}, fmt.Errorf("'%s' 장소를 못 찾았어. 등록된 장소: %s", locName, strings.Join(locationNames(locs), ", "))
				}
				fields["location_id"] = loc.ID
				fields["location_name"] = loc.Name
				fields["zone"] = loc.Zone
				changes = append(changes, "위치 → "+locWithZone(*loc))
			}
			if q := intArg(args, "quantity"); q != nil {
				fields["quantity"] = strconv.Itoa(*q)
				changes = append(changes, fmt.Sprintf("수량 → %d", *q))
			}
			if len(changes) == 0 {
				return domain.ChangeProposal{}, fmt.Errorf("뭘 바꿀지 알려줘(위치나 수량)")
			}
			return domain.ChangeProposal{
				Op:      "update_item",
				Summary: fmt.Sprintf("물건 수정\n품목: %s\n%s", item.Name, strings.Join(changes, "\n")),
				Fields:  fields,
			}, nil
		},
	}
}

func (h homeTools) deleteItem() Tool {
	return Tool{
		Write: true,
		Decl: &genai.FunctionDeclaration{
			Name:        "delete_item",
			Description: "물건을 목록에서 삭제(뺐다/버렸다)한다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"name": strSchema("삭제할 물건 이름"),
			}, "name"),
		},
		Propose: func(ctx context.Context, args map[string]any) (domain.ChangeProposal, error) {
			item, err := h.resolveItem(ctx, strArg(args, "name"))
			if err != nil {
				return domain.ChangeProposal{}, err
			}
			return domain.ChangeProposal{
				Op:      "delete_item",
				Summary: fmt.Sprintf("물건 삭제\n품목: %s", item.Name),
				Fields:  map[string]string{"item_id": item.ID, "item_name": item.Name},
			}, nil
		},
	}
}

func (h homeTools) deleteLocation() Tool {
	return Tool{
		Write: true,
		Decl: &genai.FunctionDeclaration{
			Name:        "delete_location",
			Description: "장소(자리)를 삭제한다.",
			Parameters: objSchema(map[string]*genai.Schema{
				"name": strSchema("삭제할 장소 이름"),
			}, "name"),
		},
		Propose: func(ctx context.Context, args map[string]any) (domain.ChangeProposal, error) {
			locs, err := h.port.Locations(ctx)
			if err != nil {
				return domain.ChangeProposal{}, err
			}
			loc := findLocation(locs, strArg(args, "name"))
			if loc == nil {
				return domain.ChangeProposal{}, fmt.Errorf("'%s' 장소를 못 찾았어.", strArg(args, "name"))
			}
			return domain.ChangeProposal{
				Op:      "delete_location",
				Summary: fmt.Sprintf("장소 삭제\n이름: %s", loc.Name),
				Fields:  map[string]string{"location_id": loc.ID, "location_name": loc.Name},
			}, nil
		},
	}
}

// resolveItem 은 이름으로 물건 1개를 찾는다. 0개/여러 개면 에러(에이전트가 되묻기).
func (h homeTools) resolveItem(ctx context.Context, name string) (notion.Item, error) {
	name = strings.TrimSpace(name)
	items, err := h.port.SearchItems(ctx, name)
	if err != nil {
		return notion.Item{}, err
	}
	if len(items) == 0 {
		return notion.Item{}, fmt.Errorf("'%s'을(를) 못 찾았어.", name)
	}
	if len(items) > 1 {
		var names []string
		for _, it := range items {
			names = append(names, it.Name)
		}
		return notion.Item{}, fmt.Errorf("'%s'에 해당하는 게 여러 개야: %s. 더 정확히 알려줄래?", name, strings.Join(names, ", "))
	}
	return items[0], nil
}

// --- 순수 helper ---

func formatByZone(byZone map[string][]string) string {
	zones := make([]string, 0, len(byZone))
	for z := range byZone {
		zones = append(zones, z)
	}
	sort.Strings(zones)
	var lines []string
	for _, z := range zones {
		label := z
		if label == "" {
			label = "(구역 없음)"
		}
		lines = append(lines, fmt.Sprintf("%s: %s", label, strings.Join(byZone[z], ", ")))
	}
	return strings.Join(lines, "\n")
}

func formatItemLine(it notion.Item, loc notion.Location) string {
	where := locWithZone(loc)
	if where == "" {
		where = "위치 미상"
	}
	line := fmt.Sprintf("%s — %s", it.Name, where)
	if it.Quantity != nil {
		line = fmt.Sprintf("%s, %d개", line, *it.Quantity)
	}
	return line
}

func locWithZone(loc notion.Location) string {
	switch {
	case loc.Name == "":
		return ""
	case loc.Zone == "":
		return loc.Name
	default:
		return fmt.Sprintf("%s - %s", loc.Zone, loc.Name)
	}
}

func indexLocations(locs []notion.Location) map[string]notion.Location {
	m := make(map[string]notion.Location, len(locs))
	for _, l := range locs {
		m[l.ID] = l
	}
	return m
}

func locationNames(locs []notion.Location) []string {
	out := make([]string, len(locs))
	for i, l := range locs {
		out[i] = l.Name
	}
	return out
}

func findLocation(locs []notion.Location, name string) *notion.Location {
	name = strings.TrimSpace(name)
	for i := range locs {
		if strings.EqualFold(locs[i].Name, name) {
			return &locs[i]
		}
	}
	return nil
}

func findCategory(cats []notion.Category, name string) *notion.Category {
	name = strings.TrimSpace(name)
	for i := range cats {
		if strings.EqualFold(cats[i].Name, name) {
			return &cats[i]
		}
	}
	return nil
}
