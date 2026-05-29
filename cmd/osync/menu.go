package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/obsidian-sync-f2p/internal/config"
)

// ASCII header for OSYNC.
const osyncHeader = `
╔════════════════════════════════════════════════╗
║   ____  ____  _____  ___  ____  ____  ____    ║
║  / __ \/ __ \/ __/ / _ \/ __ \/ __ \/ __ \   ║
║ / /_/ / /_/ / /_  / /_/ / /_/ / /_/ / /_/ /   ║
║ \____/\____/\__/  \____/\____/\____/\____/    ║
║                                              ║
║     Obsidian Vault Synchronization Tool      ║
╚════════════════════════════════════════════════╝
`

// Menu item types.
const (
	MenuItemInit = iota
	MenuItemSetServerURL
	MenuItemSetAPIKey
	MenuItemSetVaultID
	MenuItemShowConfig
	MenuItemExit
)

// menuItem represents a selectable menu option.
type menuItem struct {
	title       string
	description string
	action      int
}

func (m menuItem) Title() string       { return m.title }
func (m menuItem) Description() string { return m.description }
func (m menuItem) FilterValue() string { return m.title }

// statusMsg is a message type for displaying status updates.
type statusMsg struct {
	message string
	isError bool
}

// actionCompleteMsg signals that an action has completed.
type actionCompleteMsg struct{}

// menuModel is the Bubble Tea model for the menu.
type menuModel struct {
	list       list.Model
	vaultPath  string
	configFile string
	config     *config.Config
	quitting   bool
	inputMode  bool
	input      textinput.Model
	status     string
	statusErr  bool
}

// newMenuModel creates a new menu model.
func newMenuModel() *menuModel {
	vaultPath, err := os.Getwd()
	if err != nil {
		vaultPath = "."
	}

	// Try to load existing config.
	configFile := findConfigFile(vaultPath)
	cfg, err := config.Load(configFile, nil)
	if err != nil {
		// Config doesn't exist yet — use defaults.
		cfg = config.DefaultConfig()
	}

	items := []list.Item{
		menuItem{title: "Initialize", description: "Initialize osync in current vault", action: MenuItemInit},
		menuItem{title: "Set server_url", description: "Configure server URL", action: MenuItemSetServerURL},
		menuItem{title: "Set api_key", description: "Configure API key", action: MenuItemSetAPIKey},
		menuItem{title: "Set vault_id", description: "Configure vault ID", action: MenuItemSetVaultID},
		menuItem{title: "Show current config", description: "Display current configuration", action: MenuItemShowConfig},
		menuItem{title: "Exit", description: "Exit the menu", action: MenuItemExit},
	}

	// Create list with custom styles.
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "OSYNC Menu"
	l.Styles.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Bold(true)
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(false)

	// Create text input for user input.
	ti := textinput.New()
	ti.Placeholder = "Enter value..."
	ti.CharLimit = 256
	ti.Width = 40

	return &menuModel{
		list:       l,
		vaultPath:  vaultPath,
		configFile: configFile,
		config:     cfg,
		input:      ti,
	}
}

func (m *menuModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle input mode.
	if m.inputMode {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEnter:
				// Save the input value.
				if m.input.Value() != "" {
					m.saveInputValue()
				}
				m.inputMode = false
				m.input.SetValue("")
				return m, nil
			case tea.KeyCtrlC, tea.KeyEsc:
				// Cancel input.
				m.inputMode = false
				m.input.SetValue("")
				return m, nil
			}
		}
		// Update text input.
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	// Handle status messages.
	if s, ok := msg.(statusMsg); ok {
		m.status = s.message
		m.statusErr = s.isError
		return m, nil
	}

	// Handle action complete messages.
	if _, ok := msg.(actionCompleteMsg); ok {
		// Reload config after action.
		cfg, err := config.Load(m.configFile, nil)
		if err == nil {
			m.config = cfg
		}
		return m, nil
	}

	// Handle normal menu navigation.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := m.list.Styles.Title.GetFrameSize()
		if h == 0 && v == 0 {
			h, v = 2, 2
		}
		m.list.SetSize(msg.Width-h, msg.Height-v)
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyEnter:
			// Execute selected action.
			if item, ok := m.list.SelectedItem().(menuItem); ok {
				return m, m.executeAction(item.action)
			}
		}
	}

	// Update the list.
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *menuModel) View() string {
	if m.quitting {
		return ""
	}

	// Render header.
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Bold(true)
	header := headerStyle.Render(osyncHeader)

	// Show input mode.
	if m.inputMode {
		inputView := fmt.Sprintf("Enter %s: %s\n\n(Press Enter to confirm, Esc to cancel)", m.input.Placeholder, m.input.View())
		return header + "\n\n" + inputView
	}

	// Show status message if any.
	var statusView string
	if m.status != "" {
		statusStyle := lipgloss.NewStyle()
		if m.statusErr {
			statusStyle = statusStyle.Foreground(lipgloss.Color("196"))
		} else {
			statusStyle = statusStyle.Foreground(lipgloss.Color("42"))
		}
		statusView = statusStyle.Render(m.status) + "\n\n"
	}

	// Render list.
	listView := m.list.View()

	return header + "\n\n" + statusView + listView
}

