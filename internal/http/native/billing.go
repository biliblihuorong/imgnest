package native

import (
	"context"
	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
	"strconv"
	"time"
)

// Billing is the optional account-commerce service, guarded on every request.
type Billing interface {
	extension.BillingStore
	Enabled(context.Context) bool
}

// RegisterBillingRoutes binds session-only commerce and administrator routes.
func (h *Handler) RegisterBillingRoutes(ctx context.Context, r gin.IRouter, s Billing) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	gate := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		if !s.Enabled(c.Request.Context()) {
			fail(c, service.ErrNotFound)
			c.Abort()
		}
	}
	admin := func(c *gin.Context) {
		if identity(c).User.Role != model.UserRoleAdmin {
			fail(c, service.ErrForbidden)
			c.Abort()
		}
	}
	writeLimit := func(c *gin.Context) {
		if !h.limits.Allow("billing:"+strconv.FormatUint(identity(c).User.ID, 10), 30) {
			c.AbortWithStatusJSON(429, Response{Code: 30003, Message: "too many requests", Data: nil})
		}
	}
	actor := func(c *gin.Context) extension.BillingActor { return service.BillingActorFromIdentity(identity(c)) }
	user := r.Group("/api/user/billing", h.authenticate, gate)
	user.GET("", func(c *gin.Context) { v, e := s.Overview(c.Request.Context(), actor(c)); billingResult(c, v, e) })
	user.GET("/plans", func(c *gin.Context) { v, e := s.Plans(c.Request.Context(), actor(c), false); billingResult(c, v, e) })
	user.GET("/ledger", func(c *gin.Context) {
		f, ok := billingFilter(c, false)
		if !ok {
			return
		}
		v, e := s.Ledger(c.Request.Context(), actor(c), f)
		billingResult(c, v, e)
	})
	user.GET("/orders", func(c *gin.Context) {
		f, ok := billingFilter(c, false)
		if !ok {
			return
		}
		f.RequestID = c.Query("request_id")
		v, e := s.Orders(c.Request.Context(), actor(c), f)
		billingResult(c, v, e)
	})
	user.POST("/purchases", writeLimit, func(c *gin.Context) {
		var in extension.BillingPurchase
		if !decode(c, &in) {
			return
		}
		v, e := s.Purchase(c.Request.Context(), actor(c), in)
		billingResult(c, v, e)
	})
	management := r.Group("/api/admin/billing", h.authenticate, admin, gate)
	management.GET("/plans", func(c *gin.Context) { v, e := s.Plans(c.Request.Context(), actor(c), true); billingResult(c, v, e) })
	savePlan := func(c *gin.Context) {
		var in extension.BillingPlanInput
		if !decode(c, &in) {
			return
		}
		if c.Param("id") != "" {
			id, ok := adminID(c)
			if !ok {
				return
			}
			in.ID = id
		}
		v, e := s.SavePlan(c.Request.Context(), actor(c), in)
		billingResult(c, v, e)
	}
	management.POST("/plans", writeLimit, savePlan)
	management.PATCH("/plans/:id", writeLimit, savePlan)
	management.GET("/wallets", func(c *gin.Context) {
		f, ok := billingFilter(c, true)
		if !ok {
			return
		}
		v, e := s.Wallets(c.Request.Context(), actor(c), f)
		billingResult(c, v, e)
	})
	management.GET("/ledger", func(c *gin.Context) {
		f, ok := billingFilter(c, true)
		if !ok {
			return
		}
		v, e := s.Ledger(c.Request.Context(), actor(c), f)
		billingResult(c, v, e)
	})
	management.GET("/orders", func(c *gin.Context) {
		f, ok := billingFilter(c, true)
		if !ok {
			return
		}
		f.RequestID = c.Query("request_id")
		v, e := s.Orders(c.Request.Context(), actor(c), f)
		billingResult(c, v, e)
	})
	management.GET("/memberships", func(c *gin.Context) {
		f, ok := billingFilter(c, true)
		if !ok {
			return
		}
		v, e := s.Memberships(c.Request.Context(), actor(c), f)
		billingResult(c, v, e)
	})
	management.POST("/adjustments", writeLimit, func(c *gin.Context) {
		var in extension.BillingAdjustment
		if !decode(c, &in) {
			return
		}
		v, e := s.Adjust(c.Request.Context(), actor(c), in)
		billingResult(c, v, e)
	})
	management.POST("/reversals", writeLimit, func(c *gin.Context) {
		var in extension.BillingReversal
		if !decode(c, &in) {
			return
		}
		v, e := s.Reverse(c.Request.Context(), actor(c), in)
		billingResult(c, v, e)
	})
	return nil
}
func billingResult(c *gin.Context, v any, err error) {
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, v)
}
func billingFilter(c *gin.Context, all bool) (extension.BillingFilter, bool) {
	helper := adminHandler{}
	page, size, ok := helper.adminPage(c, 20)
	if !ok {
		return extension.BillingFilter{}, false
	}
	if page > 1_000_000 {
		fail(c, service.ErrInvalidInput)
		return extension.BillingFilter{}, false
	}
	f := extension.BillingFilter{All: all, Page: page, Size: size}
	if !all {
		f.UserID = identity(c).User.ID
	} else if raw := c.Query("user_id"); raw != "" {
		id, e := parseImageID(raw)
		if e != nil {
			fail(c, e)
			return f, false
		}
		f.UserID = id
	}
	for key, target := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if value := c.Query(key); value != "" {
			at, e := time.Parse(time.RFC3339, value)
			if e != nil {
				fail(c, service.ErrInvalidInput)
				return f, false
			}
			*target = &at
		}
	}
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		fail(c, service.ErrInvalidInput)
		return f, false
	}
	return f, true
}
