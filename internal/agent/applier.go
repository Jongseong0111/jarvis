package agent

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Jongseong0111/jarvis/domain"
)

// HomeApplier 는 승인된 집정리 변경안을 Notion 에 반영한다. domain.ProposalApplier 구현.
type HomeApplier struct {
	port HomePort
}

// NewHomeApplier 는 HomeApplier 를 생성한다.
func NewHomeApplier(port HomePort) HomeApplier {
	return HomeApplier{port: port}
}

// Apply 는 변경안 Op 에 따라 Notion 쓰기를 수행한다. ChannelID 는 호출자가 채운다.
func (a HomeApplier) Apply(ctx context.Context, p domain.ChangeProposal) (domain.Reply, error) {
	f := p.Fields
	switch p.Op {
	case "add_item":
		if f["name"] == "" || f["location_id"] == "" {
			return domain.Reply{}, fmt.Errorf("변경안이 불완전함(item/location 누락)")
		}
		var q *int
		if s := f["quantity"]; s != "" {
			if n, err := strconv.Atoi(s); err == nil {
				q = &n
			}
		}
		if _, err := a.port.CreateItem(ctx, f["name"], f["category_id"], f["location_id"], f["zone"], q); err != nil {
			return domain.Reply{}, fmt.Errorf("물건 추가 실패: %w", err)
		}
		return domain.Reply{Text: fmt.Sprintf("✅ '%s'을(를) %s에 추가했어.", f["name"], f["location_name"])}, nil

	case "add_items":
		if len(p.Items) == 0 {
			return domain.Reply{}, fmt.Errorf("추가할 물건이 없음")
		}
		for _, it := range p.Items {
			if _, err := a.port.CreateItem(ctx, it["name"], it["category_id"], it["location_id"], it["zone"], parseQty(it["quantity"])); err != nil {
				return domain.Reply{}, fmt.Errorf("'%s' 추가 실패: %w", it["name"], err)
			}
		}
		return domain.Reply{Text: fmt.Sprintf("✅ 물건 %d개를 추가했어.", len(p.Items))}, nil

	case "update_item":
		if f["item_id"] == "" {
			return domain.Reply{}, fmt.Errorf("변경안이 불완전함(item_id 누락)")
		}
		if err := a.port.UpdateItem(ctx, f["item_id"], f["location_id"], f["zone"], parseQty(f["quantity"])); err != nil {
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

	case "add_location":
		if f["name"] == "" || f["zone"] == "" {
			return domain.Reply{}, fmt.Errorf("변경안이 불완전함(name/zone 누락)")
		}
		if _, err := a.port.CreateLocation(ctx, f["name"], f["zone"]); err != nil {
			return domain.Reply{}, fmt.Errorf("장소 추가 실패: %w", err)
		}
		return domain.Reply{Text: fmt.Sprintf("✅ 장소 '%s'을(를) %s 구역에 추가했어.", f["name"], f["zone"])}, nil

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
