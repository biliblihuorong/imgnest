package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

// Plugin setting field types; they mirror the extension package's constants.
const (
	PluginFieldText     = "text"
	PluginFieldTextarea = "textarea"
	PluginFieldSecret   = "secret"
	PluginFieldBool     = "bool"
	PluginFieldInt      = "int"
	PluginFieldNumber   = "number"
	PluginFieldSelect   = "select"
	PluginFieldTags     = "tags"
	PluginFieldList     = "list"
)

// PluginSecretKept is what the console reads in place of a stored secret and
// sends back to keep it unchanged.
const PluginSecretKept = "__imgnest_secret_kept__" //nolint:gosec // a placeholder, not a credential

// pluginItemKey identifies a list item across saves so its secrets survive
// reordering; plugins never see it.
const pluginItemKey = "_key"

// Bounds for stored plugin values.
const (
	maxPluginText     = 4096
	maxPluginTextarea = 65536
	maxPluginItems    = 100
	maxPluginTags     = 200
	maxPluginDocument = 64 << 10
)

var (
	pluginFieldKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	pluginItemID   = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

// ErrPluginSecretsUnavailable rejects saving a secret without a master key.
var ErrPluginSecretsUnavailable = errors.New("plugin secrets require security.master_key")

// PluginSettingsError is an invalid value the administrator can fix; Message
// comes from the schema or the plugin and is shown as is.
type PluginSettingsError struct {
	Field   string
	Message string
}

func (e *PluginSettingsError) Error() string { return e.Message }

// PluginOption is one choice of a select field.
type PluginOption struct {
	Label string `json:"label"`
	Value any    `json:"value"`
}

// PluginField is one input of a plugin's settings card.
type PluginField struct {
	Key         string         `json:"key"`
	Label       string         `json:"label"`
	Help        string         `json:"help,omitempty"`
	Type        string         `json:"type"`
	Placeholder string         `json:"placeholder,omitempty"`
	Required    bool           `json:"required,omitempty"`
	Default     any            `json:"default,omitempty"`
	Min         *float64       `json:"min,omitempty"`
	Max         *float64       `json:"max,omitempty"`
	Options     []PluginOption `json:"options,omitempty"`
	OptionsFrom string         `json:"options_from,omitempty"`
	Multiple    bool           `json:"multiple,omitempty"`
	Fields      []PluginField  `json:"fields,omitempty"`
	ItemLabel   string         `json:"item_label,omitempty"`
}

// PluginSchema is a plugin's settings card.
type PluginSchema struct {
	Title       string        `json:"title"`
	Description string        `json:"description,omitempty"`
	Fields      []PluginField `json:"fields"`
}

// PluginStatus is a read-only line on a settings card.
type PluginStatus struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Level string `json:"level"`
}

// ConfigurablePlugin is a plugin with a settings card.
type ConfigurablePlugin interface {
	Name() string
	// Schema returns the card; ok=false hides it.
	Schema(ctx context.Context) (PluginSchema, bool)
	// Apply validates and takes over the values; on error the previous
	// values stay in force. A *PluginSettingsError is shown to the admin.
	Apply(ctx context.Context, values json.RawMessage) error
	// Status returns read-only lines for the card; it may return nil.
	Status(ctx context.Context) []PluginStatus
}

// PluginSettingsView is one card as the console reads it. Secret values are
// replaced by PluginSecretKept, or "" when unset.
type PluginSettingsView struct {
	Name             string         `json:"name"`
	Title            string         `json:"title"`
	Description      string         `json:"description,omitempty"`
	Fields           []PluginField  `json:"fields"`
	Values           map[string]any `json:"values"`
	Status           []PluginStatus `json:"status"`
	SecretsAvailable bool           `json:"secrets_available"`
}

// PluginSettingsStore reads and upserts raw site settings values.
type PluginSettingsStore interface {
	ReadSettings(context.Context) (map[string]json.RawMessage, error)
	UpdateSettings(context.Context, map[string]json.RawMessage) error
}

