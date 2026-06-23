package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/pkg/log"
)

// HomeApplier 는 승인된 집정리 변경안을 Notion 에 반영한다. domain.ProposalApplier 구현.
type HomeApplier struct {
	port     HomePort
	renderer *MapRenderer // 쓰기 후 지도 갱신(선택, nil 가능)
}

// NewHomeApplier 는 HomeApplier 를 생성한다. renderer 가 있으면 쓰기 후 지도를 다시 그린다.
func NewHomeApplier(port HomePort, renderer *MapRenderer) HomeApplier {
	return HomeApplier{port: port, renderer: renderer}
}

// Apply 는 변경안을 반영하고, 성공 시 지도 페이지를 다시 그린다(best-effort).
func (a HomeApplier) Apply(ctx context.Context, p domain.ChangeProposal) (domain.Reply, error) {
	reply, err := a.apply(ctx, p)
	if err != nil {
		return domain.Reply{}, err
	}
	if a.renderer != nil {
		if rerr := a.renderer.Render(ctx); rerr != nil {
			log.FromContext(ctx).Error("지도 갱신 실패", "error", rerr)
		}
	}
	return reply, nil
}

// apply 는 변경안 Op 에 따라 Notion 쓰기를 수행한다.
func (a HomeApplier) apply(ctx context.Context, p domain.ChangeProposal) (domain.Reply, error) {
	f := p.Fields
	switch p.Op {
	case "add_item":
		if f["name"] == "" || f["location_id"] == "" {
			return domain.Reply{}, fmt.Errorf("변경안이 불완전함(item/location 누락)")
		}
		catID, err := a.ensureCat(ctx, nil, f["category_name"])
		if err != nil {
			return domain.Reply{}, fmt.Errorf("카테고리 처리 실패: %w", err)
		}
		if _, err := a.port.CreateItem(ctx, f["name"], catID, f["location_id"], f["zone"], parseQty(f["quantity"])); err != nil {
			return domain.Reply{}, fmt.Errorf("물건 추가 실패: %w", err)
		}
		return domain.Reply{Text: fmt.Sprintf("✅ '%s'을(를) %s에 추가했어.", f["name"], f["location_name"])}, nil

	case "add_items":
		if len(p.Items) == 0 {
			return domain.Reply{}, fmt.Errorf("추가할 물건이 없음")
		}
		cache := map[string]string{} // 같은 카테고리 중복 생성 방지
		for _, it := range p.Items {
			catID, err := a.ensureCat(ctx, cache, it["category_name"])
			if err != nil {
				return domain.Reply{}, fmt.Errorf("카테고리 처리 실패: %w", err)
			}
			if _, err := a.port.CreateItem(ctx, it["name"], catID, it["location_id"], it["zone"], parseQty(it["quantity"])); err != nil {
				return domain.Reply{}, fmt.Errorf("'%s' 추가 실패: %w", it["name"], err)
			}
		}
		return domain.Reply{Text: fmt.Sprintf("✅ 물건 %d개를 추가했어.", len(p.Items))}, nil

	case "update_item":
		if f["item_id"] == "" {
			return domain.Reply{}, fmt.Errorf("변경안이 불완전함(item_id 누락)")
		}
		catID, err := a.ensureCat(ctx, nil, f["category_name"])
		if err != nil {
			return domain.Reply{}, fmt.Errorf("카테고리 처리 실패: %w", err)
		}
		if err := a.port.UpdateItem(ctx, f["item_id"], catID, f["location_id"], f["zone"], parseQty(f["quantity"])); err != nil {
			return domain.Reply{}, fmt.Errorf("물건 수정 실패: %w", err)
		}
		return domain.Reply{Text: fmt.Sprintf("✅ '%s'을(를) 수정했어.", f["item_name"])}, nil

	case "delete_item":
		if f["item_id"] == "" {
			return domain.Reply{}, fmt.Errorf("변경안이 불완전함(item_id 누락)")
		}
		if err := a.port.ArchiveItem(ctx, f["item_id"]); err != nil {
			return domain.Reply{}, fmt.Errorf("물건 삭제 실패: %w", err)
		}
		return domain.Reply{Text: fmt.Sprintf("✅ '%s'을(를) 삭제했어.", f["item_name"])}, nil

	case "delete_items":
		if len(p.Items) == 0 {
			return domain.Reply{}, fmt.Errorf("삭제할 물건이 없음")
		}
		for _, it := range p.Items {
			if err := a.port.ArchiveItem(ctx, it["item_id"]); err != nil {
				return domain.Reply{}, fmt.Errorf("'%s' 삭제 실패: %w", it["item_name"], err)
			}
		}
		return domain.Reply{Text: fmt.Sprintf("✅ %d개를 정리했어.", len(p.Items))}, nil

	case "add_location":
		if f["name"] == "" || f["zone"] == "" {
			return domain.Reply{}, fmt.Errorf("변경안이 불완전함(name/zone 누락)")
		}
		if _, err := a.port.CreateLocation(ctx, f["name"], f["zone"]); err != nil {
			return domain.Reply{}, fmt.Errorf("장소 추가 실패: %w", err)
		}
		return domain.Reply{Text: fmt.Sprintf("✅ 장소 '%s'을(를) %s 구역에 추가했어.", f["name"], f["zone"])}, nil

	case "update_location":
		if f["location_id"] == "" {
			return domain.Reply{}, fmt.Errorf("변경안이 불완전함(location_id 누락)")
		}
		if err := a.port.UpdateLocation(ctx, f["location_id"], f["new_name"], f["new_zone"]); err != nil {
			return domain.Reply{}, fmt.Errorf("장소 수정 실패: %w", err)
		}
		return domain.Reply{Text: fmt.Sprintf("✅ 장소 '%s'을(를) 수정했어.", f["old_label"])}, nil

	case "delete_location":
		if f["location_id"] == "" {
			return domain.Reply{}, fmt.Errorf("변경안이 불완전함(location_id 누락)")
		}
		if err := a.port.ArchiveLocation(ctx, f["location_id"]); err != nil {
			return domain.Reply{}, fmt.Errorf("장소 삭제 실패: %w", err)
		}
		return domain.Reply{Text: fmt.Sprintf("✅ 장소 '%s'을(를) 삭제했어.", f["location_name"])}, nil

	default:
		return domain.Reply{}, fmt.Errorf("알 수 없는 변경안: %s", p.Op)
	}
}

