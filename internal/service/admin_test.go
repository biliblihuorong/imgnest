package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/storage"
)

// ---------- fakes ----------

type adminUsersFake struct {
	users         map[uint64]model.User
	listed        []model.User
	total         int64
	statusChanges []adminStatusChange
	groupChanges  []adminGroupChange
}

type adminStatusChange struct {
	id     uint64
	status string
}
type adminGroupChange struct {
	id    uint64
	group uint64
}

func (f *adminUsersFake) FindUserByID(_ context.Context, id uint64) (model.User, error) {
	user, ok := f.users[id]
	if !ok {
		return model.User{}, ErrNotFound
	}
	return user, nil
}
func (f *adminUsersFake) ListUsers(context.Context, string, int, int) ([]model.User, int64, error) {
	return f.listed, f.total, nil
}
func (f *adminUsersFake) SetUserStatus(_ context.Context, id uint64, status string) error {
	f.statusChanges = append(f.statusChanges, adminStatusChange{id: id, status: status})
	user := f.users[id]
	user.Status = status
	f.users[id] = user
	return nil
}
func (f *adminUsersFake) SetUserGroup(_ context.Context, id, group uint64) error {
	f.groupChanges = append(f.groupChanges, adminGroupChange{id: id, group: group})
	user := f.users[id]
	user.GroupID = group
	f.users[id] = user
	return nil
}

func (f *adminUsersFake) CreateAdminUser(_ context.Context, _ uint64, user model.User) (model.User, error) {
	for id := range f.users {
		if id >= user.ID {
			user.ID = id + 1
		}
	}
	f.users[user.ID] = user
	return user, nil
}
func (f *adminUsersFake) UpdateAdminUser(ctx context.Context, _ uint64, id uint64, changes model.UserChanges) (model.User, error) {
	user, err := f.FindUserByID(ctx, id)
	if err != nil {
		return model.User{}, err
	}
	if changes.Username != nil {
		user.Username = *changes.Username
	}
	if changes.Email != nil {
		user.Email = *changes.Email
	}
	if changes.DisplayName != nil {
		user.DisplayName = *changes.DisplayName
	}
	if changes.Role != nil {
		user.Role = *changes.Role
	}
	if changes.Status != nil {
		user.Status = *changes.Status
		f.statusChanges = append(f.statusChanges, adminStatusChange{id: id, status: *changes.Status})
	}
	if changes.GroupID != nil {
		user.GroupID = *changes.GroupID
		f.groupChanges = append(f.groupChanges, adminGroupChange{id: id, group: *changes.GroupID})
	}
	f.users[id] = user
	return user, nil
}

type adminGroupsFake struct {
	groups   map[uint64]model.Group
	bindings map[uint64][]uint64
	counts   map[uint64]int64
	next     uint64
	created  []model.Group
	updates  []model.Group
	deleted  []uint64
}

func (f *adminGroupsFake) FindGroup(_ context.Context, id uint64) (model.Group, error) {
	group, ok := f.groups[id]
	if !ok {
		return model.Group{}, ErrNotFound
	}
	return group, nil
}
func (f *adminGroupsFake) ListGroups(context.Context) ([]model.Group, error) {
	rows := make([]model.Group, 0, len(f.groups))
	for _, group := range f.groups {
		rows = append(rows, group)
	}
	return rows, nil
}
func (f *adminGroupsFake) GroupMemberCounts(context.Context) (map[uint64]int64, error) {
	return f.counts, nil
}
func (f *adminGroupsFake) GroupPolicyIDs(context.Context) (map[uint64][]uint64, error) {
	return f.bindings, nil
}
func (f *adminGroupsFake) CreateGroup(_ context.Context, group model.Group, policyIDs []uint64) (model.Group, error) {
	f.next++
	group.ID = f.next
	f.groups[group.ID] = group
	f.bindings[group.ID] = policyIDs
	f.created = append(f.created, group)
	return group, nil
}
func (f *adminGroupsFake) UpdateGroup(_ context.Context, group model.Group, policyIDs []uint64, replace bool) (model.Group, error) {
	f.groups[group.ID] = group
	f.updates = append(f.updates, group)
	if replace {
		f.bindings[group.ID] = policyIDs
	}
	return group, nil
}
func (f *adminGroupsFake) DeleteGroup(_ context.Context, id uint64) error {
	delete(f.groups, id)
	delete(f.bindings, id)
	f.deleted = append(f.deleted, id)
	return nil
}

type adminRefsFake struct {
	storagePolicies int64
	policyImages    int64
	storageImages   int64
	policyDefaults  int64
	settingRefs     []string
}

func (f adminRefsFake) CountPoliciesForStorage(context.Context, uint64) (int64, error) {
	return f.storagePolicies, nil
}
func (f adminRefsFake) CountImagesForStorage(context.Context, uint64) (int64, error) {
	return f.storageImages, nil
}
func (f adminRefsFake) CountImagesForPolicy(context.Context, uint64) (int64, error) {
	return f.policyImages, nil
}
func (f adminRefsFake) CountGroupDefaultsForPolicy(context.Context, uint64) (int64, error) {
	return f.policyDefaults, nil
}
func (f adminRefsFake) SettingGroupReferences(context.Context, uint64) ([]string, error) {
	return f.settingRefs, nil
}

type adminStoragesFake struct {
	rows    map[uint64]model.Storage
	next    uint64
	created []model.Storage
	updates []model.Storage
	deleted []uint64
}

func (f *adminStoragesFake) Create(_ context.Context, value model.Storage) (model.Storage, error) {
	f.next++
	value.ID = f.next
	f.rows[value.ID] = value
	f.created = append(f.created, value)
	return value, nil
}
func (f *adminStoragesFake) Find(_ context.Context, id uint64) (model.Storage, error) {
	value, ok := f.rows[id]
	if !ok {
		return model.Storage{}, ErrNotFound
	}
	return value, nil
}
func (f *adminStoragesFake) List(context.Context) ([]model.Storage, error) {
	rows := make([]model.Storage, 0, len(f.rows))
	for _, value := range f.rows {
		rows = append(rows, value)
	}
	return rows, nil
}
func (f *adminStoragesFake) Update(_ context.Context, value model.Storage) (model.Storage, error) {
	if _, ok := f.rows[value.ID]; !ok {
		return model.Storage{}, ErrNotFound
	}
	f.rows[value.ID] = value
	f.updates = append(f.updates, value)
	return value, nil
}
func (f *adminStoragesFake) Delete(_ context.Context, id uint64) error {
	if _, ok := f.rows[id]; !ok {
		return ErrNotFound
	}
	delete(f.rows, id)
	f.deleted = append(f.deleted, id)
	return nil
}

