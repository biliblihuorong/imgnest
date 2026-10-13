package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/secret"
)

func TestPluginSettingsStatusOnlyCardHasArrayFields(t *testing.T) {
	plugin := &pluginFake{name: "status", schema: PluginSchema{Title: "Status only"}}
	store := &pluginStoreFake{values: map[string]json.RawMessage{}}
	svc, err := NewPluginSettingsService(t.Context(), store, nil, []ConfigurablePlugin{plugin})
	if err != nil {
		t.Fatal(err)
	}
	views, err := svc.List(t.Context())
	if err != nil || len(views) != 1 {
		t.Fatalf("list status card: %v", err)
	}
	saved, err := svc.Save(t.Context(), "status", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []PluginSettingsView{views[0], saved} {
		body, err := json.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]json.RawMessage
		if err := json.Unmarshal(body, &wire); err != nil {
			t.Fatal(err)
		}
		if string(wire["fields"]) != "[]" {
			t.Fatalf("status-only card fields=%s, want an array for the console", wire["fields"])
		}
	}
}

type pluginStoreFake struct {
	values       map[string]json.RawMessage
	fail         error
	beforeUpdate func(context.Context) error
}

func (f *pluginStoreFake) ReadSettings(context.Context) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	for key, value := range f.values {
		out[key] = value
	}
	return out, nil
}

func (f *pluginStoreFake) UpdateSettings(ctx context.Context, values map[string]json.RawMessage) error {
	if f.beforeUpdate != nil {
		if err := f.beforeUpdate(ctx); err != nil {
			return err
		}
	}
	if f.fail != nil {
		return f.fail
	}
	for key, value := range values {
		f.values[key] = value
	}
	return nil
}

type pluginFake struct {
	name    string
	schema  PluginSchema
	hidden  bool
	refuse  error
	applied []string
}

func (p *pluginFake) Name() string { return p.name }
func (p *pluginFake) Schema(context.Context) (PluginSchema, bool) {
	return p.schema, !p.hidden
}
func (p *pluginFake) Apply(_ context.Context, values json.RawMessage) error {
	if p.refuse != nil {
		return p.refuse
	}
	p.applied = append(p.applied, string(values))
	return nil
}
func (p *pluginFake) Status(context.Context) []PluginStatus {
	return []PluginStatus{{Label: "License", Value: "valid", Level: "success"}}
}

func floatPtr(v float64) *float64 { return &v }

func webhookSchema() PluginSchema {
	return PluginSchema{Title: "Webhook", Fields: []PluginField{
		{Key: "enabled", Label: "启用", Type: PluginFieldBool},
		{Key: "token", Label: "令牌", Type: PluginFieldSecret},
		{Key: "opacity", Label: "透明度", Type: PluginFieldNumber, Default: 0.6, Min: floatPtr(0), Max: floatPtr(1)},
		{Key: "position", Label: "位置", Type: PluginFieldSelect, Default: "br", Options: []PluginOption{{Label: "右下", Value: "br"}, {Label: "左上", Value: "tl"}}},
		{Key: "groups", Label: "用户组", Type: PluginFieldSelect, OptionsFrom: "groups", Multiple: true},
		{Key: "hosts", Label: "域名", Type: PluginFieldTags},
		{Key: "targets", Label: "目标", Type: PluginFieldList, Fields: []PluginField{
			{Key: "name", Label: "名称", Type: PluginFieldText},
			{Key: "url", Label: "地址", Type: PluginFieldText, Required: true},
			{Key: "secret", Label: "密钥", Type: PluginFieldSecret},
		}},
	}}
}

