package tool

// Lazy model-surface helpers: tools can be registered for execution while
// staying hidden from EyrieTools until promoted (ToolSearch select, mode, etc.).

// EnableLazyModelSurface restricts model-visible tools to essentialNames.
// All other primary tools remain executable via Get (and discoverable via
// ToolSearch when AvailableTools includes them) but are omitted from
// EyrieTools until PromoteModelTool.
func (r *Registry) EnableLazyModelSurface(essentialNames []string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelVisible = make(map[string]bool, len(essentialNames))
	for _, name := range essentialNames {
		if name == "" {
			continue
		}
		// Resolve aliases to primary tool names when already registered.
		if t, ok := r.tools[name]; ok {
			r.modelVisible[t.Name()] = true
		} else {
			r.modelVisible[name] = true
		}
	}
}

// SetModelVisibility replaces the model-visible allowlist. Empty clears
// lazy mode (all primary tools become model-visible again).
func (r *Registry) SetModelVisibility(names []string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(names) == 0 {
		r.modelVisible = nil
		return
	}
	r.modelVisible = make(map[string]bool, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		if t, ok := r.tools[name]; ok {
			r.modelVisible[t.Name()] = true
		} else {
			r.modelVisible[name] = true
		}
	}
}

// PromoteModelTool makes a registered tool visible to the model.
// Returns false if the tool is unknown.
func (r *Registry) PromoteModelTool(name string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tools[name]
	if !ok {
		return false
	}
	if r.modelVisible == nil {
		// All tools already model-visible.
		return true
	}
	r.modelVisible[t.Name()] = true
	return true
}

// ModelVisibleNames returns names currently sent to the model.
func (r *Registry) ModelVisibleNames() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []string
	for _, t := range r.primary {
		if r.modelVisible == nil || r.modelVisible[t.Name()] {
			out = append(out, t.Name())
		}
	}
	return out
}

// IsModelVisible reports whether name is included in EyrieTools.
func (r *Registry) IsModelVisible(name string) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	if !ok {
		return false
	}
	if r.modelVisible == nil {
		return true
	}
	return r.modelVisible[t.Name()]
}
