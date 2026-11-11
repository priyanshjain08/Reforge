//go:build !linux

package collector

import "errors"

func diskPercent(string) (float64, error) { return 0, errors.New("disk metrics unsupported on this platform") }
