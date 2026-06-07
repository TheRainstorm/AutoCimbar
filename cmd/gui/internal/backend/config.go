//go:build windows

package backend

import (
	"fmt"
	"strconv"
	"strings"

	coreapp "github.com/autocambar/autocambar/pkg/app"
)

type ConfigService struct {
	current TransferConfig
	lite    bool
}

func NewConfigService() *ConfigService {
	return NewConfigServiceWithMode(false)
}

func NewConfigServiceWithMode(lite bool) *ConfigService {
	return &ConfigService{current: LoadDefaultTransferConfigForMode(lite), lite: lite}
}

func (s *ConfigService) GetConfig() TransferConfig {
	return s.current
}

func (s *ConfigService) SaveConfig(cfg TransferConfig) (TransferConfig, error) {
	cfg = normalizeConfig(cfg)
	if s.lite {
		cfg = enforceLiteConfig(cfg)
	}
	if err := ValidateConfig(cfg); err != nil {
		return TransferConfig{}, err
	}
	s.current = cfg
	return s.current, nil
}

func (s *ConfigService) ResetConfig() TransferConfig {
	s.current = LoadDefaultTransferConfigForMode(s.lite)
	return s.current
}

func (s *ConfigService) ValidateConfig(cfg TransferConfig) error {
	cfg = normalizeConfig(cfg)
	if s.lite {
		cfg = enforceLiteConfig(cfg)
	}
	return ValidateConfig(cfg)
}

func ValidateConfig(cfg TransferConfig) error {
	if cfg.RQ <= 0 && cfg.Q <= 0 {
		return fmt.Errorf("RQ must be > 0")
	}
	if cfg.Scale <= 0 {
		return fmt.Errorf("B must be > 0")
	}
	if cfg.FPS <= 0 {
		return fmt.Errorf("fps must be > 0")
	}
	cell, err := coreapp.ParseCellSpec(cfg.Cell, coreapp.CellSpec{Tile: "8x8", ShapeBits: 4, ColorBits: 2})
	if err != nil {
		return err
	}
	if _, err := coreapp.ParseTileSpec(cell.Tile, cell.ShapeBits); err != nil {
		return err
	}
	ecc := eccValue(cfg)
	if ecc < 0 || ecc > 100 {
		return fmt.Errorf("ecc must be 0..100")
	}
	if cfg.Packets <= 0 {
		return fmt.Errorf("packets must be > 0")
	}
	if cfg.DecodeWorkers < 0 {
		return fmt.Errorf("decode-workers must be >= 0")
	}
	if _, err := normalizeBackend(cfg.Backend); err != nil {
		return err
	}
	if _, err := coreapp.NormalizeCaptureBackendForConfig(cfg.CaptureBackend); err != nil {
		return err
	}
	return nil
}

func normalizeConfig(cfg TransferConfig) TransferConfig {
	def := LoadDefaultTransferConfig()
	if cfg.RQ == 0 {
		if cfg.Q > 0 {
			cfg.RQ = cfg.Q
		} else {
			cfg.RQ = def.RQ
		}
	}
	if cfg.Cell == "" {
		cfg.Cell = def.Cell
	}
	if cfg.ECC == nil {
		cfg.ECC = def.ECC
	}
	if cfg.Packets == 0 {
		cfg.Packets = def.Packets
	}
	if cfg.Position == "" {
		cfg.Position = def.Position
	}
	if cfg.Scale == 0 {
		cfg.Scale = def.Scale
	}
	if cfg.FPS == 0 {
		cfg.FPS = def.FPS
	}
	if strings.TrimSpace(cfg.Output) == "" {
		cfg.Output = def.Output
	}
	if cfg.Backend == "" {
		cfg.Backend = def.Backend
	}
	if backend, err := normalizeBackend(cfg.Backend); err == nil {
		cfg.Backend = backend
	}
	if cfg.SymbolDir == "" {
		cfg.SymbolDir = def.SymbolDir
	}
	if cfg.CaptureBackend == "" {
		cfg.CaptureBackend = def.CaptureBackend
	}
	if backend, err := coreapp.NormalizeCaptureBackendForConfig(cfg.CaptureBackend); err == nil {
		cfg.CaptureBackend = backend
	}
	return cfg
}

func regionFromConfig(cfg TransferConfig) string {
	position := strings.TrimSpace(cfg.Position)
	if position == "" {
		position = "-0:-0"
	}
	return fmt.Sprintf("%d:%s", cfg.Screen, position)
}

func cellParts(cfg TransferConfig) (tile string, shapeBits int, colorBits int, err error) {
	cell, err := coreapp.ParseCellSpec(cfg.Cell, coreapp.CellSpec{Tile: "8x8", ShapeBits: 4, ColorBits: 2})
	if err != nil {
		return "", 0, 0, err
	}
	return cell.Tile, cell.ShapeBits, cell.ColorBits, nil
}

func gridSizeFromConfig(cfg TransferConfig, tile string, shapeBits int) (int, error) {
	spec, err := coreapp.ParseTileSpec(tile, shapeBits)
	if err != nil {
		return 0, err
	}
	return coreapp.ResolveGridSize(cfg.Q, cfg.RQ, spec)
}

func eccValue(cfg TransferConfig) int {
	if cfg.ECC == nil {
		return *LoadDefaultTransferConfig().ECC
	}
	return *cfg.ECC
}

func LoadDefaultTransferConfig() TransferConfig {
	return LoadDefaultTransferConfigForMode(false)
}

func LoadDefaultTransferConfigForMode(lite bool) TransferConfig {
	if lite {
		return LiteTransferConfig()
	}
	cfg := DefaultTransferConfig()
	values, err := coreapp.LoadINIConfig("gui")
	if err != nil || len(values) == 0 {
		return cfg
	}
	applyConfigValues(&cfg, values)
	return normalizeConfigForDefaults(cfg)
}

