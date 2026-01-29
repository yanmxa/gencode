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
// Overlay values are appended to base values (deduplicated).
func mergePermissionSettings(base, overlay PermissionSettings) PermissionSettings {
	result := PermissionSettings{}

	// Allow: merge both lists, deduplicate
	result.Allow = mergeStringSlices(base.Allow, overlay.Allow)

	// Deny: merge both lists, deduplicate
	result.Deny = mergeStringSlices(base.Deny, overlay.Deny)

	// Ask: merge both lists, deduplicate
	result.Ask = mergeStringSlices(base.Ask, overlay.Ask)

	return result
}

// mergeStringSlices merges two string slices, removing duplicates.
func mergeStringSlices(base, overlay []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, s := range base {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	for _, s := range overlay {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
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