// PluginSettingsService stores plugin settings in the site settings table and
// hands them to the plugins at startup and on every save.
type PluginSettingsService struct {
	store   PluginSettingsStore
	secrets SecretCodec
	sealing bool
	plugins []ConfigurablePlugin
	mu      sync.Mutex
}

type storedPluginSettings struct {
	Version int             `json:"version"`
	Values  json.RawMessage `json:"values,omitempty"`
	Sealed  json.RawMessage `json:"sealed,omitempty"`
}

// NewPluginSettingsService checks every schema the plugins currently expose.
// secrets may be nil; without a working codec, secret fields cannot be saved.
func NewPluginSettingsService(ctx context.Context, store PluginSettingsStore, secrets SecretCodec, plugins []ConfigurablePlugin) (*PluginSettingsService, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create plugin settings: %w", err)
	}
	if store == nil {
		return nil, errors.New("create plugin settings: missing settings repository")
	}
	s := &PluginSettingsService{store: store, secrets: secrets, plugins: plugins}
	if secrets != nil {
		if _, err := secrets.Seal(ctx, "plugin:probe", []byte("{}")); err == nil {
			s.sealing = true
		}
	}
	for _, plugin := range plugins {
		schema, _ := plugin.Schema(ctx)
		if err := checkPluginFields(schema.Fields, true); err != nil {
			return nil, fmt.Errorf("plugin %q settings: %w", plugin.Name(), err)
		}
		if _, err := normalizePluginValues(schema.Fields, nil, nil, false); err != nil {
			return nil, fmt.Errorf("plugin %q default settings: %w", plugin.Name(), err)
		}
	}
	return s, nil
}

// ApplyStored hands each plugin its saved values, or its defaults when none
// were saved. A plugin whose saved values it refuses gets its defaults, and
// its name is returned so the caller can log it; only a refused default is
// an error.
func (s *PluginSettingsService) ApplyStored(ctx context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, err := s.store.ReadSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("read plugin settings: %w", err)
	}
	var fallbacks []string
	for _, plugin := range s.plugins {
		schema, _ := plugin.Schema(ctx)
		values, err := s.decode(ctx, plugin.Name(), stored)
		if err == nil {
			values, err = normalizePluginValues(schema.Fields, values, values, false)
		}
		if err == nil {
			err = plugin.Apply(ctx, pluginApplyDocument(schema.Fields, values))
		}
		if err == nil {
			continue
		}
		fallbacks = append(fallbacks, plugin.Name())
		defaults, err := normalizePluginValues(schema.Fields, nil, nil, false)
		if err != nil {
			return fallbacks, fmt.Errorf("plugin %q default settings: %w", plugin.Name(), err)
		}
		if err := plugin.Apply(ctx, pluginApplyDocument(schema.Fields, defaults)); err != nil {
			return fallbacks, fmt.Errorf("plugin %q refused its default settings: %w", plugin.Name(), err)
		}
	}
	return fallbacks, nil
}

