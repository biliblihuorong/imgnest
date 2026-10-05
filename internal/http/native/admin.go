package native

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// AdminService is the management-console business API consumed by the
// /api/admin routes; it shares the persistence layer with the other services.
type AdminService interface {
	CreateUser(context.Context, uint64, service.AdminUserInput) (service.UserView, error)
	ListUsers(context.Context, int, int, string) (service.AdminUserPage, error)
	PatchUser(context.Context, uint64, uint64, service.AdminUserPatch) (service.UserView, error)
	ListGroups(context.Context) ([]service.GroupView, error)
	CreateGroup(context.Context, service.GroupInput) (service.GroupView, error)
	PatchGroup(context.Context, uint64, service.GroupPatch) (service.GroupView, error)
	DeleteGroup(context.Context, uint64) error
	ListStorages(context.Context) ([]service.StorageView, error)
	CreateStorage(context.Context, service.StorageInput) (service.StorageView, error)
	PatchStorage(context.Context, uint64, service.StoragePatch) (service.StorageView, error)
	DeleteStorage(context.Context, uint64) error
	TestStorage(context.Context, uint64) (service.StorageTestResult, error)
	ListPolicies(context.Context) ([]model.Policy, error)
	CreatePolicy(context.Context, service.PolicyPatch) (model.Policy, error)
	PatchPolicy(context.Context, uint64, service.PolicyPatch) (model.Policy, error)
	DeletePolicy(context.Context, uint64) error
	PreviewPolicy(context.Context, uint64, string, string) (string, error)
	GetSettings(context.Context) (service.AdminSettingsView, error)
	PutSettings(context.Context, service.SettingsPatch) (service.AdminSettingsView, error)
}

// AdminImages is the site-wide image portion of the console.
type AdminImages interface {
	AdminList(context.Context, service.TokenSubject, service.AdminImageQuery) (service.ImagePage, error)
	AdminTrash(context.Context, service.TokenSubject, uint64) error
	AdminPurgeAll(context.Context, service.TokenSubject) (int64, error)
}

// RegisterAdminRoutes binds the /api/admin subtree. Every route requires a
// valid bearer token AND the administrator role.
func (h *Handler) RegisterAdminRoutes(ctx context.Context, router gin.IRouter, admin AdminService, images AdminImages) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register admin routes: %w", err)
	}
	if admin == nil || images == nil {
		return fmt.Errorf("register admin routes: missing dependencies")
	}
	handler := &adminHandler{auth: h, admin: admin, images: images}
	adminGroup := router.Group("/api/admin", h.authenticate, handler.requireAdmin)
	adminGroup.GET("/users", handler.listUsers)
	adminGroup.POST("/users", handler.createUser)
	adminGroup.PATCH("/users/:id", handler.patchUser)
	adminGroup.GET("/groups", handler.listGroups)
	adminGroup.POST("/groups", handler.createGroup)
	adminGroup.PATCH("/groups/:id", handler.patchGroup)
	adminGroup.DELETE("/groups/:id", handler.deleteGroup)
	adminGroup.GET("/storages", handler.listStorages)
	adminGroup.POST("/storages", handler.createStorage)
	adminGroup.PATCH("/storages/:id", handler.patchStorage)
	adminGroup.DELETE("/storages/:id", handler.deleteStorage)
	adminGroup.POST("/storages/:id/test", handler.testStorage)
	adminGroup.GET("/policies", handler.listPolicies)
	adminGroup.POST("/policies", handler.createPolicy)
	adminGroup.POST("/policies/preview", handler.previewPolicy)
	adminGroup.PATCH("/policies/:id", handler.patchPolicy)
	adminGroup.DELETE("/policies/:id", handler.deletePolicy)
	adminGroup.GET("/settings", handler.getSettings)
	adminGroup.PUT("/settings", handler.putSettings)
	adminGroup.GET("/images", handler.listImages)
	adminGroup.DELETE("/images/:id", handler.deleteImage)
	adminGroup.POST("/trash/purge-all", handler.purgeAll)
	return nil
}

type adminHandler struct {
	auth   *Handler
	admin  AdminService
	images AdminImages
}

