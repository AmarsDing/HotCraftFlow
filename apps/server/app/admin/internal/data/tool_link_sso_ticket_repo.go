package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/toollinkssoticket"

	adminV1 "framework/api/gen/go/admin/v1"
)

// ToolLinkSsoTicketRepo SSO 票据仓储。
type ToolLinkSsoTicketRepo struct {
	data *Data
	log  *log.Helper
}

func NewToolLinkSsoTicketRepo(data *Data, logger log.Logger) biz.ToolLinkSsoTicketRepo {
	return &ToolLinkSsoTicketRepo{
		log:  log.NewHelper(log.With(logger, "module", "tool-link-sso-ticket/repo/admin-service")),
		data: data,
	}
}

func (r *ToolLinkSsoTicketRepo) Create(ctx context.Context, ticket *biz.ToolLinkSsoTicket) error {
	if ticket == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	err := r.data.db.Client().ToolLinkSsoTicket.Create().
		SetTicket(ticket.Ticket).
		SetUserID(ticket.UserID).
		SetToolLinkID(ticket.ToolLinkID).
		SetExpireAt(ticket.ExpireAt).
		SetCreateTime(time.Now()).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("insert sso ticket failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("issue sso ticket failed")
	}
	return nil
}

func (r *ToolLinkSsoTicketRepo) GetByTicket(ctx context.Context, ticket string) (*biz.ToolLinkSsoTicket, error) {
	entity, err := r.data.db.Client().ToolLinkSsoTicket.Query().
		Where(toollinkssoticket.TicketEQ(ticket)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query sso ticket failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query sso ticket failed")
	}

	return &biz.ToolLinkSsoTicket{
		Ticket:     entity.Ticket,
		UserID:     entity.UserID,
		ToolLinkID: entity.ToolLinkID,
		ExpireAt:   entity.ExpireAt,
		UsedAt:     entity.UsedAt,
	}, nil
}

func (r *ToolLinkSsoTicketRepo) MarkUsed(ctx context.Context, ticket string, usedAt time.Time) error {
	affected, err := r.data.db.Client().ToolLinkSsoTicket.Update().
		Where(toollinkssoticket.TicketEQ(ticket), toollinkssoticket.UsedAtIsNil()).
		SetUsedAt(usedAt).
		SetUpdateTime(usedAt).
		Save(ctx)
	if err != nil {
		r.log.Errorf("mark sso ticket used failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("verify sso ticket failed")
	}
	if affected == 0 {
		return adminV1.ErrorNotFound("sso ticket not found or already used")
	}
	return nil
}
