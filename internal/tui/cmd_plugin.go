// Plugin management commands (/plugin list, enable, disable, info, errors).
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/plugin"
)

func handlePluginCommand(ctx context.Context, m *model, args string) (string, error) {
	if plugin.DefaultRegistry.Count() == 0 {
		if err := plugin.DefaultRegistry.Load(ctx, m.cwd); err != nil {
			return fmt.Sprintf("Failed to load plugins: %v", err), nil
		}
		_ = plugin.DefaultRegistry.LoadClaudePlugins(ctx)
	}

	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	if len(parts) == 0 {
		if err := m.pluginSelector.EnterPluginSelect(m.width, m.height); err != nil {
			return fmt.Sprintf("Failed to open plugin selector: %v", err), nil
		}
		return "", nil
	}

	subCmd := strings.ToLower(parts[0])
	var pluginName string
	if len(parts) > 1 {
		pluginName = parts[1]
	}

	switch subCmd {
	case "list":
		return handlePluginList(m)
	case "enable":
		return handlePluginEnable(ctx, m, pluginName)
	case "disable":
		return handlePluginDisable(ctx, m, pluginName)
	case "info":
		return handlePluginInfo(m, pluginName)
	case "errors":
		return handlePluginErrors(m)
	default:
		return handlePluginInfo(m, subCmd)
	}
}

func handlePluginList(_ *model) (string, error) {
	plugins := plugin.DefaultRegistry.List()

	if len(plugins) == 0 {
		return "No plugins installed.\n\nInstall with: gen plugin install <plugin>@<marketplace>", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Plugins (%d installed, %d enabled):\n\n",
		plugin.DefaultRegistry.Count(),
		plugin.DefaultRegistry.EnabledCount())

	for _, p := range plugins {
		writePluginSummary(&sb, p)
	}

	sb.WriteString("\nLegend: ● enabled  ○ disabled  👤 user  📁 project  💻 local")
	sb.WriteString("\n\nCommands:\n")
	sb.WriteString("  /plugin enable <name>   Enable a plugin\n")
	sb.WriteString("  /plugin disable <name>  Disable a plugin\n")
	sb.WriteString("  /plugin info <name>     Show plugin details\n")

	return sb.String(), nil
}

func writePluginSummary(sb *strings.Builder, p *plugin.Plugin) {
	status := "○"
	if p.Enabled {
		status = "●"
	}

	fmt.Fprintf(sb, "  %s %s %s (%s)\n", status, p.Scope.Icon(), p.FullName(), p.Scope)

	if p.Manifest.Description != "" {
		fmt.Fprintf(sb, "      %s\n", p.Manifest.Description)
	}

	components := formatComponentCounts(p)
	if len(components) > 0 {
		fmt.Fprintf(sb, "      [%s]\n", strings.Join(components, ", "))
	}
}

func formatComponentCounts(p *plugin.Plugin) []string {
	var components []string
	if n := len(p.Components.Skills); n > 0 {
		components = append(components, fmt.Sprintf("%d skills", n))
	}
	if n := len(p.Components.Agents); n > 0 {
		components = append(components, fmt.Sprintf("%d agents", n))
	}
	if n := len(p.Components.Commands); n > 0 {
		components = append(components, fmt.Sprintf("%d commands", n))
	}
	if p.Components.Hooks != nil {
		if n := len(p.Components.Hooks.Hooks); n > 0 {
			components = append(components, fmt.Sprintf("%d hooks", n))
		}
	}
	if n := len(p.Components.MCP); n > 0 {
		components = append(components, fmt.Sprintf("%d MCP", n))
	}
	if n := len(p.Components.LSP); n > 0 {
		components = append(components, fmt.Sprintf("%d LSP", n))
	}
	return components
}

func handlePluginEnable(_ context.Context, _ *model, name string) (string, error) {
	if name == "" {
		return "Usage: /plugin enable <plugin-name>", nil
	}

	if err := plugin.DefaultRegistry.Enable(name, plugin.ScopeUser); err != nil {
		return fmt.Sprintf("Failed to enable '%s': %v", name, err), nil
	}

	return fmt.Sprintf("Enabled plugin '%s'\n\nRestart session to apply changes.", name), nil
}

func handlePluginDisable(_ context.Context, _ *model, name string) (string, error) {
	if name == "" {
		return "Usage: /plugin disable <plugin-name>", nil
	}

	if err := plugin.DefaultRegistry.Disable(name, plugin.ScopeUser); err != nil {
		return fmt.Sprintf("Failed to disable '%s': %v", name, err), nil
	}

	return fmt.Sprintf("Disabled plugin '%s'\n\nRestart session to apply changes.", name), nil
}

func handlePluginInfo(_ *model, name string) (string, error) {
	if name == "" {
		return "Usage: /plugin info <plugin-name>", nil
	}

	p, ok := plugin.DefaultRegistry.Get(name)
	if !ok {
		return fmt.Sprintf("Plugin not found: %s\n\nUse /plugin list to see available plugins.", name), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Plugin: %s\n", p.FullName())
	fmt.Fprintf(&sb, "Scope: %s\n", p.Scope)
	fmt.Fprintf(&sb, "Enabled: %v\n", p.Enabled)
	fmt.Fprintf(&sb, "Path: %s\n", p.Path)

	writeOptionalField(&sb, "Version", p.Manifest.Version)
	writeOptionalField(&sb, "Description", p.Manifest.Description)
	if p.Manifest.Author != nil {
		writeOptionalField(&sb, "Author", p.Manifest.Author.Name)
	}
	writeOptionalField(&sb, "Repository", p.Manifest.Repository)

	sb.WriteString("\nComponents:\n")
	writeComponentCount(&sb, "Commands", len(p.Components.Commands))
	writeComponentCount(&sb, "Skills", len(p.Components.Skills))
	writeComponentCount(&sb, "Agents", len(p.Components.Agents))
	if p.Components.Hooks != nil {
		writeComponentCount(&sb, "Hook events", len(p.Components.Hooks.Hooks))
	}
	writeComponentCount(&sb, "MCP servers", len(p.Components.MCP))
	writeComponentCount(&sb, "LSP servers", len(p.Components.LSP))

	if len(p.Errors) > 0 {
		sb.WriteString("\nErrors:\n")
		for _, err := range p.Errors {
			fmt.Fprintf(&sb, "  - %s\n", err)
		}
	}

	return sb.String(), nil
}

func writeOptionalField(sb *strings.Builder, label, value string) {
	if value != "" {
		fmt.Fprintf(sb, "%s: %s\n", label, value)
	}
}

func writeComponentCount(sb *strings.Builder, label string, count int) {
	if count > 0 {
		fmt.Fprintf(sb, "  %s: %d\n", label, count)
	}
}

func handlePluginErrors(_ *model) (string, error) {
	plugins := plugin.DefaultRegistry.List()

	var sb strings.Builder
	hasErrors := false

	for _, p := range plugins {
		if len(p.Errors) > 0 {
			hasErrors = true
			fmt.Fprintf(&sb, "%s:\n", p.FullName())
			for _, err := range p.Errors {
				fmt.Fprintf(&sb, "  - %s\n", err)
			}
			sb.WriteString("\n")
		}
	}

	if !hasErrors {
		return "No plugin errors.", nil
	}

	return sb.String(), nil
}
