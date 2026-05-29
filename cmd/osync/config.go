package main

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/user/obsidian-sync-f2p/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View and manage osync configuration",
	Long:  "Get and set configuration values. Use 'osync config get <key>' to view a value, 'osync config set <key> <value>' to update one.",
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration keys",
	Args:  cobra.NoArgs,
	RunE:  runConfigList,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configListCmd)
}

// configKeys returns the list of valid configuration key names.
func configKeys() []string {
	cfg := config.DefaultConfig()
	val := reflect.ValueOf(cfg).Elem()
	typ := val.Type()

	var keys []string
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("koanf")
		if tag != "" {
			keys = append(keys, tag)
		}
	}
	sort.Strings(keys)
	return keys
}

// isValidConfigKey checks if a key is a valid config field.
func isValidConfigKey(key string) bool {
	for _, k := range configKeys() {
		if k == key {
			return true
		}
	}
	return false
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	if !isValidConfigKey(key) {
		return fmt.Errorf("unknown config key %q; run 'osync config list' to see available keys", key)
	}

	vaultPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	configFile := findConfigFile(vaultPath)
	cfg, err := config.Load(configFile, nil)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	value, err := getConfigValue(cfg, key)
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.OutOrStdout(), value)
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	if !isValidConfigKey(key) {
		return fmt.Errorf("unknown config key %q; run 'osync config list' to see available keys", key)
	}

	vaultPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	configFile := findConfigFile(vaultPath)
	cfg, err := config.Load(configFile, nil)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if err := setConfigValue(cfg, key, value); err != nil {
		return err
	}

	// Write updated config.
	if err := writeConfig(configFile, cfg); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Set %s\n", key)
	return nil
}

func runConfigList(cmd *cobra.Command, args []string) error {
	keys := configKeys()
	for _, k := range keys {
		fmt.Fprintln(cmd.OutOrStdout(), k)
	}
	return nil
}

// findConfigFile locates the config file relative to the vault path.
func findConfigFile(vaultPath string) string {
	return fmt.Sprintf("%s/%s/%s", vaultPath, config.DefaultConfigDir, config.DefaultConfigFile)
}

// getConfigValue retrieves a config value by koanf key name.
func getConfigValue(cfg *config.Config, key string) (string, error) {
	val := reflect.ValueOf(cfg).Elem()
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("koanf")
		if tag == key {
			fieldVal := val.Field(i)
			switch fieldVal.Kind() {
			case reflect.String:
				return fieldVal.String(), nil
			case reflect.Int, reflect.Int64:
				// Check if it's a Duration.
				if field.Type.String() == "time.Duration" {
					return fieldVal.Interface().(fmt.Stringer).String(), nil
				}
				return fmt.Sprintf("%d", fieldVal.Int()), nil
			case reflect.Slice:
				// Format as comma-separated.
				sliceVal := fieldVal.Interface().([]string)
				return strings.Join(sliceVal, ","), nil
			default:
				return fmt.Sprintf("%v", fieldVal.Interface()), nil
			}
		}
	}

	return "", fmt.Errorf("config key %q not found", key)
}

// setConfigValue sets a config value by koanf key name.
func setConfigValue(cfg *config.Config, key, value string) error {
	val := reflect.ValueOf(cfg).Elem()
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("koanf")
		if tag == key {
			fieldVal := val.Field(i)
			switch fieldVal.Kind() {
			case reflect.String:
				fieldVal.SetString(value)
			case reflect.Int, reflect.Int64:
				if field.Type.String() == "time.Duration" {
					// Parse duration.
					d, err := parseDuration(value)
					if err != nil {
						return fmt.Errorf("invalid duration %q: %w", value, err)
					}
					fieldVal.SetInt(int64(d))
				} else {
					var intVal int64
					if _, err := fmt.Sscanf(value, "%d", &intVal); err != nil {
						return fmt.Errorf("invalid integer %q: %w", value, err)
					}
					fieldVal.SetInt(intVal)
				}
			case reflect.Slice:
				// Parse as comma-separated.
				parts := strings.Split(value, ",")
				for i := range parts {
					parts[i] = strings.TrimSpace(parts[i])
				}
				fieldVal.Set(reflect.ValueOf(parts))
			default:
				return fmt.Errorf("unsupported config type for key %q", key)
			}
			return nil
		}
	}

	return fmt.Errorf("config key %q not found", key)
}

// parseDuration parses a duration string, accepting common formats.
func parseDuration(s string) (int64, error) {
	var d int64
	var unit string
	if _, err := fmt.Sscanf(s, "%d%s", &d, &unit); err != nil {
		return 0, fmt.Errorf("parsing duration: %w", err)
	}

	switch unit {
	case "ns":
		return d, nil
	case "us", "µs":
		return d * 1000, nil
	case "ms":
		return d * 1000000, nil
	case "s":
		return d * 1000000000, nil
	case "m":
		return d * 60000000000, nil
	case "h":
		return d * 3600000000000, nil
	default:
		return 0, fmt.Errorf("unknown duration unit %q", unit)
	}
}