func EnforceLiteConfig(cfg TransferConfig) TransferConfig {
	return enforceLiteConfig(cfg)
}

func enforceLiteConfig(cfg TransferConfig) TransferConfig {
	lite := LiteTransferConfig()
	if cfg.RQ <= 0 {
		cfg.RQ = lite.RQ
	}
	if cfg.RQ > LiteMaxRQ {
		cfg.RQ = LiteMaxRQ
	}
	cfg.Q = 0
	cfg.Cell = lite.Cell
	cfg.ECC = lite.ECC
	cfg.Packets = lite.Packets
	if strings.TrimSpace(cfg.Position) == "" {
		cfg.Position = lite.Position
	}
	if cfg.Scale <= 0 {
		cfg.Scale = lite.Scale
	}
	cfg.FPS = lite.FPS
	if strings.TrimSpace(cfg.Output) == "" {
		cfg.Output = lite.Output
	}
	cfg.Backend = lite.Backend
	cfg.SymbolDir = ""
	cfg.NoZstd = false
	cfg.DecodeWorkers = 0
	cfg.CaptureBackend = lite.CaptureBackend
	return cfg
}

func normalizeConfigForDefaults(cfg TransferConfig) TransferConfig {
	def := DefaultTransferConfig()
	if cfg.RQ == 0 {
		if cfg.Q > 0 {
			cfg.RQ = cfg.Q
		} else {
			cfg.RQ = def.RQ
		}
	}
	if cfg.Cell == "" {
		cfg.Cell = def.Cell
	}
	if cfg.ECC == nil {
		cfg.ECC = def.ECC
	}
	if cfg.Packets == 0 {
		cfg.Packets = def.Packets
	}
	if cfg.Position == "" {
		cfg.Position = def.Position
	}
	if cfg.Scale == 0 {
		cfg.Scale = def.Scale
	}
	if cfg.FPS == 0 {
		cfg.FPS = def.FPS
	}
	if cfg.Output == "" {
		cfg.Output = def.Output
	}
	if cfg.Backend == "" {
		cfg.Backend = def.Backend
	}
	if cfg.CaptureBackend == "" {
		cfg.CaptureBackend = def.CaptureBackend
	}
	return cfg
}

func applyConfigValues(cfg *TransferConfig, values map[string]string) {
	aliases := map[string]string{
		"c":       "cell",
		"p":       "packets",
		"r":       "R",
		"f":       "fps",
		"s":       "symbols",
		"b":       "B",
		"rq":      "RQ",
		"q":       "Q",
		"no_zstd": "no-zstd",
	}
	hasRQ := false
	for rawKey := range values {
		key := strings.TrimLeft(strings.TrimSpace(rawKey), "-")
		key = strings.ReplaceAll(key, "_", "-")
		lower := strings.ToLower(key)
		if alias, ok := aliases[lower]; ok {
			key = alias
		}
		if strings.EqualFold(key, "RQ") {
			hasRQ = true
			break
		}
	}
	for rawKey, value := range values {
		key := strings.TrimLeft(strings.TrimSpace(rawKey), "-")
		key = strings.ReplaceAll(key, "_", "-")
		lower := strings.ToLower(key)
		if alias, ok := aliases[lower]; ok {
			key = alias
		}
		switch strings.ToLower(key) {
		case "rq":
			setInt(&cfg.RQ, value)
		case "q":
			if setInt(&cfg.Q, value) && !hasRQ {
				cfg.RQ = cfg.Q
			}
		case "screen":
			setInt(&cfg.Screen, value)
		case "cell":
			cfg.Cell = value
		case "ecc":
			var ecc int
			if setInt(&ecc, value) {
				cfg.ECC = &ecc
			}
		case "packets":
			setInt(&cfg.Packets, value)
		case "r":
			cfg.Screen, cfg.Position = parseRegionForGUI(value, cfg.Screen, cfg.Position)
		case "position":
			cfg.Position = value
		case "b":
			setInt(&cfg.Scale, value)
		case "fps":
			setInt(&cfg.FPS, value)
		case "o", "output":
			cfg.Output = value
		case "backend":
			cfg.Backend = value
		case "symbols":
			cfg.SymbolDir = value
		case "no-zstd":
			setBool(&cfg.NoZstd, value)
		case "decode-workers":
			setInt(&cfg.DecodeWorkers, value)
		case "capture-backend":
			cfg.CaptureBackend = value
		}
	}
}

func setInt(dst *int, value string) bool {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	*dst = n
	return true
}

func setBool(dst *bool, value string) {
	v, err := strconv.ParseBool(strings.TrimSpace(value))
	if err == nil {
		*dst = v
	}
}

func parseRegionForGUI(value string, defaultScreen int, defaultPosition string) (int, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultScreen, defaultPosition
	}
	parts := strings.Split(value, ":")
	switch len(parts) {
	case 1:
		if screen, err := strconv.Atoi(parts[0]); err == nil {
			return screen, defaultPosition
		}
	case 2:
		return defaultScreen, value
	case 3:
		screen, err := strconv.Atoi(parts[0])
		if err != nil {
			return defaultScreen, defaultPosition
		}
		return screen, parts[1] + ":" + parts[2]
	}
	return defaultScreen, defaultPosition
}

func normalizeBackend(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return coreapp.BackendSymbols, nil
	}
	switch name {
	case coreapp.BackendSymbols, "symbol", "cimbar", "tiles":
		return coreapp.BackendSymbols, nil
	case coreapp.BackendQR, "qrcode":
		return coreapp.BackendQR, nil
	default:
		return "", fmt.Errorf("unknown backend %q; use symbols or qr", name)
	}
}