// requireAdmin rejects authenticated non-administrators; the native identity
// already carries the account role.
func (a *adminHandler) requireAdmin(c *gin.Context) {
	if identity(c).User.Role != model.UserRoleAdmin {
		fail(c, service.ErrForbidden)
		c.Abort()
	}
}

func adminID(c *gin.Context) (uint64, bool) {
	id, err := parseImageID(c.Param("id"))
	if err != nil {
		fail(c, err)
		return 0, false
	}
	return id, true
}

// adminPage parses the shared page/size pagination parameters.
func (a *adminHandler) adminPage(c *gin.Context, defaultSize int) (int, int, bool) {
	page, size := 1, defaultSize
	for name, target := range map[string]*int{"page": &page, "size": &size} {
		if value, exists := c.GetQuery(name); exists {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 1 {
				fail(c, service.ErrInvalidInput)
				return 0, 0, false
			}
			*target = parsed
		}
	}
	if size > 100 || page > (math.MaxInt/size) {
		fail(c, service.ErrInvalidInput)
		return 0, 0, false
	}
	return page, size, true
}

func (a *adminHandler) listUsers(c *gin.Context) {
	page, size, ok := a.adminPage(c, 20)
	if !ok {
		return
	}
	result, err := a.admin.ListUsers(c.Request.Context(), page, size, c.Query("keyword"))
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, result)
}

func (a *adminHandler) createUser(c *gin.Context) {
	var input service.AdminUserInput
	if !decodeAdminAccount(c, &input) {
		return
	}
	user, err := a.admin.CreateUser(c.Request.Context(), identity(c).User.ID, input)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 201, user)
}

// Account fields are never nullable. Decode the bounded object first so null
// cannot silently become a missing pointer or an empty scalar, then enforce
// the explicit request DTO's allowlist with DisallowUnknownFields.
func decodeAdminAccount(c *gin.Context, target any) bool {
	var fields map[string]json.RawMessage
	if !decode(c, &fields) {
		return false
	}
	if fields == nil {
		fail(c, service.ErrInvalidInput)
		return false
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			fail(c, service.ErrInvalidInput)
			return false
		}
	}
	body, err := json.Marshal(fields)
	if err != nil {
		fail(c, service.ErrInvalidInput)
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		fail(c, service.ErrInvalidInput)
		return false
	}
	return true
}

func (a *adminHandler) patchUser(c *gin.Context) {
	id, ok := adminID(c)
	if !ok {
		return
	}
	var patch service.AdminUserPatch
	if !decodeAdminAccount(c, &patch) {
		return
	}
	user, err := a.admin.PatchUser(c.Request.Context(), identity(c).User.ID, id, patch)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, user)
}

func (a *adminHandler) listGroups(c *gin.Context) {
	groups, err := a.admin.ListGroups(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, groups)
}

func (a *adminHandler) createGroup(c *gin.Context) {
	var input service.GroupInput
	if !decode(c, &input) {
		return
	}
	group, err := a.admin.CreateGroup(c.Request.Context(), input)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 201, group)
}

func (a *adminHandler) patchGroup(c *gin.Context) {
	id, ok := adminID(c)
	if !ok {
		return
	}
	var patch service.GroupPatch
	if !decode(c, &patch) {
		return
	}
	group, err := a.admin.PatchGroup(c.Request.Context(), id, patch)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, group)
}

func (a *adminHandler) deleteGroup(c *gin.Context) {
	id, ok := adminID(c)
	if !ok {
		return
	}
	if err := a.admin.DeleteGroup(c.Request.Context(), id); err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, nil)
}

func (a *adminHandler) listStorages(c *gin.Context) {
	storages, err := a.admin.ListStorages(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, storages)
}

func (a *adminHandler) createStorage(c *gin.Context) {
	var input service.StorageInput
	if !decode(c, &input) {
		return
	}
	storage, err := a.admin.CreateStorage(c.Request.Context(), input)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 201, storage)
}

