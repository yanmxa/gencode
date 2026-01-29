package config

// MergeSettings merges two Settings objects.
// Values from 'overlay' override values in 'base'.
// For slices (like permission rules), overlay values replace base values.
// For maps, overlay values are merged with base values.
func MergeSettings(base, overlay *Settings) *Settings {
	if base == nil {
		return overlay
	}
	if overlay == nil {
		return base
	}

	result := NewSettings()

	// Merge Permissions
	result.Permissions = mergePermissionSettings(base.Permissions, overlay.Permissions)

	// Merge Model (overlay wins if set)
	if overlay.Model != "" {
		result.Model = overlay.Model
	} else {
		result.Model = base.Model
	}

	// Merge Hooks (map merge)
	result.Hooks = mergeMaps(base.Hooks, overlay.Hooks)

	// Merge Env (map merge)
	result.Env = mergeStringMaps(base.Env, overlay.Env)

	// Merge EnabledPlugins (map merge)
	result.EnabledPlugins = mergeBoolMaps(base.EnabledPlugins, overlay.EnabledPlugins)

	// Merge DisabledTools (map merge)
	result.DisabledTools = mergeBoolMaps(base.DisabledTools, overlay.DisabledTools)

	return result
}

// mergePermissionSettings merges two PermissionSettings.
// Overlay values replace base values for each field.
func mergePermissionSettings(base, overlay PermissionSettings) PermissionSettings {
	result := PermissionSettings{}

	// Allow: overlay replaces base if non-empty
	if len(overlay.Allow) > 0 {
		result.Allow = make([]string, len(overlay.Allow))
		copy(result.Allow, overlay.Allow)
	} else if len(base.Allow) > 0 {
		result.Allow = make([]string, len(base.Allow))
		copy(result.Allow, base.Allow)
	}

	// Deny: overlay replaces base if non-empty
	if len(overlay.Deny) > 0 {
		result.Deny = make([]string, len(overlay.Deny))
		copy(result.Deny, overlay.Deny)
	} else if len(base.Deny) > 0 {
		result.Deny = make([]string, len(base.Deny))
		copy(result.Deny, base.Deny)
	}

	// Ask: overlay replaces base if non-empty
	if len(overlay.Ask) > 0 {
		result.Ask = make([]string, len(overlay.Ask))
		copy(result.Ask, overlay.Ask)
	} else if len(base.Ask) > 0 {
		result.Ask = make([]string, len(base.Ask))
		copy(result.Ask, base.Ask)
	}

	return result
}

// mergeMaps merges two map[string][]Hook.
// Overlay values are added to or replace base values.
func mergeMaps(base, overlay map[string][]Hook) map[string][]Hook {
	result := make(map[string][]Hook)

	// Copy base
	for k, v := range base {
		result[k] = append([]Hook{}, v...)
	}

	// Overlay
	for k, v := range overlay {
		result[k] = append([]Hook{}, v...)
	}

	return result
}

// mergeStringMaps merges two map[string]string.
func mergeStringMaps(base, overlay map[string]string) map[string]string {
	result := make(map[string]string)

	// Copy base
	for k, v := range base {
		result[k] = v
	}

	// Overlay
	for k, v := range overlay {
		result[k] = v
	}

	return result
}

// mergeBoolMaps merges two map[string]bool.
func mergeBoolMaps(base, overlay map[string]bool) map[string]bool {
	result := make(map[string]bool)

	// Copy base
	for k, v := range base {
		result[k] = v
	}

	// Overlay
	for k, v := range overlay {
		result[k] = v
	}

	return result
}
