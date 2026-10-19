package service

import (
	"fmt"
	"testing"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"

	bizmme "framework/app/admin/internal/biz/mme"
)

func TestMmeBizErrMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		code   int32
		reason string
	}{
		{"reject reason empty wraps to 400", fmt.Errorf("%w: variant v_1", bizmme.ErrRejectReasonEmpty), 400, "MME_REJECT_REASON_EMPTY"},
		{"expand too large 400", bizmme.ErrExpandTooLarge, 400, "MME_EXPAND_TOO_LARGE"},
		{"highlights empty 400", bizmme.ErrHighlightsEmpty, 400, "MME_HIGHLIGHTS_EMPTY"},
		{"task not found 404", bizmme.ErrTaskNotFound, 404, "MME_TASK_NOT_FOUND"},
		{"set not found 404", bizmme.ErrSetNotFound, 404, "MME_SET_NOT_FOUND"},
		{"asset missing 404", bizmme.ErrAssetMissing, 404, "MME_ASSET_MISSING"},
		{"invalid state 409", bizmme.ErrInvalidState, 409, "MME_INVALID_STATE"},
		{"qc block 409", bizmme.ErrQCBlock, 409, "MME_QC_BLOCK"},
		{"storage full 409", bizmme.ErrStorageFull, 409, "MME_STORAGE_FULL"},
		{"llm unavailable 502", bizmme.ErrLLMUnavailable, 502, "MME_LLM_UNAVAILABLE"},
		{"render fail 502", bizmme.ErrRenderFail, 502, "MME_RENDER_FAIL"},
		{"queue unavailable 502", bizmme.ErrQueueUnavailable, 502, "MME_QUEUE_UNAVAILABLE"},
	}
	for _, c := range cases {
		got := mmeBizErr(c.err)
		ke := kratoserrors.FromError(got)
		if ke.Code != c.code {
			t.Errorf("%s: want code %d, got %d", c.name, c.code, ke.Code)
		}
		if ke.Reason != c.reason {
			t.Errorf("%s: want reason %q, got %q", c.name, c.reason, ke.Reason)
		}
		if ke.Message == "" {
			t.Errorf("%s: message must retain original context", c.name)
		}
	}
}

func TestMmeBizErrPassthrough(t *testing.T) {
	if mmeBizErr(nil) != nil {
		t.Fatal("nil must stay nil")
	}
	plain := fmt.Errorf("boom")
	if got := mmeBizErr(plain); got != plain {
		t.Fatalf("non-sentinel errors must pass through unchanged, got %v", got)
	}
}