func (a *adminHandler) patchStorage(c *gin.Context) {
	id, ok := adminID(c)
	if !ok {
		return
	}
	var patch service.StoragePatch
	if !decode(c, &patch) {
		return
	}
	storage, err := a.admin.PatchStorage(c.Request.Context(), id, patch)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, storage)
}

func (a *adminHandler) deleteStorage(c *gin.Context) {
	id, ok := adminID(c)
	if !ok {
		return
	}
	if err := a.admin.DeleteStorage(c.Request.Context(), id); err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, nil)
}

func (a *adminHandler) testStorage(c *gin.Context) {
	id, ok := adminID(c)
	if !ok {
		return
	}
	result, err := a.admin.TestStorage(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, result)
}

func (a *adminHandler) listPolicies(c *gin.Context) {
	policies, err := a.admin.ListPolicies(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, policies)
}

func (a *adminHandler) createPolicy(c *gin.Context) {
	var patch service.PolicyPatch
	if !decode(c, &patch) {
		return
	}
	policy, err := a.admin.CreatePolicy(c.Request.Context(), patch)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 201, policy)
}

func (a *adminHandler) patchPolicy(c *gin.Context) {
	id, ok := adminID(c)
	if !ok {
		return
	}
	var patch service.PolicyPatch
	if !decode(c, &patch) {
		return
	}
	policy, err := a.admin.PatchPolicy(c.Request.Context(), id, patch)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, policy)
}

func (a *adminHandler) deletePolicy(c *gin.Context) {
	id, ok := adminID(c)
	if !ok {
		return
	}
	if err := a.admin.DeletePolicy(c.Request.Context(), id); err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, nil)
}

// previewPolicy never fails the request for a bad template; the diagnostic
// travels in the data payload so the console can render it next to the form.
func (a *adminHandler) previewPolicy(c *gin.Context) {
	var input struct {
		PathTpl string `json:"path_tpl"`
		NameTpl string `json:"name_tpl"`
	}
	if !decode(c, &input) {
		return
	}
	sample, err := a.admin.PreviewPolicy(c.Request.Context(), identity(c).User.ID, input.PathTpl, input.NameTpl)
	if err != nil {
		respond(c, 200, gin.H{"sample": "", "error": err.Error()})
		return
	}
	respond(c, 200, gin.H{"sample": sample, "error": ""})
}

func (a *adminHandler) getSettings(c *gin.Context) {
	settings, err := a.admin.GetSettings(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, settings)
}

func (a *adminHandler) putSettings(c *gin.Context) {
	var patch service.SettingsPatch
	if !decode(c, &patch) {
		return
	}
	settings, err := a.admin.PutSettings(c.Request.Context(), patch)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, settings)
}

func (a *adminHandler) listImages(c *gin.Context) {
	page, size, ok := a.adminPage(c, 20)
	if !ok {
		return
	}
	userID := uint64(0)
	if value, exists := c.GetQuery("user_id"); exists && value != "" {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			fail(c, service.ErrInvalidInput)
			return
		}
		userID = parsed
	}
	result, err := a.images.AdminList(c.Request.Context(), identity(c).Subject, service.AdminImageQuery{
		Page: page, Size: size, UserID: userID, Keyword: c.Query("keyword"),
	})
	if err != nil {
		fail(c, err)
		return
	}
	if result.Items == nil {
		result.Items = []service.ImageView{}
	}
	respond(c, 200, result)
}

func (a *adminHandler) deleteImage(c *gin.Context) {
	id, ok := adminID(c)
	if !ok {
		return
	}
	if err := a.images.AdminTrash(c.Request.Context(), identity(c).Subject, id); err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, nil)
}

// purgeAll empties the site-wide recycle bin image by image through the
// regular purge path. Partial failures keep their progress visible: the
// response carries the successful count alongside the failure envelope.
func (a *adminHandler) purgeAll(c *gin.Context) {
	purged, err := a.images.AdminPurgeAll(c.Request.Context(), identity(c).Subject)
	if err != nil {
		c.JSON(502, Response{Code: 50002, Message: "trash purge incomplete", Data: gin.H{"purged": purged}})
		return
	}
	respond(c, 200, gin.H{"purged": purged})
}