type adminPoliciesFake struct {
	rows    map[uint64]model.Policy
	next    uint64
	updates []model.Policy
	deleted []uint64
}

func (f *adminPoliciesFake) Find(_ context.Context, id uint64) (model.Policy, error) {
	value, ok := f.rows[id]
	if !ok {
		return model.Policy{}, ErrNotFound
	}
	return value, nil
}
func (f *adminPoliciesFake) All(context.Context) ([]model.Policy, error) {
	rows := make([]model.Policy, 0, len(f.rows))
	for _, value := range f.rows {
		rows = append(rows, value)
	}
	return rows, nil
}
func (f *adminPoliciesFake) Create(_ context.Context, value model.Policy) (model.Policy, error) {
	f.next++
	value.ID = f.next
	f.rows[value.ID] = value
	return value, nil
}
func (f *adminPoliciesFake) Update(_ context.Context, value model.Policy) (model.Policy, error) {
	if _, ok := f.rows[value.ID]; !ok {
		return model.Policy{}, ErrNotFound
	}
	f.rows[value.ID] = value
	f.updates = append(f.updates, value)
	return value, nil
}
func (f *adminPoliciesFake) Delete(_ context.Context, id uint64) error {
	if _, ok := f.rows[id]; !ok {
		return ErrNotFound
	}
	delete(f.rows, id)
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *adminPoliciesFake) UploadPolicy(context.Context, uint64, uint64) (model.Policy, model.Storage, model.Group, error) {
	return model.Policy{}, model.Storage{}, model.Group{}, ErrNotFound
}
func (f *adminPoliciesFake) UploadGroup(context.Context, uint64) (model.Group, error) {
	return model.Group{}, ErrNotFound
}
func (f *adminPoliciesFake) CreateAndBind(_ context.Context, value model.Policy, _ uint64, _ bool) (model.Policy, error) {
	return f.Create(context.Background(), value)
}
func (f *adminPoliciesFake) GroupPolicies(context.Context, uint64) ([]model.Policy, error) {
	return []model.Policy{}, nil
}

type adminSettingsFake struct {
	values  map[string]json.RawMessage
	written []map[string]json.RawMessage
}

func (f *adminSettingsFake) ReadSettings(context.Context) (map[string]json.RawMessage, error) {
	out := make(map[string]json.RawMessage, len(f.values))
	for key, value := range f.values {
		out[key] = value
	}
	return out, nil
}
func (f *adminSettingsFake) UpdateSettings(_ context.Context, values map[string]json.RawMessage) error {
	f.written = append(f.written, values)
	return nil
}
func (f *adminSettingsFake) AvatarConfig(context.Context) (model.AvatarConfig, error) {
	return model.AvatarConfig{Provider: model.DefaultAvatarProvider}, nil
}

type adminCodecFake struct {
	drivers []string
	secrets []string
}

func (c *adminCodecFake) Seal(_ context.Context, driver string, plain []byte) (json.RawMessage, error) {
	c.drivers = append(c.drivers, driver)
	c.secrets = append(c.secrets, string(plain))
	return json.RawMessage(`{"version":1,"sealed":"cipher"}`), nil
}
func (c *adminCodecFake) Open(context.Context, string, json.RawMessage) ([]byte, error) {
	return nil, model.ErrInvalidInput
}

type adminTemplatesFake struct {
	err   error
	calls [][]string
}

func (t *adminTemplatesFake) Validate(_ context.Context, templates ...string) error {
	t.calls = append(t.calls, templates)
	return t.err
}

type adminProbeObject struct {
	data  []byte
	owner string
}

type adminProbeDriver struct {
	putErr, copyErr, deleteErr    error
	puts, copies, deletes, purges int
	imagePurges                   int
	objects                       map[string]adminProbeObject
}

func (d *adminProbeDriver) PutNew(_ context.Context, key string, body io.ReadSeeker, opts storage.PutOptions) (storage.Receipt, error) {
	d.puts++
	if d.putErr != nil {
		return storage.Receipt{}, d.putErr
	}
	if _, exists := d.objects[key]; exists {
		return storage.Receipt{}, storage.ErrExists
	}
	data, _ := io.ReadAll(body)
	if d.objects == nil {
		d.objects = map[string]adminProbeObject{}
	}
	d.objects[key] = adminProbeObject{data: data, owner: opts.OwnerID}
	return storage.Receipt{Key: key, OwnerID: opts.OwnerID, Size: int64(len(data))}, nil
}
func (d *adminProbeDriver) Copy(_ context.Context, source, target string, opts storage.CopyOptions) (storage.Receipt, error) {
	d.copies++
	if d.copyErr != nil {
		return storage.Receipt{}, d.copyErr
	}
	object, ok := d.objects[source]
	if !ok {
		return storage.Receipt{}, storage.ErrNotFound
	}
	d.objects[target] = adminProbeObject{data: object.data, owner: opts.OwnerID}
	return storage.Receipt{Key: target, OwnerID: opts.OwnerID, Size: int64(len(object.data))}, nil
}
func (d *adminProbeDriver) Open(_ context.Context, key string) (io.ReadCloser, storage.ObjectInfo, error) {
	object, ok := d.objects[key]
	if !ok {
		return nil, storage.ObjectInfo{}, storage.ErrNotFound
	}
	info := storage.ObjectInfo{OwnerID: object.owner, Size: int64(len(object.data)), MIME: "application/octet-stream"}
	return io.NopCloser(strings.NewReader(string(object.data))), info, nil
}
func (d *adminProbeDriver) Stat(_ context.Context, key string) (storage.ObjectInfo, error) {
	object, ok := d.objects[key]
	if !ok {
		return storage.ObjectInfo{}, storage.ErrNotFound
	}
	return storage.ObjectInfo{OwnerID: object.owner, Size: int64(len(object.data))}, nil
}
func (d *adminProbeDriver) DeleteCurrent(_ context.Context, key string) error {
	d.deletes++
	if d.deleteErr != nil {
		return d.deleteErr
	}
	delete(d.objects, key)
	return nil
}
func (d *adminProbeDriver) PurgeAllVersions(context.Context, string) error { return nil }
func (d *adminProbeDriver) PurgeOwned(_ context.Context, key string, owner string) error {
	d.purges++
	if object, ok := d.objects[key]; ok && object.owner != owner {
		return storage.ErrOwnership
	}
	delete(d.objects, key)
	return nil
}
func (d *adminProbeDriver) PurgeImage(_ context.Context, key string, owner string) error {
	d.purges++
	d.imagePurges++
	if object, ok := d.objects[key]; ok && object.owner != owner {
		return storage.ErrOwnership
	}
	delete(d.objects, key)
	return nil
}

