package registry

import (
	"fmt"
	"sort"

	"github.com/bitop-dev/agent/pkg/plugin"
	"github.com/bitop-dev/agent/pkg/provider"
	"github.com/bitop-dev/agent/pkg/tool"
)

type ToolRegistry struct {
	tools map[string]tool.Tool
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]tool.Tool)}
}

func (r *ToolRegistry) Register(t tool.Tool) error {
	def := t.Definition()
	if def.ID == "" {
		return fmt.Errorf("tool id is required")
	}
	r.tools[def.ID] = t
	return nil
}

func (r *ToolRegistry) Get(id string) (tool.Tool, bool) {
	t, ok := r.tools[id]
	return t, ok
}

func (r *ToolRegistry) List() []tool.Definition {
	defs := make([]tool.Definition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition())
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].ID < defs[j].ID })
	return defs
}

type ProviderRegistry struct {
	providers map[string]provider.Provider
}

func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{providers: make(map[string]provider.Provider)}
}

func (r *ProviderRegistry) Register(p provider.Provider) error {
	if p == nil {
		return fmt.Errorf("provider is required")
	}
	if p.Name() == "" {
		return fmt.Errorf("provider name is required")
	}
	r.providers[p.Name()] = p
	return nil
}

func (r *ProviderRegistry) Get(name string) (provider.Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

func (r *ProviderRegistry) List() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type AssetReference struct {
	PluginName string
	ID         string
	Path       string
}

type PromptRegistry struct {
	items map[string]AssetReference
}

func NewPromptRegistry() *PromptRegistry {
	return &PromptRegistry{items: make(map[string]AssetReference)}
}

func (r *PromptRegistry) Register(ref AssetReference) error {
	if ref.ID == "" {
		return fmt.Errorf("prompt id is required")
	}
	r.items[ref.ID] = ref
	return nil
}

func (r *PromptRegistry) Get(id string) (AssetReference, bool) {
	ref, ok := r.items[id]
	return ref, ok
}

func (r *PromptRegistry) List() []AssetReference {
	return listAssetReferences(r.items)
}

type ProfileTemplateRegistry struct {
	items map[string]AssetReference
}

func NewProfileTemplateRegistry() *ProfileTemplateRegistry {
	return &ProfileTemplateRegistry{items: make(map[string]AssetReference)}
}

func (r *ProfileTemplateRegistry) Register(ref AssetReference) error {
	if ref.ID == "" {
		return fmt.Errorf("profile template id is required")
	}
	r.items[ref.ID] = ref
	return nil
}

func (r *ProfileTemplateRegistry) Get(id string) (AssetReference, bool) {
	ref, ok := r.items[id]
	return ref, ok
}

func (r *ProfileTemplateRegistry) List() []AssetReference {
	return listAssetReferences(r.items)
}

type PolicyRegistry struct {
	items map[string]AssetReference
}

func NewPolicyRegistry() *PolicyRegistry {
	return &PolicyRegistry{items: make(map[string]AssetReference)}
}

func (r *PolicyRegistry) Register(ref AssetReference) error {
	if ref.ID == "" {
		return fmt.Errorf("policy id is required")
	}
	r.items[ref.ID] = ref
	return nil
}

func (r *PolicyRegistry) Get(id string) (AssetReference, bool) {
	ref, ok := r.items[id]
	return ref, ok
}

func (r *PolicyRegistry) List() []AssetReference {
	return listAssetReferences(r.items)
}

type PluginRegistry struct {
	items map[string]plugin.Manifest
}

func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{items: make(map[string]plugin.Manifest)}
}

func (r *PluginRegistry) Register(manifest plugin.Manifest) error {
	if manifest.Metadata.Name == "" {
		return fmt.Errorf("plugin name is required")
	}
	r.items[manifest.Metadata.Name] = manifest
	return nil
}

func (r *PluginRegistry) Get(name string) (plugin.Manifest, bool) {
	manifest, ok := r.items[name]
	return manifest, ok
}

func (r *PluginRegistry) List() []string {
	names := make([]string, 0, len(r.items))
	for name := range r.items {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func listAssetReferences(items map[string]AssetReference) []AssetReference {
	refs := make([]AssetReference, 0, len(items))
	for _, ref := range items {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	return refs
}
