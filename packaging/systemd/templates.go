// Package servicetemplates embeds the reviewed unit sources in the installer
// binary; arbitrary downloaded unit text cannot enter a configuration plan.
package servicetemplates

import "embed"

//go:embed *.service
var units embed.FS

func Unit(name string) ([]byte, error) { return units.ReadFile(name) }
