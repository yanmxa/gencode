// Plugin management commands (/plugin list, enable, disable, info, errors).
package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	coreplugin "github.com/yanmxa/gencode/internal/plugin"
)

// HandleCommand dispatches /plugin subcommands.
func HandleCommand(ctx context.Context, selector *Model, cwd string, width, height int, args string) (string, error) {
	if coreplugin.DefaultRegistry.Count() == 0 {
		if err := coreplugin.DefaultRegistry.Load(ctx, cwd); err != nil {
			return fmt.Sprintf("Failed to load plugins: %v", err), nil
		}
		_ = coreplugin.DefaultRegistry.LoadClaudePlugins(ctx)
	}

	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	if len(parts) == 0 {
		if err := selector.EnterSelect(width, height); err != nil {
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
		return HandleList()
	case "install":
		return HandleInstall(ctx, cwd, parts[1:])
	case "marketplace":
		return HandleMarketplace(ctx, cwd, parts[1:])
	case "enable":
		return HandleEnable(ctx, pluginName)
	case "disable":
		return HandleDisable(ctx, pluginName)
	case "info":
		return HandleInfo(pluginName)
	case "errors":
		return HandleErrors()
	default:
		return HandleInfo(subCmd)
	}
}

// HandleList shows all installed plugins.
func HandleList() (string, error) {
	plugins := coreplugin.DefaultRegistry.List()

	if len(plugins) == 0 {
		return "No plugins installed.\n\nInstall with: gen plugin install <plugin>@<marketplace>", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Plugins (%d installed, %d enabled):\n\n",
		coreplugin.DefaultRegistry.Count(),
		coreplugin.DefaultRegistry.EnabledCount())

	for _, p := range plugins {
		writePluginSummary(&sb, p)
	}

	sb.WriteString("\nLegend: ● enabled  ○ disabled  👤 user  📁 project  💻 local")
	sb.WriteString("\n\nCommands:\n")
	sb.WriteString("  /plugin install <ref>    Install a plugin from a marketplace\n")
	sb.WriteString("  /plugin marketplace ...  Manage plugin marketplaces\n")
	sb.WriteString("  /plugin enable <name>   Enable a plugin\n")
	sb.WriteString("  /plugin disable <name>  Disable a plugin\n")
	sb.WriteString("  /plugin info <name>     Show plugin details\n")

	return sb.String(), nil
}

func writePluginSummary(sb *strings.Builder, p *coreplugin.Plugin) {
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

func formatComponentCounts(p *coreplugin.Plugin) []string {
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

// HandleEnable enables a plugin.
func HandleEnable(_ context.Context, name string) (string, error) {
	if name == "" {
		return "Usage: /plugin enable <plugin-name>", nil
	}

	if err := coreplugin.DefaultRegistry.Enable(name, coreplugin.ScopeUser); err != nil {
		return fmt.Sprintf("Failed to enable '%s': %v", name, err), nil
	}

	return fmt.Sprintf("Enabled plugin '%s'\n\nRun /reload-plugins to apply changes in the current session.", name), nil
}

// HandleDisable disables a plugin.
func HandleDisable(_ context.Context, name string) (string, error) {
	if name == "" {
		return "Usage: /plugin disable <plugin-name>", nil
	}

	if err := coreplugin.DefaultRegistry.Disable(name, coreplugin.ScopeUser); err != nil {
		return fmt.Sprintf("Failed to disable '%s': %v", name, err), nil
	}

	return fmt.Sprintf("Disabled plugin '%s'\n\nRun /reload-plugins to apply changes in the current session.", name), nil
}

// HandleInfo shows detailed info for a plugin.
func HandleInfo(name string) (string, error) {
	if name == "" {
		return "Usage: /plugin info <plugin-name>", nil
	}

	p, ok := coreplugin.DefaultRegistry.Get(name)
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

// HandleErrors shows all plugin errors.
func HandleErrors() (string, error) {
	plugins := coreplugin.DefaultRegistry.List()

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

// HandleInstall installs a plugin from a configured marketplace.
func HandleInstall(ctx context.Context, cwd string, args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /plugin install <plugin>@<marketplace> [user|project|local]", nil
	}
	if len(args) > 2 {
		return "Usage: /plugin install <plugin>@<marketplace> [user|project|local]", nil
	}

	scope, err := parseScopeArg("")
	if err != nil {
		return "", err
	}
	if len(args) == 2 {
		scope, err = parseScopeArg(args[1])
		if err != nil {
			return err.Error(), nil
		}
	}

	installer := coreplugin.NewInstaller(coreplugin.DefaultRegistry, cwd)
	if err := installer.LoadMarketplaces(); err != nil {
		return fmt.Sprintf("Failed to load marketplaces: %v", err), nil
	}

	ref := args[0]
	if err := installer.Install(ctx, ref, scope); err != nil {
		return fmt.Sprintf("Failed to install plugin '%s': %v", ref, err), nil
	}

	return fmt.Sprintf(
		"Installed plugin '%s' to %s scope.\n\nRun /reload-plugins to refresh skills, agents, MCP servers, and hooks.",
		ref,
		scope,
	), nil
}

// HandleMarketplace dispatches /plugin marketplace subcommands.
func HandleMarketplace(ctx context.Context, cwd string, args []string) (string, error) {
	if len(args) == 0 {
		return strings.Join([]string{
			"Usage: /plugin marketplace <subcommand>",
			"",
			"Subcommands:",
			"  /plugin marketplace list",
			"  /plugin marketplace add <owner/repo|path> [marketplace-id]",
			"  /plugin marketplace remove <marketplace-id>",
			"  /plugin marketplace sync <marketplace-id|all>",
		}, "\n"), nil
	}

	switch strings.ToLower(args[0]) {
	case "list":
		return HandleMarketplaceList(cwd)
	case "add":
		return HandleMarketplaceAdd(cwd, args[1:])
	case "remove":
		return HandleMarketplaceRemove(cwd, args[1:])
	case "sync":
		return HandleMarketplaceSync(ctx, cwd, args[1:])
	default:
		return fmt.Sprintf("Unknown marketplace subcommand: %s", args[0]), nil
	}
}

// HandleMarketplaceList shows configured plugin marketplaces.
func HandleMarketplaceList(cwd string) (string, error) {
	manager := coreplugin.NewMarketplaceManager(cwd)
	if err := manager.Load(); err != nil {
		return fmt.Sprintf("Failed to load marketplaces: %v", err), nil
	}

	ids := manager.List()
	sort.Strings(ids)
	if len(ids) == 0 {
		return "No marketplaces configured.\n\nAdd one with: /plugin marketplace add <owner/repo|path> [marketplace-id]", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Marketplaces (%d configured):\n\n", len(ids))
	for _, id := range ids {
		entry, ok := manager.Get(id)
		if !ok {
			continue
		}

		source := entry.Source.Path
		if entry.Source.Source == "github" {
			source = entry.Source.Repo
		}
		fmt.Fprintf(&sb, "  %s (%s)\n", id, entry.Source.Source)
		if source != "" {
			fmt.Fprintf(&sb, "      %s\n", source)
		}
	}

	sb.WriteString("\nCommands:\n")
	sb.WriteString("  /plugin marketplace add <owner/repo|path> [marketplace-id]\n")
	sb.WriteString("  /plugin marketplace remove <marketplace-id>\n")
	sb.WriteString("  /plugin marketplace sync <marketplace-id|all>\n")
	return sb.String(), nil
}

// HandleMarketplaceAdd registers a new marketplace source.
func HandleMarketplaceAdd(cwd string, args []string) (string, error) {
	if len(args) == 0 || len(args) > 2 {
		return "Usage: /plugin marketplace add <owner/repo|path> [marketplace-id]", nil
	}

	source := strings.TrimSpace(args[0])
	explicitID := ""
	if len(args) == 2 {
		explicitID = strings.TrimSpace(args[1])
	}

	id, normalizedSource, addFn, err := parseMarketplaceSource(source, explicitID)
	if err != nil {
		return err.Error(), nil
	}

	manager := coreplugin.NewMarketplaceManager(cwd)
	if err := manager.Load(); err != nil {
		return fmt.Sprintf("Failed to load marketplaces: %v", err), nil
	}
	if err := addFn(manager, id); err != nil {
		return fmt.Sprintf("Failed to add marketplace: %v", err), nil
	}

	return fmt.Sprintf(
		"Added marketplace '%s'.\n\nSource: %s\nInstall plugins with: /plugin install <plugin>@%s",
		id,
		normalizedSource,
		id,
	), nil
}

// HandleMarketplaceRemove removes a configured marketplace.
func HandleMarketplaceRemove(cwd string, args []string) (string, error) {
	if len(args) != 1 {
		return "Usage: /plugin marketplace remove <marketplace-id>", nil
	}

	manager := coreplugin.NewMarketplaceManager(cwd)
	if err := manager.Load(); err != nil {
		return fmt.Sprintf("Failed to load marketplaces: %v", err), nil
	}

	id := strings.TrimSpace(args[0])
	if _, ok := manager.Get(id); !ok {
		return fmt.Sprintf("Marketplace not found: %s", id), nil
	}
	if err := manager.Remove(id); err != nil {
		return fmt.Sprintf("Failed to remove marketplace '%s': %v", id, err), nil
	}

	return fmt.Sprintf("Removed marketplace '%s'.", id), nil
}

// HandleMarketplaceSync updates one or all configured marketplaces.
func HandleMarketplaceSync(ctx context.Context, cwd string, args []string) (string, error) {
	if len(args) != 1 {
		return "Usage: /plugin marketplace sync <marketplace-id|all>", nil
	}

	manager := coreplugin.NewMarketplaceManager(cwd)
	if err := manager.Load(); err != nil {
		return fmt.Sprintf("Failed to load marketplaces: %v", err), nil
	}

	target := strings.TrimSpace(args[0])
	if target == "all" {
		errs := manager.SyncAll(ctx)
		if len(errs) == 0 {
			return "Synced all marketplaces.", nil
		}
		var sb strings.Builder
		sb.WriteString("Failed to sync some marketplaces:\n")
		for _, err := range errs {
			fmt.Fprintf(&sb, "  - %v\n", err)
		}
		return strings.TrimRight(sb.String(), "\n"), nil
	}

	if err := manager.Sync(ctx, target); err != nil {
		return fmt.Sprintf("Failed to sync marketplace '%s': %v", target, err), nil
	}
	return fmt.Sprintf("Synced marketplace '%s'.", target), nil
}

func parseScopeArg(raw string) (coreplugin.Scope, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(coreplugin.ScopeUser):
		return coreplugin.ScopeUser, nil
	case string(coreplugin.ScopeProject):
		return coreplugin.ScopeProject, nil
	case string(coreplugin.ScopeLocal):
		return coreplugin.ScopeLocal, nil
	default:
		return "", fmt.Errorf("invalid scope: %s (expected user, project, or local)", raw)
	}
}

func parseMarketplaceSource(source, explicitID string) (id, normalizedSource string, addFn func(*coreplugin.MarketplaceManager, string) error, err error) {
	source = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(source, "]"), "["))
	if source == "" {
		return "", "", nil, fmt.Errorf("usage: /plugin marketplace add <owner/repo|path> [marketplace-id]")
	}

	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "~") {
		absPath, err := expandMarketplacePath(source)
		if err != nil {
			return "", "", nil, err
		}
		id = explicitID
		if id == "" {
			id = filepath.Base(absPath)
		}
		return id, absPath, func(manager *coreplugin.MarketplaceManager, id string) error {
			return manager.AddDirectory(id, absPath)
		}, nil
	}

	if strings.HasPrefix(source, "https://github.com/") {
		repo := strings.TrimPrefix(source, "https://github.com/")
		repo = strings.TrimSuffix(repo, ".git")
		repo = strings.TrimSuffix(repo, "/")
		return parseGitHubMarketplace(repo, explicitID)
	}

	if strings.Contains(source, "/") && !strings.Contains(source, "://") {
		return parseGitHubMarketplace(source, explicitID)
	}

	return "", "", nil, fmt.Errorf("invalid source format. Use owner/repo, https://github.com/owner/repo, or ./path")
}

func parseGitHubMarketplace(repo, explicitID string) (id, normalizedSource string, addFn func(*coreplugin.MarketplaceManager, string) error, err error) {
	parts := strings.Split(repo, "/")
	if len(parts) < 2 {
		return "", "", nil, fmt.Errorf("invalid GitHub repository: %s", repo)
	}

	id = explicitID
	if id == "" {
		id = parts[len(parts)-1]
	}

	return id, repo, func(manager *coreplugin.MarketplaceManager, id string) error {
		return manager.AddGitHub(id, repo)
	}, nil
}

func expandMarketplacePath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[1:])
	}
	return filepath.Abs(path)
}
