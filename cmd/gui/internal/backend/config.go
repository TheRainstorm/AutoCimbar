//go:build windows

package backend

import (
	"fmt"
	"strings"

	coreapp "github.com/autocambar/autocambar/pkg/app"
)

type ConfigService struct {
	current TransferConfig
}

func NewConfigService() *ConfigService {
	return &ConfigService{current: DefaultTransferConfig()}
}

func (s *ConfigService) GetConfig() TransferConfig {
	return s.current
}

func (s *ConfigService) SaveConfig(cfg TransferConfig) (TransferConfig, error) {
	cfg = normalizeConfig(cfg)
	if err := ValidateConfig(cfg); err != nil {
		return TransferConfig{}, err
	}
	s.current = cfg
	return s.current, nil
}

func (s *ConfigService) ResetConfig() TransferConfig {
	s.current = DefaultTransferConfig()
	return s.current
}

func (s *ConfigService) ValidateConfig(cfg TransferConfig) error {
	return ValidateConfig(normalizeConfig(cfg))
}

func ValidateConfig(cfg TransferConfig) error {
	if cfg.Q <= 0 {
		return fmt.Errorf("Q must be > 0")
	}
	if cfg.Scale <= 0 {
		return fmt.Errorf("scale must be > 0")
	}
	if cfg.FPS <= 0 {
		return fmt.Errorf("fps must be > 0")
	}
	cell, err := coreapp.ParseCellSpec(cfg.Cell, coreapp.CellSpec{Tile: "4x4", ShapeBits: 4, ColorBits: 8})
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
	return nil
}

func normalizeConfig(cfg TransferConfig) TransferConfig {
	def := DefaultTransferConfig()
	if cfg.Q == 0 {
		cfg.Q = def.Q
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
	cell, err := coreapp.ParseCellSpec(cfg.Cell, coreapp.CellSpec{Tile: "4x4", ShapeBits: 4, ColorBits: 8})
	if err != nil {
		return "", 0, 0, err
	}
	return cell.Tile, cell.ShapeBits, cell.ColorBits, nil
}

func eccValue(cfg TransferConfig) int {
	if cfg.ECC == nil {
		return *DefaultTransferConfig().ECC
	}
	return *cfg.ECC
}