// List returns every visible settings card.
func (s *PluginSettingsService) List(ctx context.Context) ([]PluginSettingsView, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list plugin settings: %w", err)
	}
	stored, err := s.store.ReadSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("read plugin settings: %w", err)
	}
	views := []PluginSettingsView{}
	for _, plugin := range s.plugins {
		schema, ok := plugin.Schema(ctx)
		if !ok {
			continue
		}
		view, err := s.view(ctx, plugin, schema, stored)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// Save validates values against the plugin's schema, lets the plugin accept
// them, then stores them.
func (s *PluginSettingsService) Save(ctx context.Context, name string, raw json.RawMessage) (PluginSettingsView, error) {
	if err := ctx.Err(); err != nil {
		return PluginSettingsView{}, fmt.Errorf("save plugin settings: %w", err)
	}
	if len(raw) > maxPluginDocument {
		return PluginSettingsView{}, ErrInvalidInput
	}
	var plugin ConfigurablePlugin
	for _, candidate := range s.plugins {
		if candidate.Name() == name {
			plugin = candidate
		}
	}
	if plugin == nil {
		return PluginSettingsView{}, ErrNotFound
	}
	schema, ok := plugin.Schema(ctx)
	if !ok {
		return PluginSettingsView{}, ErrNotFound
	}
	input, err := decodePluginObject(raw)
	if err != nil {
		return PluginSettingsView{}, ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, err := s.store.ReadSettings(ctx)
	if err != nil {
		return PluginSettingsView{}, fmt.Errorf("read plugin settings: %w", err)
	}
	previous, err := s.decode(ctx, name, stored)
	if err != nil {
		// Unreadable old values (for example after a master key change) only
		// mean kept secrets cannot be resolved; the new values replace them.
		previous = nil
	}
	if previous == nil {
		// Nothing saved yet: the card showed the defaults, so kept secrets
		// refer to them.
		if previous, err = normalizePluginValues(schema.Fields, nil, nil, false); err != nil {
			return PluginSettingsView{}, fmt.Errorf("plugin %q default settings: %w", name, err)
		}
	}
	values, err := normalizePluginValues(schema.Fields, input, previous, true)
	if err != nil {
		return PluginSettingsView{}, err
	}
	if !s.sealing && pluginHasSecret(schema.Fields, values) {
		return PluginSettingsView{}, ErrPluginSecretsUnavailable
	}
	encoded, err := s.encode(ctx, name, values)
	if err != nil {
		return PluginSettingsView{}, err
	}
	if err := plugin.Apply(ctx, pluginApplyDocument(schema.Fields, values)); err != nil {
		var settingsErr *PluginSettingsError
		if errors.As(err, &settingsErr) {
			return PluginSettingsView{}, settingsErr
		}
		return PluginSettingsView{}, fmt.Errorf("apply plugin settings: %w", ErrInvalidInput)
	}
	if err := s.store.UpdateSettings(ctx, map[string]json.RawMessage{pluginSettingsKey(name): encoded}); err != nil {
		// Put the plugin back on what is still stored.
		if old, decodeErr := s.decode(ctx, name, stored); decodeErr == nil {
			if old, normErr := normalizePluginValues(schema.Fields, old, old, false); normErr == nil {
				_ = plugin.Apply(ctx, pluginApplyDocument(schema.Fields, old))
			}
		}
		return PluginSettingsView{}, fmt.Errorf("write plugin settings: %w", err)
	}
	stored[pluginSettingsKey(name)] = encoded
	return s.view(ctx, plugin, schema, stored)
}

func (s *PluginSettingsService) view(ctx context.Context, plugin ConfigurablePlugin, schema PluginSchema, stored map[string]json.RawMessage) (PluginSettingsView, error) {
	values, err := s.decode(ctx, plugin.Name(), stored)
	if err == nil {
		values, err = normalizePluginValues(schema.Fields, values, values, false)
	}
	if err != nil {
		// Show defaults rather than failing the whole page; saving replaces
		// the unreadable values.
		values, err = normalizePluginValues(schema.Fields, nil, nil, false)
		if err != nil {
			return PluginSettingsView{}, fmt.Errorf("plugin %q default settings: %w", plugin.Name(), err)
		}
	}
	status := plugin.Status(ctx)
	if status == nil {
		status = []PluginStatus{}
	}
	return PluginSettingsView{Name: plugin.Name(), Title: schema.Title, Description: schema.Description, Fields: schema.Fields, Values: maskPluginSecrets(schema.Fields, values), Status: status, SecretsAvailable: s.sealing}, nil
}

func pluginSettingsKey(name string) string { return "plugin." + name }

// decode returns the stored values, or nil when nothing was saved.
func (s *PluginSettingsService) decode(ctx context.Context, name string, stored map[string]json.RawMessage) (map[string]any, error) {
	raw, ok := stored[pluginSettingsKey(name)]
	if !ok {
		return nil, nil
	}
	var doc storedPluginSettings
	if err := json.Unmarshal(raw, &doc); err != nil || doc.Version != 1 {
		return nil, ErrInvalidInput
	}
	plain := []byte(doc.Values)
	if len(doc.Sealed) > 0 {
		if s.secrets == nil {
			return nil, ErrInvalidInput
		}
		opened, err := s.secrets.Open(ctx, "plugin:"+name, doc.Sealed)
		if err != nil {
			return nil, ErrInvalidInput
		}
		plain = opened
	}
	return decodePluginObject(plain)
}

func (s *PluginSettingsService) encode(ctx context.Context, name string, values map[string]any) (json.RawMessage, error) {
	plain, err := json.Marshal(values)
	if err != nil {
		return nil, ErrInvalidInput
	}
	doc := storedPluginSettings{Version: 1}
	if s.sealing {
		sealed, err := s.secrets.Seal(ctx, "plugin:"+name, plain)
		if err != nil {
			return nil, fmt.Errorf("seal plugin settings: %w", err)
		}
		doc.Sealed = sealed
	} else {
		doc.Values = plain
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, ErrInvalidInput
	}
	return encoded, nil
}

func decodePluginObject(raw []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, ErrInvalidInput
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var values map[string]any
	if err := decoder.Decode(&values); err != nil || decoder.More() {
		return nil, ErrInvalidInput
	}
	if values == nil {
		values = map[string]any{}
	}
	return values, nil
}

// checkPluginFields rejects malformed schemas when the server starts.
func checkPluginFields(fields []PluginField, topLevel bool) error {
	seen := map[string]bool{}
	for _, field := range fields {
		if !pluginFieldKey.MatchString(field.Key) || seen[field.Key] {
			return fmt.Errorf("field %q: invalid or duplicate key", field.Key)
		}
		seen[field.Key] = true
		switch field.Type {
		case PluginFieldText, PluginFieldTextarea, PluginFieldSecret, PluginFieldBool, PluginFieldInt, PluginFieldNumber, PluginFieldTags:
		case PluginFieldSelect:
			if len(field.Options) == 0 && field.OptionsFrom == "" {
				return fmt.Errorf("field %q: select needs options", field.Key)
			}
			switch field.OptionsFrom {
			case "", "groups", "policies", "storages":
			default:
				return fmt.Errorf("field %q: unknown options source %q", field.Key, field.OptionsFrom)
			}
		case PluginFieldList:
			if !topLevel || len(field.Fields) == 0 {
				return fmt.Errorf("field %q: lists need item fields and cannot nest", field.Key)
			}
			if err := checkPluginFields(field.Fields, false); err != nil {
				return fmt.Errorf("field %q: %w", field.Key, err)
			}
		default:
			return fmt.Errorf("field %q: unknown type %q", field.Key, field.Type)
		}
	}
	return nil
}

// normalizePluginValues returns exactly the schema's keys with checked values:
// missing ones take their defaults, secrets equal to PluginSecretKept take
// their previous value, and list items get stable keys. required enforces
// Required (on save only, so defaults may leave required fields empty).
func normalizePluginValues(fields []PluginField, input, previous map[string]any, required bool) (map[string]any, error) {
	out := make(map[string]any, len(fields))
	for _, field := range fields {
		value, present := input[field.Key]
		if !present || value == nil {
			var err error
			value, err = pluginDefault(field)
			if err != nil {
				return nil, err
			}
		}
		normalized, err := normalizePluginValue(field, value, previous[field.Key], required)
		if err != nil {
			return nil, err
		}
		out[field.Key] = normalized
	}
	return out, nil
}

func pluginDefault(field PluginField) (any, error) {
	if field.Default == nil {
		switch field.Type {
		case PluginFieldText, PluginFieldTextarea, PluginFieldSecret:
			return "", nil
		case PluginFieldBool:
			return false, nil
		case PluginFieldTags, PluginFieldList:
			return []any{}, nil
		case PluginFieldSelect:
			if field.Multiple {
				return []any{}, nil
			}
			return nil, nil
		case PluginFieldInt, PluginFieldNumber:
			return json.Number("0"), nil
		}
		return nil, nil
	}
	raw, err := json.Marshal(field.Default)
	if err != nil {
		return nil, fmt.Errorf("field %q: invalid default", field.Key)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("field %q: invalid default", field.Key)
	}
	return value, nil
}

func pluginInvalid(field PluginField, reason string) error {
	label := field.Label
	if label == "" {
		label = field.Key
	}
	return &PluginSettingsError{Field: field.Key, Message: label + reason}
}

func normalizePluginValue(field PluginField, value, previous any, required bool) (any, error) {
	switch field.Type {
	case PluginFieldText, PluginFieldTextarea:
		text, ok := value.(string)
		limit := maxPluginText
		if field.Type == PluginFieldTextarea {
			limit = maxPluginTextarea
		}
		if !ok || !utf8.ValidString(text) || len(text) > limit {
			return nil, pluginInvalid(field, "：格式不正确")
		}
		if field.Type == PluginFieldText {
			text = strings.TrimSpace(text)
		}
		if required && field.Required && strings.TrimSpace(text) == "" {
			return nil, pluginInvalid(field, "：不能为空")
		}
		return text, nil
	case PluginFieldSecret:
		text, ok := value.(string)
		if !ok || !utf8.ValidString(text) || len(text) > maxPluginText {
			return nil, pluginInvalid(field, "：格式不正确")
		}
		if text == PluginSecretKept {
			old, _ := previous.(string)
			if old == "" {
				return nil, pluginInvalid(field, "：请重新填写")
			}
			text = old
		}
		if required && field.Required && text == "" {
			return nil, pluginInvalid(field, "：不能为空")
		}
		return text, nil
	case PluginFieldBool:
		flag, ok := value.(bool)
		if !ok {
			return nil, pluginInvalid(field, "：格式不正确")
		}
		return flag, nil
	case PluginFieldInt, PluginFieldNumber:
		number, ok := value.(json.Number)
		if !ok {
			return nil, pluginInvalid(field, "：必须是数字")
		}
		parsed, err := number.Float64()
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return nil, pluginInvalid(field, "：必须是数字")
		}
		if field.Type == PluginFieldInt {
			if _, err := number.Int64(); err != nil {
				return nil, pluginInvalid(field, "：必须是整数")
			}
		}
		if (field.Min != nil && parsed < *field.Min) || (field.Max != nil && parsed > *field.Max) {
			return nil, pluginInvalid(field, "：超出允许范围")
		}
		return number, nil
	case PluginFieldSelect:
		if !field.Multiple {
			if value == nil {
				if required && field.Required {
					return nil, pluginInvalid(field, "：请选择")
				}
				return nil, nil
			}
			if !pluginOptionAllowed(field, value) {
				return nil, pluginInvalid(field, "：选项无效")
			}
			return value, nil
		}
		items, ok := value.([]any)
		if !ok || len(items) > maxPluginTags {
			return nil, pluginInvalid(field, "：格式不正确")
		}
		seen := map[string]bool{}
		out := make([]any, 0, len(items))
		for _, item := range items {
			if !pluginOptionAllowed(field, item) {
				return nil, pluginInvalid(field, "：选项无效")
			}
			key := fmt.Sprint(item)
			if !seen[key] {
				seen[key] = true
				out = append(out, item)
			}
		}
		if required && field.Required && len(out) == 0 {
			return nil, pluginInvalid(field, "：请至少选择一项")
		}
		return out, nil
	case PluginFieldTags:
		items, ok := value.([]any)
		if !ok || len(items) > maxPluginTags {
			return nil, pluginInvalid(field, "：格式不正确")
		}
		out := make([]any, 0, len(items))
		seen := map[string]bool{}
		for _, item := range items {
			text, ok := item.(string)
			text = strings.TrimSpace(text)
			if !ok || text == "" || !utf8.ValidString(text) || len(text) > 255 {
				return nil, pluginInvalid(field, "：包含无效的条目")
			}
			if !seen[text] {
				seen[text] = true
				out = append(out, text)
			}
		}
		if required && field.Required && len(out) == 0 {
			return nil, pluginInvalid(field, "：不能为空")
		}
		return out, nil
	case PluginFieldList:
		return normalizePluginList(field, value, previous, required)
	}
	return nil, pluginInvalid(field, "：类型未知")
}

func pluginOptionAllowed(field PluginField, value any) bool {
	if field.OptionsFrom != "" {
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		id, err := number.Int64()
		return err == nil && id > 0
	}
	for _, option := range field.Options {
		if pluginOptionEqual(option.Value, value) {
			return true
		}
	}
	return false
}

func pluginOptionEqual(option, value any) bool {
	switch v := value.(type) {
	case string:
		text, ok := option.(string)
		return ok && text == v
	case json.Number:
		if _, ok := option.(string); ok || option == nil {
			return false
		}
		return fmt.Sprint(option) == v.String()
	}
	return false
}

func normalizePluginList(field PluginField, value, previous any, required bool) (any, error) {
	items, ok := value.([]any)
	if !ok || len(items) > maxPluginItems {
		return nil, pluginInvalid(field, "：格式不正确")
	}
	old := map[string]map[string]any{}
	if previousItems, ok := previous.([]any); ok {
		for _, item := range previousItems {
			if object, ok := item.(map[string]any); ok {
				if key, ok := object[pluginItemKey].(string); ok {
					old[key] = object
				}
			}
		}
	}
	out := make([]any, 0, len(items))
	// Keys the client sent are claimed first so a new item never takes the
	// key of an existing one.
	claimed := map[string]int{}
	for _, item := range items {
		if object, ok := item.(map[string]any); ok {
			if key, ok := object[pluginItemKey].(string); ok && pluginItemID.MatchString(key) {
				claimed[key]++
			}
		}
	}
	used := map[string]bool{}
	for index, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, pluginInvalid(field, "：格式不正确")
		}
		key, _ := object[pluginItemKey].(string)
		if !pluginItemID.MatchString(key) || used[key] {
			// Items without a key get one from their position, so default
			// items keep the same key between reading and saving the card.
			key = fmt.Sprintf("%016x", index)
			if used[key] || claimed[key] > 0 {
				key = newPluginItemKey()
			}
		}
		used[key] = true
		normalized, err := normalizePluginValues(field.Fields, object, old[key], required)
		if err != nil {
			var settingsErr *PluginSettingsError
			if errors.As(err, &settingsErr) {
				return nil, &PluginSettingsError{Field: field.Key + "." + settingsErr.Field, Message: field.Label + " · " + settingsErr.Message}
			}
			return nil, err
		}
		normalized[pluginItemKey] = key
		out = append(out, normalized)
	}
	if required && field.Required && len(out) == 0 {
		return nil, pluginInvalid(field, "：至少需要一项")
	}
	return out, nil
}

