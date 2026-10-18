package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	bizmme "framework/app/admin/internal/biz/mme"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/mmetemplate"
)

// MmeTemplateRepo 混剪模板仓储（FR-MIX-08）。
type MmeTemplateRepo struct {
	data *Data
	log  *log.Helper
}

// NewMmeTemplateRepo 构造。
func NewMmeTemplateRepo(data *Data, logger log.Logger) bizmme.TemplateRepo {
	return &MmeTemplateRepo{data: data, log: log.NewHelper(log.With(logger, "module", "mme/template/repo"))}
}

func toBizTemplate(e *ent.MmeTemplate) *bizmme.Template {
	if e == nil {
		return nil
	}
	return &bizmme.Template{
		ID:          e.ID,
		Name:        e.Name,
		IntroPath:   e.IntroPath,
		OutroPath:   e.OutroPath,
		Transition:  e.Transition,
		CaptionSkin: e.CaptionSkin,
		Enabled:     e.Enabled,
	}
}

func (r *MmeTemplateRepo) Create(ctx context.Context, t *bizmme.Template) error {
	now := time.Now()
	b := r.data.db.Client().MmeTemplate.Create().
		SetName(t.Name).
		SetIntroPath(t.IntroPath).
		SetOutroPath(t.OutroPath).
		SetTransition(t.Transition).
		SetEnabled(t.Enabled).
		SetCreateTime(now).
		SetUpdateTime(now)
	if t.CaptionSkin != nil {
		b = b.SetCaptionSkin(t.CaptionSkin)
	}
	if t.ID != "" {
		b = b.SetID(t.ID)
	}
	e, err := b.Save(ctx)
	if err != nil {
		r.log.Errorf("create mme template failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("create mme template failed")
	}
	t.ID = e.ID
	return nil
}

func (r *MmeTemplateRepo) List(ctx context.Context, enabledOnly bool) ([]*bizmme.Template, error) {
	q := r.data.db.Client().MmeTemplate.Query()
	if enabledOnly {
		q = q.Where(mmetemplate.EnabledEQ(true))
	}
	rows, err := q.Order(ent.Asc(mmetemplate.FieldCreateTime)).All(ctx)
	if err != nil {
		r.log.Errorf("list mme templates failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("list mme templates failed")
	}
	out := make([]*bizmme.Template, 0, len(rows))
	for _, e := range rows {
		out = append(out, toBizTemplate(e))
	}
	return out, nil
}

func (r *MmeTemplateRepo) Get(ctx context.Context, id string) (*bizmme.Template, error) {
	e, err := r.data.db.Client().MmeTemplate.Query().
		Where(mmetemplate.IDEQ(id)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get mme template %s failed: %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("get mme template failed")
	}
	return toBizTemplate(e), nil
}

func (r *MmeTemplateRepo) Count(ctx context.Context) (int, error) {
	n, err := r.data.db.Client().MmeTemplate.Query().Count(ctx)
	if err != nil {
		r.log.Errorf("count mme templates failed: %s", err.Error())
		return 0, adminV1.ErrorInternalServerError("count mme templates failed")
	}
	return n, nil
}
