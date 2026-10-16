package data

import "context"

import "github.com/go-kratos/kratos/v2/log"

type treePair struct {
	ID       uint32 `json:"id"`
	ParentID uint32 `json:"parent_id"`
}

func buildChildrenMap(rows []treePair) map[uint32][]uint32 {
	children := make(map[uint32][]uint32, len(rows))
	for _, r := range rows {
		children[r.ParentID] = append(children[r.ParentID], r.ID)
	}
	return children
}

func subtreeIDs(children map[uint32][]uint32, root uint32) []uint32 {
	ids := make([]uint32, 0, 8)
	seen := make(map[uint32]struct{}, 8)
	queue := []uint32{root}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if _, ok := seen[cur]; ok {
			continue
		}
		seen[cur] = struct{}{}
		ids = append(ids, cur)
		queue = append(queue, children[cur]...)
	}
	return ids
}

// deptDirectAssetCounts skeleton stub: asset tables removed.
func deptDirectAssetCounts(ctx context.Context, logger *log.Helper, data *Data, byUseDept bool) map[uint32]int {
	_ = ctx
	_ = logger
	_ = data
	_ = byUseDept
	return map[uint32]int{}
}
