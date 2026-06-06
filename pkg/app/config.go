package app

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultConfigPath = ".autocimbar"

func ApplyINIConfig(fs *flag.FlagSet, command string, aliases map[string][]string) error {
	values, err := LoadINIConfig(command)
	if err != nil {
		return err
	}
	if len(values) == 0 {
		return nil
	}

	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	flagNames := map[string]string{}
	fs.VisitAll(func(f *flag.Flag) {
		flagNames[strings.ToLower(f.Name)] = f.Name
	})

	aliasToName := map[string]string{}
	for name, list := range aliases {
		for _, alias := range list {
			aliasToName[strings.ToLower(alias)] = name
		}
	}

	for key, value := range values {
		key = normalizeConfigKey(key)
		name, ok := aliasToName[strings.ToLower(key)]
		if !ok {
			name, ok = flagNames[strings.ToLower(key)]
		}
		if !ok {
			if isIgnoredConfigKey(key) {
				continue
			}
			return fmt.Errorf("unknown config key %q", key)
		}
		if visited[name] {
			continue
		}
		overriddenByAlias := false
		for _, alias := range aliases[name] {
			if visited[alias] {
				overriddenByAlias = true
				break
			}
		}
		if overriddenByAlias {
			continue
		}
		if err := fs.Set(name, value); err != nil {
			return fmt.Errorf("set config %s=%q: %w", name, value, err)
		}
	}
	return nil
}

func isIgnoredConfigKey(key string) bool {
	switch strings.ToLower(normalizeConfigKey(key)) {
	case "block-size", "timeout":
		return true
	default:
		return false
	}
}

func LoadINIConfig(command string) (map[string]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find user home: %w", err)
	}
	path := filepath.Join(home, DefaultConfigPath)
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open config %s: %w", path, err)
	}
	defer f.Close()

	command = strings.ToLower(strings.TrimSpace(command))
	defaults := map[string]string{}
	commandValues := map[string]string{}
	section := "default"
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: expected key=value", path, lineNo)
		}
		key = normalizeConfigKey(key)
		value = stripInlineComment(strings.TrimSpace(value))
		switch section {
		case "", "default":
			defaults[key] = value
		case command:
			commandValues[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	values := map[string]string{}
	for key, value := range defaults {
		values[key] = value
	}
	for key, value := range commandValues {
		values[key] = value
	}
	return values, nil
}

func normalizeConfigKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.TrimLeft(key, "-")
	key = strings.ReplaceAll(key, "_", "-")
	return key
}

func stripInlineComment(value string) string {
	quote := byte(0)
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\'', '"':
			if quote == 0 {
				quote = value[i]
			} else if quote == value[i] {
				quote = 0
			}
		case '#', ';':
			if quote == 0 && (i == 0 || value[i-1] == ' ' || value[i-1] == '\t') {
				return strings.TrimSpace(value[:i])
			}
		}
	}
	return strings.Trim(value, `"'`)
}