// ensureCat 은 카테고리 이름을 page ID 로 바꾼다(없으면 생성). 빈 이름이면 "". cache 로 일괄 중복 생성 방지.
func (a HomeApplier) ensureCat(ctx context.Context, cache map[string]string, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	if cache != nil {
		if id, ok := cache[name]; ok {
			return id, nil
		}
	}
	id, err := a.port.EnsureCategory(ctx, name)
	if err != nil {
		return "", err
	}
	if cache != nil {
		cache[name] = id
	}
	return id, nil
}

// parseQty 는 문자열 수량을 *int 로 바꾼다(빈 값/오류면 nil).
func parseQty(s string) *int {
	if s == "" {
		return nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		return &n
	}
	return nil
}

// todoistApplier 는 delete_todo 변경안을 Todoist 에 반영한다.
type todoistApplier struct {
	port TodoistPort
}

// NewTodoistApplier 는 Todoist 삭제 승인 처리기를 만든다.
func NewTodoistApplier(port TodoistPort) domain.ProposalApplier {
	return todoistApplier{port: port}
}

func (a todoistApplier) Apply(ctx context.Context, p domain.ChangeProposal) (domain.Reply, error) {
	if p.Op != "delete_todo" {
		return domain.Reply{}, fmt.Errorf("todoistApplier: 지원하지 않는 op %q", p.Op)
	}
	if err := a.port.DeleteTask(ctx, p.Fields["task_id"]); err != nil {
		return domain.Reply{}, err
	}
	return domain.Reply{Text: "🗑️ '" + p.Fields["content"] + "' 삭제했습니다."}, nil
}

// dispatchApplier 는 ChangeProposal.Op 로 applier 를 고르고, 없으면 fallback 으로 위임한다.
type dispatchApplier struct {
	byOp     map[string]domain.ProposalApplier
	fallback domain.ProposalApplier
}

// NewDispatchApplier 는 Op 분기 applier 를 만든다.
// fallback 은 nil 이면 안 된다(매칭 안 되는 op 가 오면 fallback 으로 위임하므로).
func NewDispatchApplier(byOp map[string]domain.ProposalApplier, fallback domain.ProposalApplier) domain.ProposalApplier {
	return dispatchApplier{byOp: byOp, fallback: fallback}
}

func (a dispatchApplier) Apply(ctx context.Context, p domain.ChangeProposal) (domain.Reply, error) {
	if ap, ok := a.byOp[p.Op]; ok {
		return ap.Apply(ctx, p)
	}
	return a.fallback.Apply(ctx, p)
}
