// Package adapter provides a plugin-based tool management system
// with support for dynamic loading, hot-reload, and loose coupling.
//
// Key components:
//   - ToolAdapter: Main adapter that manages plugins
//   - PluginLoader: Loads plugins from shared libraries (.so files)
//   - JSONPluginLoader: Loads plugins from JSON configuration files
//   - ConfigManager: Manages adapter configuration
package adapter