func newPluginItemKey() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}

// pluginApplyDocument is what the plugin receives: the values without item keys.
func pluginApplyDocument(fields []PluginField, values map[string]any) json.RawMessage {
	plain := make(map[string]any, len(values))
	for _, field := range fields {
		value := values[field.Key]
		if field.Type == PluginFieldList {
			items, _ := value.([]any)
			stripped := make([]any, 0, len(items))
			for _, item := range items {
				object, _ := item.(map[string]any)
				copied := make(map[string]any, len(object))
				for key, v := range object {
					if key != pluginItemKey {
						copied[key] = v
					}
				}
				stripped = append(stripped, copied)
			}
			value = stripped
		}
		plain[field.Key] = value
	}
	encoded, _ := json.Marshal(plain)
	return encoded
}

func maskPluginSecrets(fields []PluginField, values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for _, field := range fields {
		value := values[field.Key]
		switch field.Type {
		case PluginFieldSecret:
			if text, _ := value.(string); text != "" {
				value = PluginSecretKept
			}
		case PluginFieldList:
			items, _ := value.([]any)
			masked := make([]any, 0, len(items))
			for _, item := range items {
				object, _ := item.(map[string]any)
				copied := maskPluginSecrets(field.Fields, object)
				copied[pluginItemKey] = object[pluginItemKey]
				masked = append(masked, copied)
			}
			value = masked
		}
		out[field.Key] = value
	}
	return out
}

func pluginHasSecret(fields []PluginField, values map[string]any) bool {
	for _, field := range fields {
		switch field.Type {
		case PluginFieldSecret:
			if text, _ := values[field.Key].(string); text != "" {
				return true
			}
		case PluginFieldList:
			items, _ := values[field.Key].([]any)
			for _, item := range items {
				if object, ok := item.(map[string]any); ok && pluginHasSecret(field.Fields, object) {
					return true
				}
			}
		}
	}
	return false
}