type adminDriverProvider struct{ driver storage.Driver }

func (p adminDriverProvider) DriverFor(context.Context, model.Storage) (storage.Driver, error) {
	return p.driver, nil
}

// ---------- fixtures ----------

var adminTestNow = func() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) }

func adminServiceFixture(t *testing.T, deps AdminDependencies) (*AdminService, *adminUsersFake, *adminGroupsFake, *adminRefsFake, *adminStoragesFake, *adminPoliciesFake, *adminSettingsFake) {
	users := &adminUsersFake{users: map[uint64]model.User{
		7: {ID: 7, GroupID: 1, Username: "alice", Email: "alice@example.com", Role: model.UserRoleUser, Status: model.UserStatusEnabled},
		8: {ID: 8, GroupID: 1, Username: "bob", Email: "bob@example.com", Role: model.UserRoleUser, Status: model.UserStatusEnabled},
	}}
	groups := &adminGroupsFake{
		next: 10,
		groups: map[uint64]model.Group{
			1: {ID: 1, Name: "Default", IsDefault: true},
			2: {ID: 2, Name: "Guests", IsGuest: true},
			3: {ID: 3, Name: "Team"},
		},
		bindings: map[uint64][]uint64{},
		counts:   map[uint64]int64{1: 2},
	}
	refs := &adminRefsFake{}
	storages := &adminStoragesFake{rows: map[uint64]model.Storage{
		5: {ID: 5, Name: "cloud", Driver: "s3", Config: json.RawMessage(`{"version":1,"sealed":"cipher"}`), BaseURL: "https://cdn.test", Enabled: true},
		6: {ID: 6, Name: "disk", Driver: "local", Config: json.RawMessage(`{"root":"data"}`), BaseURL: "http://images.test/i/6", Enabled: true},
	}}
	policies := &adminPoliciesFake{rows: map[uint64]model.Policy{
		9: {ID: 9, StorageID: 5, Name: "alpha", PathTpl: "{Y}/{m}", NameTpl: "{uniqid}", WebPMode: "both", WebPQuality: 80, WebPEffort: 4, ThumbSize: 400, ScrubMode: "gps", LinkPrefer: "webp", HEIFMode: "webp_only", OnConflict: "rename", Enabled: true},
	}}
	settingsFake := &adminSettingsFake{values: map[string]json.RawMessage{}}
	if deps.Users == nil {
		deps.Users = users
	}
	if deps.Groups == nil {
		deps.Groups = groups
	}
	if deps.References == nil {
		deps.References = refs
	}
	if deps.Storages == nil {
		deps.Storages = storages
	}
	if deps.Policies == nil {
		deps.Policies = policies
	}
	if deps.Settings == nil {
		deps.Settings = settingsFake
	}
	if deps.Drivers == nil {
		deps.Drivers = adminDriverProvider{driver: &adminProbeDriver{}}
	}
	if deps.Secrets == nil {
		deps.Secrets = &adminCodecFake{}
	}
	if deps.Templates == nil {
		deps.Templates = &adminTemplatesFake{}
	}
	if deps.Provision == nil {
		provision, err := NewProvisionService(t.Context(), storages, policies, deps.Drivers, deps.Secrets, deps.Templates)
		if err != nil {
			panic(err)
		}
		deps.Provision = provision
	}
	if deps.Now == nil {
		deps.Now = adminTestNow
	}
	service, err := NewAdminService(t.Context(), deps)
	if err != nil {
		panic(err)
	}
	return service, users, groups, refs, storages, policies, settingsFake
}

// ---------- users ----------