func newPluginSettingsFixture(t *testing.T, sealing bool) (*PluginSettingsService, *pluginStoreFake, *pluginFake) {
	t.Helper()
	key := ""
	if sealing {
		key = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	}
	codec, err := secret.NewCodec(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	store := &pluginStoreFake{values: map[string]json.RawMessage{"site_name": json.RawMessage(`"x"`)}}
	plugin := &pluginFake{name: "webhook", schema: webhookSchema()}
	svc, err := NewPluginSettingsService(t.Context(), store, codec, []ConfigurablePlugin{plugin})
	if err != nil {
		t.Fatal(err)
	}
	return svc, store, plugin
}

func TestPluginSettingsDefaultsAreAppliedAndListed(t *testing.T) {
	svc, _, plugin := newPluginSettingsFixture(t, true)
	if fallbacks, err := svc.ApplyStored(t.Context()); err != nil || len(fallbacks) != 0 {
		t.Fatalf("apply: %v %v", fallbacks, err)
	}
	want := `{"enabled":false,"groups":[],"hosts":[],"opacity":0.6,"position":"br","targets":[],"token":""}`
	if len(plugin.applied) != 1 || plugin.applied[0] != want {
		t.Fatalf("applied: %v", plugin.applied)
	}
	views, err := svc.List(t.Context())
	if err != nil || len(views) != 1 {
		t.Fatalf("list: %v %v", views, err)
	}
	if !views[0].SecretsAvailable || views[0].Status[0].Value != "valid" || views[0].Values["position"] != "br" {
		t.Fatalf("view: %+v", views[0])
	}
}

func TestPluginSettingsSaveSealsSecretsAndMasksThem(t *testing.T) {
	svc, store, plugin := newPluginSettingsFixture(t, true)
	view, err := svc.Save(t.Context(), "webhook", json.RawMessage(`{"enabled":true,"token":"tok-123","hosts":[" a.com ","a.com"],"groups":[2],"targets":[{"name":"ops","url":"https://h/1","secret":"s-1","extra":1}],"unknown":true}`))
	if err != nil {
		t.Fatal(err)
	}
	stored := string(store.values["plugin.webhook"])
	if strings.Contains(stored, "tok-123") || strings.Contains(stored, "s-1") || !strings.Contains(stored, `"sealed"`) {
		t.Fatalf("stored in the clear: %s", stored)
	}
	last := plugin.applied[len(plugin.applied)-1]
	if !strings.Contains(last, `"token":"tok-123"`) || !strings.Contains(last, `"secret":"s-1"`) || strings.Contains(last, "_key") || strings.Contains(last, "extra") || strings.Contains(last, "unknown") || !strings.Contains(last, `"hosts":["a.com"]`) {
		t.Fatalf("plugin got: %s", last)
	}
	if view.Values["token"] != PluginSecretKept {
		t.Fatalf("token not masked: %v", view.Values["token"])
	}
	target := view.Values["targets"].([]any)[0].(map[string]any)
	if target["secret"] != PluginSecretKept || target["_key"] == "" {
		t.Fatalf("target view: %v", target)
	}

	// Sending the masks back keeps the secrets, even after reordering.
	key := target["_key"].(string)
	body := `{"enabled":true,"token":"` + PluginSecretKept + `","targets":[{"name":"new","url":"https://h/2","secret":"s-2"},{"_key":"` + key + `","name":"ops","url":"https://h/1","secret":"` + PluginSecretKept + `"}]}`
	if _, err := svc.Save(t.Context(), "webhook", json.RawMessage(body)); err != nil {
		t.Fatal(err)
	}
	last = plugin.applied[len(plugin.applied)-1]
	if !strings.Contains(last, `"token":"tok-123"`) || !strings.Contains(last, `"secret":"s-1"`) || !strings.Contains(last, `"secret":"s-2"`) {
		t.Fatalf("kept secrets lost: %s", last)
	}

	// A fresh process reads the sealed values back.
	again, err := NewPluginSettingsService(t.Context(), store, svc.secrets, []ConfigurablePlugin{plugin})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := again.ApplyStored(t.Context()); err != nil {
		t.Fatal(err)
	}
	if last := plugin.applied[len(plugin.applied)-1]; !strings.Contains(last, `"token":"tok-123"`) {
		t.Fatalf("restart lost settings: %s", last)
	}
}

func TestPluginSettingsRejectInvalidValues(t *testing.T) {
	for name, body := range map[string]string{
		"bool type":       `{"enabled":"yes"}`,
		"number range":    `{"opacity":2}`,
		"bad option":      `{"position":"middle"}`,
		"bad id":          `{"groups":[0]}`,
		"empty tag":       `{"hosts":[""]}`,
		"required item":   `{"targets":[{"name":"x","url":" "}]}`,
		"unknown kept":    `{"token":"` + PluginSecretKept + `"}`,
		"item kept fresh": `{"targets":[{"url":"https://h","secret":"` + PluginSecretKept + `"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			svc, store, plugin := newPluginSettingsFixture(t, true)
			_, err := svc.Save(t.Context(), "webhook", json.RawMessage(body))
			var settingsErr *PluginSettingsError
			if !errors.As(err, &settingsErr) || settingsErr.Message == "" {
				t.Fatalf("accepted: %v", err)
			}
			if _, saved := store.values["plugin.webhook"]; saved || len(plugin.applied) != 0 {
				t.Fatal("invalid values were applied or saved")
			}
		})
	}
}

func TestPluginSettingsPluginRefusalIsNotSaved(t *testing.T) {
	svc, store, plugin := newPluginSettingsFixture(t, true)
	plugin.refuse = &PluginSettingsError{Field: "hosts", Message: "域名格式不正确"}
	_, err := svc.Save(t.Context(), "webhook", json.RawMessage(`{"hosts":["x"]}`))
	var settingsErr *PluginSettingsError
	if !errors.As(err, &settingsErr) || settingsErr.Message != "域名格式不正确" {
		t.Fatalf("error: %v", err)
	}
	if _, saved := store.values["plugin.webhook"]; saved {
		t.Fatal("refused values saved")
	}
	plugin.refuse = errors.New("internal detail")
	if _, err := svc.Save(t.Context(), "webhook", json.RawMessage(`{}`)); !errors.Is(err, ErrInvalidInput) || strings.Contains(err.Error(), "internal detail") {
		t.Fatalf("plain refusal: %v", err)
	}
}

func TestPluginSettingsSecretsNeedAMasterKey(t *testing.T) {
	svc, store, _ := newPluginSettingsFixture(t, false)
	if _, err := svc.Save(t.Context(), "webhook", json.RawMessage(`{"token":"abc"}`)); !errors.Is(err, ErrPluginSecretsUnavailable) {
		t.Fatalf("secret saved without a key: %v", err)
	}
	if _, err := svc.Save(t.Context(), "webhook", json.RawMessage(`{"enabled":true}`)); err != nil {
		t.Fatalf("plain settings: %v", err)
	}
	if !strings.Contains(string(store.values["plugin.webhook"]), `"enabled":true`) {
		t.Fatalf("stored: %s", store.values["plugin.webhook"])
	}
}

func TestPluginSettingsHiddenAndUnknownPlugins(t *testing.T) {
	svc, _, plugin := newPluginSettingsFixture(t, true)
	plugin.hidden = true
	views, err := svc.List(t.Context())
	if err != nil || len(views) != 0 {
		t.Fatalf("hidden card listed: %v %v", views, err)
	}
	for _, name := range []string{"webhook", "missing"} {
		if _, err := svc.Save(t.Context(), name, json.RawMessage(`{}`)); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestPluginSettingsUnreadableStoredValuesFallBackToDefaults(t *testing.T) {
	svc, store, plugin := newPluginSettingsFixture(t, true)
	store.values["plugin.webhook"] = json.RawMessage(`{"version":1,"sealed":{"version":1,"sealed":"garbage"}}`)
	fallbacks, err := svc.ApplyStored(t.Context())
	if err != nil || len(fallbacks) != 1 || fallbacks[0] != "webhook" {
		t.Fatalf("fallbacks %v err %v", fallbacks, err)
	}
	if len(plugin.applied) != 1 || !strings.Contains(plugin.applied[0], `"position":"br"`) {
		t.Fatalf("defaults not applied: %v", plugin.applied)
	}
}

func TestPluginSettingsStoreFailureRestoresThePreviousValues(t *testing.T) {
	svc, store, plugin := newPluginSettingsFixture(t, true)
	if _, err := svc.Save(t.Context(), "webhook", json.RawMessage(`{"hosts":["old.com"]}`)); err != nil {
		t.Fatal(err)
	}
	store.fail = errors.New("disk full")
	if _, err := svc.Save(t.Context(), "webhook", json.RawMessage(`{"hosts":["new.com"]}`)); err == nil {
		t.Fatal("save succeeded")
	}
	if last := plugin.applied[len(plugin.applied)-1]; !strings.Contains(last, "old.com") {
		t.Fatalf("plugin left on unsaved values: %s", last)
	}
}

func TestPluginSettingsRejectBadSchemas(t *testing.T) {
	for name, fields := range map[string][]PluginField{
		"bad key":        {{Key: "Bad", Type: PluginFieldText}},
		"duplicate":      {{Key: "a", Type: PluginFieldText}, {Key: "a", Type: PluginFieldBool}},
		"unknown type":   {{Key: "a", Type: "color"}},
		"empty select":   {{Key: "a", Type: PluginFieldSelect}},
		"nested list":    {{Key: "a", Type: PluginFieldList, Fields: []PluginField{{Key: "b", Type: PluginFieldList, Fields: []PluginField{{Key: "c", Type: PluginFieldText}}}}}},
		"bad default":    {{Key: "a", Type: PluginFieldBool, Default: "yes"}},
		"default bounds": {{Key: "a", Type: PluginFieldInt, Default: 5, Max: floatPtr(3)}},
	} {
		t.Run(name, func(t *testing.T) {
			plugin := &pluginFake{name: "x", schema: PluginSchema{Fields: fields}}
			if _, err := NewPluginSettingsService(t.Context(), &pluginStoreFake{values: map[string]json.RawMessage{}}, nil, []ConfigurablePlugin{plugin}); err == nil {
				t.Fatal("schema accepted")
			}
		})
	}
}

func TestPluginSettingsDefaultSecretsCanBeKept(t *testing.T) {
	svc, _, plugin := newPluginSettingsFixture(t, true)
	schema := webhookSchema()
	schema.Fields[1].Default = "from-file"
	schema.Fields[6].Default = []map[string]any{{"name": "ops", "url": "https://h/1", "secret": "file-secret"}}
	plugin.schema = schema
	views, err := svc.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	values := views[0].Values
	target := values["targets"].([]any)[0].(map[string]any)
	if values["token"] != PluginSecretKept || target["secret"] != PluginSecretKept {
		t.Fatalf("default secrets shown: %v", values)
	}
	// Save the card exactly as read, plus a new item.
	values["targets"] = append(values["targets"].([]any), map[string]any{"url": "https://h/2", "secret": "s-2"})
	body, _ := json.Marshal(values)
	if _, err := svc.Save(t.Context(), "webhook", body); err != nil {
		t.Fatal(err)
	}
	last := plugin.applied[len(plugin.applied)-1]
	if !strings.Contains(last, `"token":"from-file"`) || !strings.Contains(last, `"secret":"file-secret"`) || !strings.Contains(last, `"secret":"s-2"`) {
		t.Fatalf("default secrets lost: %s", last)
	}
}

func TestPluginSettingsResponsesOmitSecretDefaults(t *testing.T) {
	for _, operation := range []string{"list", "save"} {
		t.Run(operation, func(t *testing.T) {
			svc, _, plugin := newPluginSettingsFixture(t, true)
			plugin.schema.Fields[1].Default = "synthetic-top-default"
			plugin.schema.Fields[6].Fields[2].Default = "synthetic-child-default"
			plugin.schema.Fields[6].Default = []map[string]any{{"url": "https://example.invalid", "secret": "synthetic-item-default"}}
			original, err := json.Marshal(plugin.schema)
			if err != nil {
				t.Fatal(err)
			}
			views, err := svc.List(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			view := views[0]
			if operation == "save" {
				raw, err := json.Marshal(view.Values)
				if err != nil {
					t.Fatal(err)
				}
				view, err = svc.Save(t.Context(), "webhook", raw)
				if err != nil {
					t.Fatal(err)
				}
			}
			response, err := json.Marshal(view)
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range []string{"synthetic-top-default", "synthetic-child-default", "synthetic-item-default"} {
				if strings.Contains(string(response), value) {
					t.Fatal("response includes a plaintext secret default")
				}
			}
			if view.Fields[1].Default != nil || view.Fields[6].Fields[2].Default != nil {
				t.Fatal("response schema exposes secret defaults")
			}
			items, ok := view.Fields[6].Default.([]any)
			if !ok || len(items) != 1 {
				t.Fatal("response lost the public list defaults")
			}
			item, ok := items[0].(map[string]any)
			if !ok || item["url"] != "https://example.invalid" {
				t.Fatal("response lost the public item defaults")
			}
			if _, exposed := item["secret"]; exposed {
				t.Fatal("list default includes a secret member")
			}
			// Changing the returned schema must not change the plugin's defaults.
			item["url"] = "https://changed.invalid"
			view.Fields[6].Fields[2].Default = "changed"
			after, err := json.Marshal(plugin.schema)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(original) {
				t.Fatal("response shares mutable schema data with the plugin")
			}
			if operation == "save" {
				last := plugin.applied[len(plugin.applied)-1]
				if !strings.Contains(last, `"token":"synthetic-top-default"`) || !strings.Contains(last, `"secret":"synthetic-item-default"`) {
					t.Fatal("masking the response changed the applied defaults")
				}
			}
		})
	}
}

type runtimePluginContextKey struct{}

type runtimePluginFake struct {
	pluginFake
	current     string
	reject      func(json.RawMessage) error
	deadline    time.Time
	bounded     bool
	contextMark any
}

func (p *runtimePluginFake) Apply(ctx context.Context, raw json.RawMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.reject != nil {
		if err := p.reject(raw); err != nil {
			return err
		}
	}
	p.current = string(raw)
	p.deadline, p.bounded = ctx.Deadline()
	p.contextMark = ctx.Value(runtimePluginContextKey{})
	return nil
}

func newRuntimePluginSettingsFixture(t *testing.T) (*PluginSettingsService, *pluginStoreFake, *runtimePluginFake) {
	t.Helper()
	old, store, plugin := newPluginSettingsFixture(t, true)
	runtime := &runtimePluginFake{pluginFake: *plugin}
	svc, err := NewPluginSettingsService(t.Context(), store, old.secrets, []ConfigurablePlugin{runtime})
	if err != nil {
		t.Fatal(err)
	}
	return svc, store, runtime
}

func TestPluginSettingsCanceledWriteRestoresRuntime(t *testing.T) {
	svc, store, plugin := newRuntimePluginSettingsFixture(t)
	if _, err := svc.Save(t.Context(), "webhook", json.RawMessage(`{"hosts":["old.example"]}`)); err != nil {
		t.Fatal(err)
	}
	oldRuntime, oldStored := plugin.current, string(store.values["plugin.webhook"])
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), runtimePluginContextKey{}, "request-marker"))
	defer cancel()
	store.beforeUpdate = func(ctx context.Context) error {
		cancel()
		return ctx.Err()
	}
	if _, err := svc.Save(ctx, "webhook", json.RawMessage(`{"hosts":["new.example"]}`)); !errors.Is(err, context.Canceled) {
		t.Fatal("expected a canceled persistence error")
	}
	if plugin.current != oldRuntime || string(store.values["plugin.webhook"]) != oldStored {
		t.Fatal("canceled persistence changed runtime or stored settings")
	}
	if !plugin.bounded || time.Until(plugin.deadline) <= 0 || time.Until(plugin.deadline) > time.Minute || plugin.contextMark != "request-marker" {
		t.Fatal("rollback must retain context values with a fresh bounded deadline")
	}
}

func TestPluginSettingsStoreFailureRestoresStartupFallback(t *testing.T) {
	for _, cause := range []string{"unreadable", "refused"} {
		t.Run(cause, func(t *testing.T) {
			svc, store, plugin := newRuntimePluginSettingsFixture(t)
			if cause == "unreadable" {
				store.values["plugin.webhook"] = json.RawMessage(`{"version":1,"sealed":{"version":1,"sealed":"garbage"}}`)
			} else {
				if _, err := svc.Save(t.Context(), "webhook", json.RawMessage(`{"hosts":["refused.example"]}`)); err != nil {
					t.Fatal(err)
				}
				plugin.reject = func(raw json.RawMessage) error {
					if strings.Contains(string(raw), "refused.example") {
						return &PluginSettingsError{Message: "stored setting refused"}
					}
					return nil
				}
			}
			fallbacks, err := svc.ApplyStored(t.Context())
			if err != nil || len(fallbacks) != 1 {
				t.Fatalf("apply stored settings: %v", err)
			}
			oldRuntime, oldStored := plugin.current, string(store.values["plugin.webhook"])
			store.fail = errors.New("persistence failed")
			if _, err := svc.Save(t.Context(), "webhook", json.RawMessage(`{"hosts":["new.example"]}`)); err == nil {
				t.Fatal("save succeeded despite persistence failure")
			}
			if plugin.current != oldRuntime || string(store.values["plugin.webhook"]) != oldStored {
				t.Fatal("failed save did not restore the active startup defaults")
			}
		})
	}
}

func TestPluginSettingsRollbackFailureIsReportedWithoutSecrets(t *testing.T) {
	svc, store, plugin := newRuntimePluginSettingsFixture(t)
	if _, err := svc.ApplyStored(t.Context()); err != nil {
		t.Fatal(err)
	}
	store.fail = errors.New("persistence failed")
	plugin.reject = func(raw json.RawMessage) error {
		if !strings.Contains(string(raw), "first.example") {
			return &PluginSettingsError{Message: "synthetic-sensitive-rollback-detail"}
		}
		return nil
	}
	_, err := svc.Save(t.Context(), "webhook", json.RawMessage(`{"hosts":["first.example"]}`))
	if !errors.Is(err, store.fail) || !strings.Contains(err.Error(), "restore plugin settings failed") || strings.Contains(err.Error(), "synthetic-sensitive-rollback-detail") {
		t.Fatal("rollback failure must be reported without plugin error details")
	}
	var settingsErr *PluginSettingsError
	if errors.As(err, &settingsErr) {
		t.Fatal("rollback refusal must not become a displayed validation message")
	}
	firstRuntime := plugin.current
	plugin.reject = nil
	if _, err := svc.Save(t.Context(), "webhook", json.RawMessage(`{"hosts":["second.example"]}`)); err == nil {
		t.Fatal("second save succeeded despite persistence failure")
	}
	if plugin.current != firstRuntime {
		t.Fatal("later compensation did not restore the actual active configuration")
	}
}

func TestPluginSettingsSelectNullClearsOptionalValues(t *testing.T) {
	for _, tc := range []struct {
		name      string
		required  bool
		body      string
		wantMode  any
		wantItem  bool
		wantField string
	}{
		{name: "missing uses default", body: `{}`, wantMode: "a"},
		{name: "optional null clears", body: `{"mode":null}`},
		{name: "required null rejected", required: true, body: `{"mode":null}`, wantField: "mode"},
		{name: "missing required uses default", required: true, body: `{}`, wantMode: "a"},
		{name: "optional item null clears", body: `{"entries":[{"mode":null}]}`, wantMode: "a", wantItem: true},
		{name: "required item null rejected", required: true, body: `{"entries":[{"mode":null}]}`, wantField: "entries.mode"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			field := PluginField{Key: "mode", Type: PluginFieldSelect, Required: tc.required, Default: "a", Options: []PluginOption{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}}}
			plugin := &pluginFake{name: "select", schema: PluginSchema{Fields: []PluginField{field, {Key: "entries", Type: PluginFieldList, Fields: []PluginField{field}}}}}
			store := &pluginStoreFake{values: map[string]json.RawMessage{}}
			svc, err := NewPluginSettingsService(t.Context(), store, nil, []ConfigurablePlugin{plugin})
			if err != nil {
				t.Fatal(err)
			}
			view, err := svc.Save(t.Context(), "select", json.RawMessage(tc.body))
			if tc.wantField != "" {
				var settingsErr *PluginSettingsError
				if !errors.As(err, &settingsErr) || settingsErr.Field != tc.wantField || len(plugin.applied) != 0 || len(store.values) != 0 {
					t.Fatal("required null was applied or persisted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			check := func(view PluginSettingsView) {
				t.Helper()
				if view.Values["mode"] != tc.wantMode {
					t.Fatal("select value differs from the submitted clear or default")
				}
				if tc.wantItem {
					items := view.Values["entries"].([]any)
					if len(items) != 1 || items[0].(map[string]any)["mode"] != nil {
						t.Fatal("cleared item select reverted to its default")
					}
				}
			}
			check(view)
			// The persisted clear must survive both a console read and startup.
			views, err := svc.List(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			check(views[0])
			again, err := NewPluginSettingsService(t.Context(), store, nil, []ConfigurablePlugin{plugin})
			if err != nil {
				t.Fatal(err)
			}
			if fallbacks, err := again.ApplyStored(t.Context()); err != nil || len(fallbacks) != 0 {
				t.Fatalf("apply persisted select settings: %v", err)
			}
			var applied map[string]any
			if err := json.Unmarshal([]byte(plugin.applied[len(plugin.applied)-1]), &applied); err != nil {
				t.Fatal(err)
			}
			if applied["mode"] != tc.wantMode {
				t.Fatal("startup reverted the persisted select value")
			}
			if tc.wantItem && applied["entries"].([]any)[0].(map[string]any)["mode"] != nil {
				t.Fatal("startup reverted the persisted item select clear")
			}
		})
	}
}