// executeAction performs the action for the selected menu item.
func (m *menuModel) executeAction(action int) tea.Cmd {
	switch action {
	case MenuItemInit:
		return m.doInit()
	case MenuItemSetServerURL:
		return m.startInput("server_url", "Enter server URL (e.g., https://api.example.com)")
	case MenuItemSetAPIKey:
		return m.startInput("api_key", "Enter API key")
	case MenuItemSetVaultID:
		return m.startInput("vault_id", "Enter vault ID")
	case MenuItemShowConfig:
		return m.showConfig()
	case MenuItemExit:
		return tea.Quit
	default:
		return nil
	}
}

// startInput begins text input mode for a specific field.
func (m *menuModel) startInput(field, placeholder string) tea.Cmd {
	m.inputMode = true
	m.input.Placeholder = placeholder
	m.input.SetValue("")
	m.input.Focus()
	return textinput.Blink
}

// saveInputValue saves the input value to the config.
func (m *menuModel) saveInputValue() {
	field := m.input.Placeholder
	// Extract field name from placeholder.
	if strings.Contains(field, "server") {
		field = "server_url"
	} else if strings.Contains(field, "API") {
		field = "api_key"
	} else if strings.Contains(field, "vault") {
		field = "vault_id"
	}

	value := m.input.Value()
	if field == "" || value == "" {
		return
	}

	// Set the config value.
	if err := setConfigValue(m.config, field, value); err != nil {
		m.status = fmt.Sprintf("Error setting %s: %v", field, err)
		m.statusErr = true
		return
	}

	// Write the config file.
	if err := writeConfig(m.configFile, m.config); err != nil {
		m.status = fmt.Sprintf("Error writing config: %v", err)
		m.statusErr = true
		return
	}

	m.status = fmt.Sprintf("✓ Set %s", field)
	m.statusErr = false
}

// doInit performs the initialization action.
func (m *menuModel) doInit() tea.Cmd {
	return func() tea.Msg {
		// Create .osync/ directory.
		osyncDir := m.vaultPath + "/" + config.DefaultConfigDir
		if err := os.MkdirAll(osyncDir, 0o755); err != nil {
			return statusMsg{message: fmt.Sprintf("Error: %v", err), isError: true}
		}

		// Check if already initialized.
		if _, err := os.Stat(m.configFile); err == nil {
			return statusMsg{message: "osync already initialized in " + m.vaultPath, isError: false}
		}

		// Generate a vault ID.
		vaultID, err := generateVaultID()
		if err != nil {
			return statusMsg{message: fmt.Sprintf("Error generating vault ID: %v", err), isError: true}
		}

		// Write default config.
		cfg := config.DefaultConfig()
		cfg.VaultPath = m.vaultPath
		cfg.VaultID = vaultID

		if err := writeConfig(m.configFile, cfg); err != nil {
			return statusMsg{message: fmt.Sprintf("Error writing config: %v", err), isError: true}
		}

		return statusMsg{
			message: fmt.Sprintf("✓ Initialized osync in %s (Vault ID: %s)", m.vaultPath, vaultID),
			isError: false,
		}
	}
}

// showConfig displays the current configuration.
func (m *menuModel) showConfig() tea.Cmd {
	return func() tea.Msg {
		var sb strings.Builder
		sb.WriteString("Current Configuration:\n")
		sb.WriteString("======================\n")
		sb.WriteString(fmt.Sprintf("  server_url:     %s\n", m.config.ServerURL))
		sb.WriteString(fmt.Sprintf("  api_key:        %s\n", maskAPIKey(m.config.APIKey)))
		sb.WriteString(fmt.Sprintf("  vault_path:     %s\n", m.config.VaultPath))
		sb.WriteString(fmt.Sprintf("  vault_id:       %s\n", m.config.VaultID))
		sb.WriteString(fmt.Sprintf("  sync_interval:  %s\n", m.config.SyncInterval))
		sb.WriteString(fmt.Sprintf("  max_file_size:  %d\n", m.config.MaxFileSize))
		sb.WriteString(fmt.Sprintf("  port:           %d\n", m.config.Port))
		return statusMsg{message: sb.String(), isError: false}
	}
}

// maskAPIKey masks the API key for display (shows only first 4 and last 4 chars).
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

// runMenu launches the interactive TUI menu.
func runMenu() error {
	// Create model.
	model := newMenuModel()

	// Create program.
	p := tea.NewProgram(model, tea.WithAltScreen())

	// Run the program.
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("running menu: %w", err)
	}

	return nil
}