func TestAdminPatchUserValidatesAndApplies(t *testing.T) {
	svc, users, groups, _, _, _, _ := adminServiceFixture(t, AdminDependencies{})
	disabled := model.UserStatusDisabled
	enabled := model.UserStatusEnabled
	if _, err := svc.PatchUser(t.Context(), 7, 8, AdminUserPatch{Status: &disabled}); err != nil {
		t.Fatal(err)
	}
	if len(users.statusChanges) != 1 || users.statusChanges[0].id != 8 || users.statusChanges[0].status != model.UserStatusDisabled {
		t.Fatalf("status changes=%+v", users.statusChanges)
	}

	invalid := "suspended"
	if _, err := svc.PatchUser(t.Context(), 7, 8, AdminUserPatch{Status: &invalid}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid status accepted: %v", err)
	}
	if _, err := svc.PatchUser(t.Context(), 7, 7, AdminUserPatch{Status: &disabled}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("self-disable accepted: %v", err)
	}
	zero := uint64(0)
	if _, err := svc.PatchUser(t.Context(), 7, 8, AdminUserPatch{GroupID: &zero}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero group accepted: %v", err)
	}
	missing := uint64(99)
	if _, err := svc.PatchUser(t.Context(), 7, 8, AdminUserPatch{GroupID: &missing}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing group accepted: %v", err)
	}
	guest := uint64(2)
	if _, err := svc.PatchUser(t.Context(), 7, 8, AdminUserPatch{GroupID: &guest}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("guest group accepted: %v", err)
	}
	if _, err := svc.PatchUser(t.Context(), 7, 99, AdminUserPatch{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user accepted: %v", err)
	}
	team := uint64(3)
	view, err := svc.PatchUser(t.Context(), 7, 8, AdminUserPatch{Status: &enabled, GroupID: &team})
	if err != nil {
		t.Fatal(err)
	}
	if view.ID != 8 || view.GroupID != 3 || view.Status != model.UserStatusEnabled {
		t.Fatalf("patched view=%+v", view)
	}
	if len(users.groupChanges) != 1 || users.groupChanges[0].group != 3 {
		t.Fatalf("group changes=%+v", users.groupChanges)
	}
	if len(users.statusChanges) != 2 {
		t.Fatalf("status changes=%+v", users.statusChanges)
	}
	if _, err := svc.PatchUser(t.Context(), 7, 8, AdminUserPatch{}); err != nil {
		t.Fatalf("empty patch rejected: %v", err)
	}
	if len(users.statusChanges) != 2 || len(users.groupChanges) != 1 {
		t.Fatalf("empty patch mutated state: %+v %+v", users.statusChanges, users.groupChanges)
	}
	if len(groups.deleted) != 0 {
		t.Fatalf("user patch touched groups: %+v", groups.deleted)
	}
}

func TestAdminListUsersRejectsBadPaging(t *testing.T) {
	svc, users, _, _, _, _, _ := adminServiceFixture(t, AdminDependencies{})
	users.listed = []model.User{users.users[7]}
	users.total = 1
	page, err := svc.ListUsers(t.Context(), 1, 20, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Page != 1 || page.Size != 20 {
		t.Fatalf("page=%+v", page)
	}
	raw, err := json.Marshal(page.Items)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"group_id"`) || !strings.Contains(string(raw), `"used_bytes"`) {
		t.Fatalf("admin user view fields=%s", raw)
	}
	if _, err := svc.ListUsers(t.Context(), 0, 20, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero page accepted: %v", err)
	}
	if _, err := svc.ListUsers(t.Context(), 1, 0, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero size accepted: %v", err)
	}
	if _, err := svc.ListUsers(t.Context(), 1, 101, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("oversized page accepted: %v", err)
	}
}

// ---------- groups ----------

func TestAdminCreateGroupValidation(t *testing.T) {
	svc, _, groups, _, _, _, _ := adminServiceFixture(t, AdminDependencies{})
	_, err := svc.CreateGroup(t.Context(), GroupInput{Name: "  "})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank name accepted: %v", err)
	}
	if _, err := svc.CreateGroup(t.Context(), GroupInput{Name: strings.Repeat("图", 65)}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("overlong name accepted: %v", err)
	}
	if _, err := svc.CreateGroup(t.Context(), GroupInput{Name: "Team", DefaultPolicyID: 9}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("default outside bindings accepted: %v", err)
	}
	if _, err := svc.CreateGroup(t.Context(), GroupInput{Name: "Team", PolicyIDs: []uint64{77}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing policy accepted: %v", err)
	}
	if _, err := svc.CreateGroup(t.Context(), GroupInput{Name: "Second", IsDefault: true}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("second default group accepted: %v", err)
	}
	if _, err := svc.CreateGroup(t.Context(), GroupInput{Name: "SecondGuest", IsGuest: true}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("second guest group accepted: %v", err)
	}
	view, err := svc.CreateGroup(t.Context(), GroupInput{
		Name: "Team", CapacityBytes: 1024, MaxFileBytes: 64, AllowedExts: []string{"png"},
		UploadPerMin: 5, DefaultPolicyID: 9, PolicyIDs: []uint64{9},
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Name != "Team" || view.CapacityBytes != 1024 || view.UserCount != 0 || len(view.PolicyIDs) != 1 || view.PolicyIDs[0] != 9 {
		t.Fatalf("created view=%+v", view)
	}
	if len(groups.created) != 1 || len(groups.created[0].AllowedExts) != 1 {
		t.Fatalf("created group=%+v", groups.created)
	}
}

func TestAdminGroupDeleteRules(t *testing.T) {
	svc, _, groups, refs, _, _, _ := adminServiceFixture(t, AdminDependencies{})
	if err := svc.DeleteGroup(t.Context(), 1); !errors.Is(err, ErrForbidden) {
		t.Fatalf("default group deleted: %v", err)
	}
	if err := svc.DeleteGroup(t.Context(), 2); !errors.Is(err, ErrForbidden) {
		t.Fatalf("guest group deleted: %v", err)
	}
	if err := svc.DeleteGroup(t.Context(), 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing group delete error=%v", err)
	}
	if len(groups.deleted) != 0 {
		t.Fatalf("protected groups reached deletion: %+v", groups.deleted)
	}
	team := groups.groups[3]
	team.ID = 4
	groups.groups[4] = team
	groups.counts[4] = 2
	if err := svc.DeleteGroup(t.Context(), 4); !errors.Is(err, ErrGroupHasMembers) {
		t.Fatalf("member group deleted: %v", err)
	}
	groups.counts[4] = 0
	refs.settingRefs = []string{"guest_group_id"}
	if err := svc.DeleteGroup(t.Context(), 4); !errors.Is(err, ErrStillReferenced) {
		t.Fatalf("settings-referenced group deleted: %v", err)
	}
	refs.settingRefs = nil
	if err := svc.DeleteGroup(t.Context(), 4); err != nil {
		t.Fatal(err)
	}
	if len(groups.deleted) != 1 || groups.deleted[0] != 4 {
		t.Fatalf("deletions=%+v", groups.deleted)
	}
}

func TestAdminPatchGroupMergesAndValidates(t *testing.T) {
	svc, _, groups, _, _, _, _ := adminServiceFixture(t, AdminDependencies{})
	name := "Renamed"
	capacity := int64(4096)
	view, err := svc.PatchGroup(t.Context(), 3, GroupPatch{Name: &name, CapacityBytes: &capacity})
	if err != nil {
		t.Fatal(err)
	}
	if view.Name != "Renamed" || view.CapacityBytes != 4096 {
		t.Fatalf("patched view=%+v", view)
	}
	if len(groups.updates) != 1 {
		t.Fatalf("updates=%+v", groups.updates)
	}
	negative := int64(-1)
	if _, err := svc.PatchGroup(t.Context(), 3, GroupPatch{CapacityBytes: &negative}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("negative capacity accepted: %v", err)
	}
	missing := uint64(77)
	if _, err := svc.PatchGroup(t.Context(), 3, GroupPatch{DefaultPolicyID: &missing}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing default policy accepted: %v", err)
	}
	orphan := uint64(9)
	if _, err := svc.PatchGroup(t.Context(), 3, GroupPatch{DefaultPolicyID: &orphan}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("default outside bindings accepted: %v", err)
	}
	bound := []uint64{9}
	if _, err := svc.PatchGroup(t.Context(), 3, GroupPatch{PolicyIDs: &bound, DefaultPolicyID: &orphan}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PatchGroup(t.Context(), 99, GroupPatch{Name: &name}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing group patch error=%v", err)
	}
}

// ---------- storages ----------

func TestAdminCreateStorageEncryptsAndHidesSecrets(t *testing.T) {
	svc, _, _, _, storages, policies, _ := adminServiceFixture(t, AdminDependencies{})
	codec := &adminCodecFake{}
	templates := &adminTemplatesFake{}
	svc.deps.Secrets = codec
	svc.deps.Templates = templates
	provision, provisionErr := NewProvisionService(t.Context(), storages, policies, svc.deps.Drivers, codec, templates)
	if provisionErr != nil {
		t.Fatal(provisionErr)
	}
	svc.deps.Provision = provision

	view, err := svc.CreateStorage(t.Context(), StorageInput{
		Name: "cloud", Driver: "s3", BaseURL: "https://cdn.test",
		Config: json.RawMessage(`{"endpoint":"https://s3.test","secret_access_key":"SUPER-SECRET-VALUE"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.ID == 0 || view.Driver != "s3" {
		t.Fatalf("created view=%+v", view)
	}
	if len(codec.drivers) != 1 || codec.drivers[0] != "s3" {
		t.Fatalf("s3 config was not sealed through the codec: %+v", codec.drivers)
	}
	stored := storages.created[0]
	if string(stored.Config) == "" || strings.Contains(string(stored.Config), "SUPER-SECRET-VALUE") {
		t.Fatalf("plaintext credential reached persistence: %s", stored.Config)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "config") || strings.Contains(string(raw), "SUPER-SECRET-VALUE") {
		t.Fatalf("storage view leaked configuration: %s", raw)
	}
}

func TestAdminPatchStorageMergesAndReseals(t *testing.T) {
	svc, _, _, _, storages, _, _ := adminServiceFixture(t, AdminDependencies{})
	codec := svc.deps.Secrets.(*adminCodecFake)
	enabled := false
	view, err := svc.PatchStorage(t.Context(), 5, StoragePatch{Enabled: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	if view.Enabled {
		t.Fatalf("disable ignored: %+v", view)
	}
	name := "renamed"
	secretConfig := json.RawMessage(`{"endpoint":"https://s3.test","secret_access_key":"ROTATED-SECRET-VALUE"}`)
	if _, err = svc.PatchStorage(t.Context(), 5, StoragePatch{Name: &name, Config: secretConfig}); err != nil {
		t.Fatal(err)
	}
	if len(codec.drivers) != 1 || codec.drivers[0] != "s3" || codec.secrets[0] != string(secretConfig) {
		t.Fatalf("s3 config not resealed: %+v", codec.drivers)
	}
	if string(storages.updates[len(storages.updates)-1].Config) != `{"version":1,"sealed":"cipher"}` {
		t.Fatalf("sealed envelope not persisted: %s", storages.updates[len(storages.updates)-1].Config)
	}
	rawConfig := json.RawMessage(`{"root":"elsewhere"}`)
	if _, err := svc.PatchStorage(t.Context(), 6, StoragePatch{Config: rawConfig}); err != nil {
		t.Fatal(err)
	}
	// Local driver configuration is not secret material and stays verbatim.
	if string(storages.updates[len(storages.updates)-1].Config) != `{"root":"elsewhere"}` {
		t.Fatalf("local config changed: %s", storages.updates[len(storages.updates)-1].Config)
	}
	bad := "ftp://cdn.test"
	if _, err := svc.PatchStorage(t.Context(), 5, StoragePatch{BaseURL: &bad}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid base URL accepted: %v", err)
	}
	empty := " "
	if _, err := svc.PatchStorage(t.Context(), 5, StoragePatch{Name: &empty}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank name accepted: %v", err)
	}
	broken := json.RawMessage(`{broken`)
	if _, err := svc.PatchStorage(t.Context(), 5, StoragePatch{Config: broken}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("broken config accepted: %v", err)
	}
	if _, err := svc.PatchStorage(t.Context(), 99, StoragePatch{Name: &name}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing storage patch error=%v", err)
	}
	if _, err := svc.ListStorages(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestAdminDeleteStorageChecksReferences(t *testing.T) {
	svc, _, _, refs, storages, _, _ := adminServiceFixture(t, AdminDependencies{})
	refs.storagePolicies = 2
	if err := svc.DeleteStorage(t.Context(), 5); !errors.Is(err, ErrStillReferenced) {
		t.Fatalf("referenced storage deleted: %v", err)
	}
	if len(storages.deleted) != 0 {
		t.Fatalf("delete executed anyway: %+v", storages.deleted)
	}
	refs.storagePolicies = 0
	if err := svc.DeleteStorage(t.Context(), 5); err != nil {
		t.Fatal(err)
	}
	if len(storages.deleted) != 1 || storages.deleted[0] != 5 {
		t.Fatalf("deletions=%+v", storages.deleted)
	}
	if err := svc.DeleteStorage(t.Context(), 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing storage delete error=%v", err)
	}
}

func TestAdminStorageTestReportsEachCheck(t *testing.T) {
	svc, _, _, _, _, _, _ := adminServiceFixture(t, AdminDependencies{})
	driver := &adminProbeDriver{}
	svc.deps.Drivers = adminDriverProvider{driver: driver}
	result, err := svc.TestStorage(t.Context(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || !result.Checks.Put || !result.Checks.Copy || !result.Checks.Delete {
		t.Fatalf("healthy probe=%+v", result)
	}
	if driver.purges < 2 {
		t.Fatalf("probe left objects behind: purges=%d", driver.purges)
	}
	if driver.imagePurges < 2 {
		t.Fatalf("probe kept delete markers: full-history purges=%d", driver.imagePurges)
	}

	driver = &adminProbeDriver{copyErr: storage.ErrExists}
	svc.deps.Drivers = adminDriverProvider{driver: driver}
	result, err = svc.TestStorage(t.Context(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || !result.Checks.Put || result.Checks.Copy || !result.Checks.Delete {
		t.Fatalf("copy-failure probe=%+v", result)
	}

	driver = &adminProbeDriver{putErr: storage.ErrOwnership}
	svc.deps.Drivers = adminDriverProvider{driver: driver}
	result, err = svc.TestStorage(t.Context(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Checks.Put || result.Checks.Copy || result.Checks.Delete {
		t.Fatalf("put-failure probe=%+v", result)
	}
	if _, err := svc.TestStorage(t.Context(), 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing storage test error=%v", err)
	}
}

// ---------- policies ----------

func TestAdminCreatePolicyAppliesDefaults(t *testing.T) {
	svc, _, _, _, storages, _, _ := adminServiceFixture(t, AdminDependencies{})
	templates := svc.deps.Templates.(*adminTemplatesFake)
	created, err := svc.CreatePolicy(t.Context(), PolicyPatch{
		Name: ptr("alpha"), StorageID: ptr(uint64(6)), PathTpl: ptr("{Y}/{m}"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Enabled != true || created.WebPQuality != 80 || created.WebPEffort != 4 || created.ThumbSize != 400 ||
		created.WebPMode != "both" || created.ScrubMode != "gps" || created.LinkPrefer != "webp" ||
		created.HEIFMode != "webp_only" || created.OnConflict != "rename" || created.NameTpl != "{uniqid}" ||
		!created.StripMeta || !created.SkipIfLarger || !created.ThumbEnabled {
		t.Fatalf("defaults missing: %+v", created)
	}
	if created.StorageID != 6 || created.PathTpl != "{Y}/{m}" {
		t.Fatalf("overrides missing: %+v", created)
	}
	if len(templates.calls) == 0 {
		t.Fatal("path templates were not validated")
	}
	if _, err := svc.CreatePolicy(t.Context(), PolicyPatch{Name: ptr("x")}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing storage accepted: %v", err)
	}
	missingStorage := uint64(42)
	if _, err := svc.CreatePolicy(t.Context(), PolicyPatch{Name: ptr("x"), StorageID: &missingStorage}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown storage accepted: %v", err)
	}
	templates.err = errors.New("bad template")
	if _, err := svc.CreatePolicy(t.Context(), PolicyPatch{Name: ptr("x"), StorageID: ptr(uint64(6))}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid template accepted: %v", err)
	}
	templates.err = nil
	if _, err := svc.CreatePolicy(t.Context(), PolicyPatch{Name: ptr(" ")}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank name accepted: %v", err)
	}
	if _, err := svc.ListPolicies(t.Context()); err != nil {
		t.Fatal(err)
	}
	if storages.updatedAnything() {
		t.Fatal("policy create touched storages")
	}
}

func TestAdminPatchPolicyMergesFields(t *testing.T) {
	svc, _, _, _, _, policies, _ := adminServiceFixture(t, AdminDependencies{})
	quality := 95
	updated, err := svc.PatchPolicy(t.Context(), 9, PolicyPatch{WebPQuality: &quality})
	if err != nil {
		t.Fatal(err)
	}
	if updated.WebPQuality != 95 || updated.Name != "alpha" || updated.PathTpl != "{Y}/{m}" {
		t.Fatalf("merged policy=%+v", updated)
	}
	disabled := false
	if _, err := svc.PatchPolicy(t.Context(), 9, PolicyPatch{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if policies.rows[9].Enabled {
		t.Fatal("disable ignored")
	}
	badMode := "raw"
	if _, err := svc.PatchPolicy(t.Context(), 9, PolicyPatch{WebPMode: &badMode}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid mode accepted: %v", err)
	}
	zeroQuality := 0
	if _, err := svc.PatchPolicy(t.Context(), 9, PolicyPatch{WebPQuality: &zeroQuality}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero quality accepted: %v", err)
	}
	if _, err := svc.PatchPolicy(t.Context(), 42, PolicyPatch{WebPQuality: &quality}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing policy patch error=%v", err)
	}
}

func TestAdminDeletePolicyChecksReferences(t *testing.T) {
	svc, _, _, refs, _, policies, _ := adminServiceFixture(t, AdminDependencies{})
	refs.policyImages = 3
	if err := svc.DeletePolicy(t.Context(), 9); !errors.Is(err, ErrStillReferenced) {
		t.Fatalf("image-referenced policy deleted: %v", err)
	}
	refs.policyImages = 0
	refs.policyDefaults = 1
	if err := svc.DeletePolicy(t.Context(), 9); !errors.Is(err, ErrStillReferenced) {
		t.Fatalf("group-default policy deleted: %v", err)
	}
	refs.policyDefaults = 0
	if err := svc.DeletePolicy(t.Context(), 9); err != nil {
		t.Fatal(err)
	}
	if len(policies.deleted) != 1 || policies.deleted[0] != 9 {
		t.Fatalf("deletions=%+v", policies.deleted)
	}
	if err := svc.DeletePolicy(t.Context(), 42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing policy delete error=%v", err)
	}
}

func TestAdminPolicyPreviewRendersSample(t *testing.T) {
	svc, _, _, _, _, _, _ := adminServiceFixture(t, AdminDependencies{})
	sample, err := svc.PreviewPolicy(t.Context(), 7, "{Y}/{m}/{d}", "{filename}")
	if err != nil {
		t.Fatal(err)
	}
	if sample != "2026/10/04/example" {
		t.Fatalf("sample=%q", sample)
	}
	sample, err = svc.PreviewPolicy(t.Context(), 7, "{uid}", "{uniqid}")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sample, "7/") {
		t.Fatalf("uid sample=%q", sample)
	}
	if _, err = svc.PreviewPolicy(t.Context(), 7, "{broken}", "{filename}"); err == nil {
		t.Fatal("invalid template accepted")
	}
	if _, err = svc.PreviewPolicy(t.Context(), 7, "{Y}", "{Y}/{filename}"); err == nil {
		t.Fatal("directory name template accepted")
	}
}

// ---------- settings ----------

func TestAdminSettingsDefaultsAndRoundTrip(t *testing.T) {
	svc, _, groups, _, _, _, settingsFake := adminServiceFixture(t, AdminDependencies{})
	view, err := svc.GetSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	expected := AdminSettingsView{
		SiteName: DefaultSiteName, TrashDays: 7, APIEnabled: true, AvatarProvider: model.DefaultAvatarProvider,
	}
	if view != expected {
		t.Fatalf("defaults view=%+v want %+v", view, expected)
	}
	settingsFake.values = map[string]json.RawMessage{
		"site_name":            json.RawMessage(`"My Nest"`),
		"registration_enabled": json.RawMessage(`true`),
		"guest_upload_enabled": json.RawMessage(`true`),
		"gallery_enabled":      json.RawMessage(`true`),
		"trash_days":           json.RawMessage(`3`),
		"api_enabled":          json.RawMessage(`false`),
		"guest_group_id":       json.RawMessage(`2`),
		"default_group_id":     json.RawMessage(`1`),
	}
	view, err = svc.GetSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if view.SiteName != "My Nest" || !view.RegistrationEnabled || !view.GuestUploadEnabled || !view.GalleryEnabled ||
		view.TrashDays != 3 || view.APIEnabled || view.GuestGroupID != 2 || view.DefaultGroupID != 1 {
		t.Fatalf("stored view=%+v", view)
	}

	name := "Changed"
	if _, err := svc.PutSettings(t.Context(), SettingsPatch{SiteName: &name}); err != nil {
		t.Fatal(err)
	}
	if len(settingsFake.written) != 1 || string(settingsFake.written[0]["site_name"]) != `"Changed"` {
		t.Fatalf("written=%+v", settingsFake.written)
	}
	if len(settingsFake.written[0]) != 1 {
		t.Fatalf("partial write touched %d keys", len(settingsFake.written[0]))
	}
	blank := "   "
	if _, err := svc.PutSettings(t.Context(), SettingsPatch{SiteName: &blank}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank site name accepted: %v", err)
	}
	tooLong := strings.Repeat("图", 101)
	if _, err := svc.PutSettings(t.Context(), SettingsPatch{SiteName: &tooLong}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("overlong site name accepted: %v", err)
	}
	negative := -1
	if _, err := svc.PutSettings(t.Context(), SettingsPatch{TrashDays: &negative}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("negative trash days accepted: %v", err)
	}
	huge := 36501
	if _, err := svc.PutSettings(t.Context(), SettingsPatch{TrashDays: &huge}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("oversized trash days accepted: %v", err)
	}
	zeroGroup := uint64(0)
	if _, err := svc.PutSettings(t.Context(), SettingsPatch{DefaultGroupID: &zeroGroup}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero default group accepted: %v", err)
	}
	missingGroup := uint64(99)
	if _, err := svc.PutSettings(t.Context(), SettingsPatch{DefaultGroupID: &missingGroup}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing default group accepted: %v", err)
	}
	if _, err := svc.PutSettings(t.Context(), SettingsPatch{GuestGroupID: &missingGroup}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing guest group accepted: %v", err)
	}
	days := 0
	guest := uint64(2)
	if _, err := svc.PutSettings(t.Context(), SettingsPatch{TrashDays: &days, GuestGroupID: &guest}); err != nil {
		t.Fatal(err)
	}
	if len(groups.updates) != 0 {
		t.Fatal("settings write mutated groups")
	}
	ordinary, guestGroup := uint64(1), uint64(2)
	if _, err := svc.PutSettings(t.Context(), SettingsPatch{DefaultGroupID: &guestGroup}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("guest group accepted as default group: %v", err)
	}
	if _, err := svc.PutSettings(t.Context(), SettingsPatch{GuestGroupID: &ordinary}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ordinary group accepted as guest group: %v", err)
	}
	albumsOnly := true
	if _, err := svc.PutSettings(t.Context(), SettingsPatch{GalleryPublicAlbumsOnly: &albumsOnly}); err != nil {
		t.Fatal(err)
	}
	last := settingsFake.written[len(settingsFake.written)-1]
	if string(last["gallery_public_albums_only"]) != "true" {
		t.Fatalf("gallery album scope not written: %+v", last)
	}
}

// ---------- admin image operations ----------

type adminImageStoreStub struct {
	ImageRepository
	rows       []model.Image
	trash      []model.Image
	total      int64
	keys       []string
	gotUserID  uint64
	gotKeyword string
	gotPage    int
	gotSize    int
	purged     []string
	finishErr  error
}

func (s *adminImageStoreStub) ListAdminPage(_ context.Context, userID uint64, keyword string, page, size int) ([]model.Image, int64, error) {
	s.gotUserID, s.gotKeyword, s.gotPage, s.gotSize = userID, keyword, page, size
	return s.rows, s.total, nil
}
func (s *adminImageStoreStub) TrashKeys(context.Context) ([]string, error) { return s.keys, nil }
func (s *adminImageStoreStub) find(key string) (model.Image, bool) {
	for _, row := range append(append([]model.Image{}, s.rows...), s.trash...) {
		if row.Key == key {
			return row, true
		}
	}
	return model.Image{}, false
}
func (s *adminImageStoreStub) FindByID(_ context.Context, id uint64) (model.Image, error) {
	for _, row := range append(append([]model.Image{}, s.rows...), s.trash...) {
		if row.ID == id {
			return row, nil
		}
	}
	return model.Image{}, ErrNotFound
}
func (s *adminImageStoreStub) FindByKey(_ context.Context, key string) (model.Image, error) {
	if row, ok := s.find(key); ok {
		return row, nil
	}
	return model.Image{}, ErrNotFound
}
func (s *adminImageStoreStub) BeginPurge(_ context.Context, key, _ string, _ model.TokenGrant) (model.Image, error) {
	if row, ok := s.find(key); ok {
		row.Operation = model.ImageOperationPurge
		return row, nil
	}
	return model.Image{}, ErrNotFound
}
func (s *adminImageStoreStub) FinishPurge(_ context.Context, key, _ string) error {
	if s.finishErr != nil {
		return s.finishErr
	}
	s.purged = append(s.purged, key)
	return nil
}

func adminImageFixture(t *testing.T, role string) (*ImageService, *adminImageStoreStub, TokenSubject) {
	t.Helper()
	svc, _, _, _, subject := uploadFixture(t, "png")
	store := &adminImageStoreStub{
		rows: []model.Image{
			{ID: 11, UserID: 7, PolicyID: 1, StorageID: 1, Key: "k1", Path: "2026/10/04/a", Ext: "png", MIME: "image/png", State: model.ImageStateActive, Frames: 1},
			{ID: 12, UserID: 8, PolicyID: 1, StorageID: 1, Key: "k2", Path: "2026/10/04/b", Ext: "png", MIME: "image/png", State: model.ImageStateActive, Frames: 1},
		},
		total: 2,
		keys:  []string{"t1", "t2"},
		trash: []model.Image{
			{ID: 21, UserID: 8, PolicyID: 1, StorageID: 1, Key: "t1", Path: "2026/10/04/t1", Ext: "png", MIME: "image/png", State: model.ImageStateTrash, Frames: 1},
			{ID: 22, UserID: 8, PolicyID: 1, StorageID: 1, Key: "t2", Path: "2026/10/04/t2", Ext: "png", MIME: "image/png", State: model.ImageStateTrash, Frames: 1},
		},
	}
	svc.deps.Images = store
	svc.deps.Users = uploadUserRepo{user: model.User{ID: subject.userID, GroupID: 1, PasswordHash: "verified", Status: model.UserStatusEnabled, Role: role}}
	return svc, store, subject
}

func TestAdminListImagesRequiresAdminRole(t *testing.T) {
	svc, store, subject := adminImageFixture(t, model.UserRoleUser)
	if _, err := svc.AdminList(t.Context(), subject, AdminImageQuery{Page: 1, Size: 20}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-admin listing accepted: %v", err)
	}
	svc.deps.Users = uploadUserRepo{user: model.User{ID: subject.userID, GroupID: 1, PasswordHash: "verified", Status: model.UserStatusEnabled, Role: model.UserRoleAdmin}}
	page, err := svc.AdminList(t.Context(), subject, AdminImageQuery{Page: 2, Size: 10, UserID: 7, Keyword: "sun"})
	if err != nil {
		t.Fatal(err)
	}
	if store.gotUserID != 7 || store.gotKeyword != "sun" || store.gotPage != 2 || store.gotSize != 10 {
		t.Fatalf("filters not passed: %+v", store)
	}
	if page.Total != 2 || len(page.Items) != 2 || page.Page != 2 || page.Size != 10 {
		t.Fatalf("page=%+v", page)
	}
	// Zero values fall back to the shared listing defaults, mirroring List.
	if _, err := svc.AdminList(t.Context(), subject, AdminImageQuery{}); err != nil {
		t.Fatalf("defaults rejected: %v", err)
	}
	if store.gotPage != 1 || store.gotSize != 20 {
		t.Fatalf("defaults page=%d size=%d", store.gotPage, store.gotSize)
	}
	if _, err := svc.AdminList(t.Context(), subject, AdminImageQuery{Page: 1, Size: 101}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("oversized page accepted: %v", err)
	}
}

func TestAdminTrashAllowsAnyOwnerForAdmin(t *testing.T) {
	svc, store, subject := adminImageFixture(t, model.UserRoleUser)
	if err := svc.AdminTrash(t.Context(), subject, 12); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-admin cross-owner delete accepted: %v", err)
	}
	svc.deps.Users = uploadUserRepo{user: model.User{ID: subject.userID, GroupID: 1, PasswordHash: "verified", Status: model.UserStatusEnabled, Role: model.UserRoleAdmin}}
	// An already-recycled idle image short-circuits before any storage IO.
	store.rows = []model.Image{{ID: 12, UserID: 8, PolicyID: 1, StorageID: 1, Key: "k2", Path: "2026/10/04/b", Ext: "png", MIME: "image/png", State: model.ImageStateTrash, Frames: 1}}
	if err := svc.AdminTrash(t.Context(), subject, 12); err != nil {
		t.Fatalf("admin cross-owner trash failed: %v", err)
	}
	if err := svc.AdminTrash(t.Context(), subject, 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing image trash error=%v", err)
	}
}

func TestAdminPurgeAllCountsAndReportsFailures(t *testing.T) {
	svc, store, subject := adminImageFixture(t, model.UserRoleUser)
	if _, err := svc.AdminPurgeAll(t.Context(), subject); err == nil {
		t.Fatal("non-admin purge accepted")
	}
	svc.deps.Users = uploadUserRepo{user: model.User{ID: subject.userID, GroupID: 1, PasswordHash: "verified", Status: model.UserStatusEnabled, Role: model.UserRoleAdmin}}
	purged, err := svc.AdminPurgeAll(t.Context(), subject)
	if err != nil {
		t.Fatal(err)
	}
	if purged != 2 || len(store.purged) != 2 {
		t.Fatalf("purged=%d recorded=%+v", purged, store.purged)
	}
	store.finishErr = ErrStorage
	purged, err = svc.AdminPurgeAll(t.Context(), subject)
	if err == nil || purged != 0 {
		t.Fatalf("failure reporting purged=%d err=%v", purged, err)
	}
}

// ---------- helpers ----------

func ptr[T any](value T) *T { return &value }

func (f *adminStoragesFake) updatedAnything() bool { return len(f.updates) != 0 || len(f.deleted) != 0 }

func TestAdminPatchStorageKeepsLocationOnceImagesExist(t *testing.T) {
	svc, _, _, refs, storages, _, _ := adminServiceFixture(t, AdminDependencies{})
	refs.storageImages = 3
	moved := json.RawMessage(`{"root":"elsewhere"}`)
	if _, err := svc.PatchStorage(t.Context(), 6, StoragePatch{Config: moved}); !errors.Is(err, ErrStillReferenced) {
		t.Fatalf("moving a local storage with images = %v, want still referenced", err)
	}
	if len(storages.updates) != 0 {
		t.Fatal("refused move reached persistence")
	}
	// The same directory spelled differently is not a move.
	same := json.RawMessage(`{"root":"./data/"}`)
	if _, err := svc.PatchStorage(t.Context(), 6, StoragePatch{Config: same}); err != nil {
		t.Fatalf("unchanged local root refused: %v", err)
	}
	// The stored S3 envelope cannot be opened by the fake codec, so the move
	// check fails closed rather than guessing.
	cloud := json.RawMessage(`{"endpoint":"https://s3.test","bucket":"other","secret_access_key":"x"}`)
	if _, err := svc.PatchStorage(t.Context(), 5, StoragePatch{Config: cloud}); !errors.Is(err, ErrStillReferenced) {
		t.Fatalf("unverifiable S3 change = %v, want still referenced", err)
	}
	refs.storageImages = 0
	if _, err := svc.PatchStorage(t.Context(), 6, StoragePatch{Config: moved}); err != nil {
		t.Fatalf("empty storage move refused: %v", err)
	}
}
