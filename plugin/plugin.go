package plugin

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/nekohy/MeowCLI/utils"
)

type Context struct {
	Alias   string
	Origin  string
	Handler utils.HandlerType
	APIType utils.APIType
	Stream  bool

	body []byte
	json *ast.Node
}

func NewContext(body []byte) *Context {
	return &Context{body: body}
}

func (c *Context) SetBody(body []byte) {
	if c == nil {
		return
	}
	c.body = body
	c.json = nil
}

// JSON 返回请求级共享的惰性 AST，bridge 和多个插件只解析一次
func (c *Context) JSON() (*ast.Node, error) {
	if c == nil {
		return nil, fmt.Errorf("plugin context is nil")
	}
	if c.json != nil {
		return c.json, nil
	}
	trimmed := bytes.TrimSpace(c.body)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("request body is empty")
	}
	root, err := sonic.GetWithOptions(trimmed, ast.SearchOptions{ValidateJSON: true})
	if err != nil {
		return nil, err
	}
	c.json = &root
	return c.json, nil
}

func (c *Context) Bytes() ([]byte, error) {
	if c == nil {
		return nil, nil
	}
	if c.json != nil {
		return c.json.MarshalJSON()
	}
	return c.body, nil
}

type Manifest struct {
	Name        string              `json:"name"`
	Label       string              `json:"label"`
	Description string              `json:"description"`
	Handlers    []utils.HandlerType `json:"handlers"`
	APITypes    []utils.APIType     `json:"api_types"`
}

type Interface interface {
	Manifest() Manifest
	Apply(context.Context, *Context) error
}

type Registry struct {
	plugins map[string]Interface
}

var defaultRegistry = struct {
	sync.RWMutex
	plugins []Interface
}{}

func Register(p Interface) {
	if p == nil {
		return
	}
	defaultRegistry.Lock()
	defaultRegistry.plugins = append(defaultRegistry.plugins, p)
	defaultRegistry.Unlock()
}

func DefaultRegistry() *Registry {
	registry := NewRegistry()
	defaultRegistry.RLock()
	defer defaultRegistry.RUnlock()
	for _, p := range defaultRegistry.plugins {
		registry.Register(p)
	}
	return registry
}

func NewRegistry() *Registry {
	return &Registry{plugins: map[string]Interface{}}
}

func (r *Registry) Register(p Interface) {
	if r == nil || p == nil {
		return
	}
	manifest := p.Manifest()
	name := strings.TrimSpace(manifest.Name)
	if name == "" {
		return
	}
	if r.plugins == nil {
		r.plugins = map[string]Interface{}
	}
	r.plugins[name] = p
}

func (r *Registry) Get(name string) (Interface, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.plugins[strings.TrimSpace(name)]
	return p, ok
}

func (r *Registry) Available(handler utils.HandlerType, apiType utils.APIType) []Manifest {
	if r == nil {
		return nil
	}
	plugins := make([]Manifest, 0, len(r.plugins))
	for _, p := range r.plugins {
		manifest := p.Manifest()
		if manifest.Supports(handler, apiType) {
			plugins = append(plugins, manifest)
		}
	}
	slices.SortFunc(plugins, func(a, b Manifest) int {
		return strings.Compare(a.Name, b.Name)
	})
	return plugins
}

func (r *Registry) Run(ctx context.Context, enabled []string, req *Context) ([]byte, error) {
	if err := r.Apply(ctx, enabled, req); err != nil {
		return nil, err
	}
	return req.Bytes()
}

// Apply 依次修改同一个请求 Context，不在插件链结束时强制序列化
// 调用方可以继续复用 Context 中缓存的最终 AST
func (r *Registry) Apply(ctx context.Context, enabled []string, req *Context) error {
	if req == nil {
		return fmt.Errorf("plugin context is nil")
	}
	for _, name := range enabled {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		p, ok := r.Get(name)
		if !ok {
			return fmt.Errorf("plugin %q is not registered", name)
		}
		manifest := p.Manifest()
		if !manifest.Supports(req.Handler, req.APIType) {
			continue
		}
		if err := p.Apply(ctx, req); err != nil {
			return fmt.Errorf("plugin %q: %w", name, err)
		}
	}
	return nil
}

func (m Manifest) SupportsHandler(handler utils.HandlerType) bool {
	return slices.Contains(m.Handlers, handler)
}

func (m Manifest) Supports(handler utils.HandlerType, apiType utils.APIType) bool {
	if !m.SupportsHandler(handler) {
		return false
	}
	if len(m.APITypes) > 0 && !slices.Contains(m.APITypes, apiType) {
		return false
	}
	return true
}

func ParseList(raw string) []string {
	seen := map[string]struct{}{}
	names := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}
